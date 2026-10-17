package embed

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	commonv1 "github.com/arbhalerao/picseek/gen/go/picseek/common/v1"
	embedv1 "github.com/arbhalerao/picseek/gen/go/picseek/embed/v1"
	"github.com/arbhalerao/picseek/internal/breaker"
)

const (
	serviceName      = "picseek.embed.v1.EmbedService"
	healthInterval   = 5 * time.Second
	recoverInterval  = time.Second
	healthTimeout    = 2 * time.Second
	breakerThreshold = 5
	breakerCooldown  = 10 * time.Second
)

var ErrUnavailable = errors.New("embedding service unavailable")

type Client struct {
	conn    *grpc.ClientConn
	client  embedv1.EmbedServiceClient
	health  healthpb.HealthClient
	breaker *breaker.Breaker
	healthy atomic.Bool
	log     *slog.Logger
}

type Versions struct {
	Text      string
	ClipImage string
	ClipText  string
}

func New(socket string, log *slog.Logger) (*Client, error) {
	conn, err := grpc.NewClient("unix://"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &Client{
		conn:    conn,
		client:  embedv1.NewEmbedServiceClient(conn),
		health:  healthpb.NewHealthClient(conn),
		breaker: breaker.New(breakerThreshold, breakerCooldown),
		log:     log,
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) Available() bool {
	return c.healthy.Load() && !c.breaker.Open()
}

func (c *Client) Ready() bool {
	return c.healthy.Load() && c.breaker.Allow()
}

func (c *Client) Breaker() *breaker.Breaker {
	return c.breaker
}

func (c *Client) MarkUnhealthy() {
	c.healthy.Store(false)
}

func (c *Client) MonitorHealth(ctx context.Context) {
	for {
		interval := healthInterval
		if !c.checkHealth(ctx) {
			interval = recoverInterval
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func (c *Client) Versions(ctx context.Context) (Versions, error) {
	callCtx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	info, err := c.client.Info(callCtx, &embedv1.InfoRequest{})
	if err != nil {
		return Versions{}, err
	}
	return Versions{Text: version(info.GetText()), ClipImage: version(info.GetClipImage()), ClipText: version(info.GetClipText())}, nil
}

func (c *Client) EmbedTexts(ctx context.Context, texts []string) ([][]float32, string, error) {
	resp, err := c.client.EmbedTexts(ctx, &embedv1.EmbedTextsRequest{Texts: texts})
	if err != nil {
		return nil, "", err
	}
	return vectors(resp.GetVectors()), version(resp.GetModel()), nil
}

func (c *Client) EmbedClipTexts(ctx context.Context, texts []string) ([][]float32, string, error) {
	resp, err := c.client.EmbedClipTexts(ctx, &embedv1.EmbedClipTextsRequest{Texts: texts})
	if err != nil {
		return nil, "", err
	}
	return vectors(resp.GetVectors()), version(resp.GetModel()), nil
}

func (c *Client) EmbedImages(ctx context.Context, paths []string) ([][]float32, string, error) {
	sources := make([]*commonv1.ImageSource, len(paths))
	for i, p := range paths {
		sources[i] = &commonv1.ImageSource{Source: &commonv1.ImageSource_Path{Path: p}}
	}
	resp, err := c.client.EmbedImages(ctx, &embedv1.EmbedImagesRequest{Images: sources})
	if err != nil {
		return nil, "", err
	}
	return vectors(resp.GetVectors()), version(resp.GetModel()), nil
}

func (c *Client) checkHealth(ctx context.Context) bool {
	callCtx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	resp, err := c.health.Check(callCtx, &healthpb.HealthCheckRequest{Service: serviceName})
	healthy := err == nil && resp.GetStatus() == healthpb.HealthCheckResponse_SERVING
	if was := c.healthy.Swap(healthy); was != healthy {
		if healthy {
			c.log.Info("embedding service healthy")
		} else {
			c.log.Warn("embedding service unhealthy", "err", err)
		}
	}
	return healthy
}

func vectors(in []*embedv1.Vector) [][]float32 {
	out := make([][]float32, len(in))
	for i, v := range in {
		out[i] = v.GetValues()
	}
	return out
}

func version(m *embedv1.ModelInfo) string {
	return fmt.Sprintf("%s/%s", m.GetName(), m.GetVersion())
}
