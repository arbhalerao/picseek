package ocr

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
	ocrv1 "github.com/arbhalerao/picseek/gen/go/picseek/ocr/v1"
	"github.com/arbhalerao/picseek/internal/breaker"
)

const (
	serviceName      = "picseek.ocr.v1.OcrService"
	healthInterval   = 5 * time.Second
	recoverInterval  = 1 * time.Second
	healthTimeout    = 2 * time.Second
	breakerThreshold = 5
	breakerCooldown  = 10 * time.Second
)

type Worker struct {
	Name    string
	conn    *grpc.ClientConn
	client  ocrv1.OcrServiceClient
	health  healthpb.HealthClient
	breaker *breaker.Breaker
	healthy atomic.Bool
}

type Pool struct {
	Workers []*Worker
	log     *slog.Logger
}

type Result struct {
	Text    string
	Version string
}

func NewPool(sockets map[string]string, log *slog.Logger) (*Pool, error) {
	p := &Pool{log: log}
	for name, socket := range sockets {
		conn, err := grpc.NewClient("unix://"+socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			p.Close()
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		p.Workers = append(p.Workers, &Worker{
			Name:    name,
			conn:    conn,
			client:  ocrv1.NewOcrServiceClient(conn),
			health:  healthpb.NewHealthClient(conn),
			breaker: breaker.New(breakerThreshold, breakerCooldown),
		})
	}
	return p, nil
}

func (p *Pool) Close() error {
	var errs []error
	for _, w := range p.Workers {
		errs = append(errs, w.conn.Close())
	}
	return errors.Join(errs...)
}

func (p *Pool) Healthy() int {
	n := 0
	for _, w := range p.Workers {
		if w.Available() {
			n++
		}
	}
	return n
}

func (p *Pool) MonitorHealth(ctx context.Context) {
	for {
		interval := healthInterval
		for _, w := range p.Workers {
			if !w.checkHealth(ctx, p.log) {
				interval = recoverInterval
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func (p *Pool) Version(ctx context.Context) (string, error) {
	var errs []error
	for _, w := range p.Workers {
		callCtx, cancel := context.WithTimeout(ctx, healthTimeout)
		info, err := w.client.Info(callCtx, &ocrv1.InfoRequest{})
		cancel()
		if err == nil {
			return info.GetEngine() + "/" + info.GetEngineVersion(), nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", w.Name, err))
	}
	return "", errors.Join(errs...)
}

func (w *Worker) Available() bool {
	return w.healthy.Load() && !w.breaker.Open()
}

func (w *Worker) Ready() bool {
	return w.healthy.Load() && w.breaker.Allow()
}

func (w *Worker) Recognize(ctx context.Context, path string) (Result, error) {
	resp, err := w.client.Recognize(ctx, &ocrv1.RecognizeRequest{
		Image: &commonv1.ImageSource{Source: &commonv1.ImageSource_Path{Path: path}},
	})
	if err != nil {
		return Result{}, err
	}
	text := make([]byte, 0, 256)
	for i, line := range resp.GetLines() {
		if i > 0 {
			text = append(text, '\n')
		}
		text = append(text, line.GetText()...)
	}
	return Result{Text: string(text), Version: resp.GetEngine() + "/" + resp.GetEngineVersion()}, nil
}

func (w *Worker) MarkUnhealthy() {
	w.healthy.Store(false)
}

func (w *Worker) Breaker() *breaker.Breaker {
	return w.breaker
}

func (w *Worker) checkHealth(ctx context.Context, log *slog.Logger) bool {
	callCtx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	resp, err := w.health.Check(callCtx, &healthpb.HealthCheckRequest{Service: serviceName})
	healthy := err == nil && resp.GetStatus() == healthpb.HealthCheckResponse_SERVING
	if was := w.healthy.Swap(healthy); was != healthy {
		if healthy {
			log.Info("ocr worker healthy", "worker", w.Name)
		} else {
			log.Warn("ocr worker unhealthy", "worker", w.Name, "err", err)
		}
	}
	return healthy
}
