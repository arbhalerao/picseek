package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/arbhalerao/picseek/internal/config"
	"github.com/arbhalerao/picseek/internal/indexer"
	"github.com/arbhalerao/picseek/internal/ocr"
	"github.com/arbhalerao/picseek/internal/pipeline"
	"github.com/arbhalerao/picseek/internal/store"
)

const (
	gcInterval  = time.Hour
	orphanGrace = 7 * 24 * time.Hour
)

type Daemon struct {
	cfg     config.Config
	store   *store.Store
	indexer *indexer.Indexer
	watcher *indexer.Watcher
	pool    *ocr.Pool
	ocr     *pipeline.OCRStage
	wake    chan struct{}
	log     *slog.Logger
}

func New(ctx context.Context, cfg config.Config, log *slog.Logger) (*Daemon, error) {
	dataDir, err := config.DataDir()
	if err != nil {
		return nil, err
	}
	st, err := store.Open(ctx, filepath.Join(dataDir, "index.db"))
	if err != nil {
		return nil, err
	}
	if n, err := st.ReleaseAllLeases(ctx); err != nil {
		st.Close()
		return nil, err
	} else if n > 0 {
		log.Info("released leases left by a previous run", "jobs", n)
	}
	for _, folder := range cfg.Index.Folders {
		if _, err := st.AddFolder(ctx, folder); err != nil {
			st.Close()
			return nil, err
		}
	}

	d := &Daemon{cfg: cfg, store: st, wake: make(chan struct{}, 1), log: log}
	filter := indexer.NewFilter(cfg.Index.Extensions, cfg.Index.Exclude, cfg.Index.MaxFileMB)
	d.indexer = indexer.New(st, filter, runtime.NumCPU(), d.notify, log)
	if d.watcher, err = indexer.NewWatcher(d.indexer); err != nil {
		st.Close()
		return nil, err
	}

	sockets := make(map[string]string, cfg.OCR.Workers)
	for i := 1; i <= cfg.OCR.Workers; i++ {
		name := fmt.Sprintf("ocr-%d", i)
		sockets[name] = cfg.SocketPath(name)
	}
	if d.pool, err = ocr.NewPool(sockets, log); err != nil {
		d.watcher.Close()
		st.Close()
		return nil, err
	}
	d.ocr = pipeline.NewOCRStage(st, d.pool, d.wake, log)
	return d, nil
}

func (d *Daemon) Close() error {
	return errors.Join(d.watcher.Close(), d.pool.Close(), d.store.Close())
}

func (d *Daemon) Run(ctx context.Context) error {
	folders, err := d.store.Folders(ctx)
	if err != nil {
		return err
	}
	for _, root := range folders {
		if err := d.watcher.AddRoot(root); err != nil {
			d.log.Warn("cannot watch folder", "folder", root, "err", err)
		}
	}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return d.watcher.Run(gctx) })
	g.Go(func() error {
		for _, root := range folders {
			d.scan(gctx, root)
		}
		return nil
	})
	g.Go(func() error { return d.collectGarbage(gctx) })
	g.Go(func() error {
		d.pool.MonitorHealth(gctx)
		return nil
	})
	g.Go(func() error { return d.ocr.Run(gctx) })
	return g.Wait()
}

func (d *Daemon) scan(ctx context.Context, root string) {
	stats, err := d.indexer.Scan(ctx, root)
	if err != nil {
		if ctx.Err() == nil {
			d.log.Warn("scan failed", "folder", root, "err", err)
		}
		return
	}
	d.log.Info("scan complete", "folder", root, "files", stats.Seen, "changed", stats.Changed,
		"new_images", stats.NewImages, "reused", stats.Reused, "removed", stats.Removed,
		"took", stats.Duration.Round(time.Millisecond))
}

func (d *Daemon) collectGarbage(ctx context.Context) error {
	ticker := time.NewTicker(gcInterval)
	defer ticker.Stop()
	for {
		removed, err := d.store.CollectGarbage(ctx, time.Now().Add(-orphanGrace))
		switch {
		case err != nil && ctx.Err() == nil:
			d.log.Warn("garbage collection failed", "err", err)
		case removed > 0:
			d.log.Info("garbage collected orphaned images", "count", removed)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (d *Daemon) notify() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}
