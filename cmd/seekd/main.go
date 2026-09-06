package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/arbhalerao/picseek/internal/config"
	"github.com/arbhalerao/picseek/internal/daemon"
)

const (
	drainTimeout  = 10 * time.Second
	maxSocketPath = 107
)

func main() {
	if err := run(); err != nil {
		slog.Error("seekd stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	defaultPath, err := config.DefaultPath()
	if err != nil {
		return err
	}
	configPath := flag.String("config", defaultPath, "path to config.toml")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	lis, err := listenUnix(cfg.SocketPath("seekd"))
	if err != nil {
		return err
	}

	d, err := daemon.New(ctx, cfg, slog.Default())
	if err != nil {
		return err
	}
	defer d.Close()

	server := grpc.NewServer()
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(server, healthServer)

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(lis) }()
	runErr := make(chan error, 1)
	go func() { runErr <- d.Run(ctx) }()
	slog.Info("seekd listening", "socket", lis.Addr().String(), "config", *configPath)

	select {
	case err := <-serveErr:
		return err
	case err := <-runErr:
		if err != nil {
			return err
		}
		<-ctx.Done()
	case <-ctx.Done():
	}

	slog.Info("seekd shutting down")
	healthServer.Shutdown()
	shutdown(server)
	select {
	case err := <-runErr:
		if err != nil {
			slog.Warn("background work stopped with error", "err", err)
		}
	case <-time.After(drainTimeout):
		slog.Warn("drain timeout exceeded, unfinished jobs will be retried")
	}
	return nil
}

func listenUnix(path string) (net.Listener, error) {
	if len(path) > maxSocketPath {
		return nil, fmt.Errorf("socket path %q is %d bytes, the limit is %d: set a shorter services.socket_dir", path, len(path), maxSocketPath)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if conn, err := net.DialTimeout("unix", path, time.Second); err == nil {
		conn.Close()
		return nil, fmt.Errorf("seekd is already running on %s", path)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove stale socket: %w", err)
	}
	return net.Listen("unix", path)
}

func shutdown(server *grpc.Server) {
	done := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(drainTimeout):
		slog.Warn("drain timeout exceeded, forcing stop")
		server.Stop()
	}
}
