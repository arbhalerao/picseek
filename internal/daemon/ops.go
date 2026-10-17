package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/arbhalerao/picseek/internal/search"
	"github.com/arbhalerao/picseek/internal/store"
)

var (
	ErrInvalidFolder = errors.New("invalid folder")
	ErrConfigFolder  = errors.New("folder is set in config.toml")
	ErrUnknownFolder = errors.New("folder is not being watched")
)

type Status struct {
	Folders      []string
	Counts       store.QueueCounts
	Rate         float64
	ETA          time.Duration
	Skipped      int64
	OCRHealthy   int
	OCRTotal     int
	EmbedHealthy bool
	EnrichRate   float64
}

func (d *Daemon) Search(ctx context.Context, raw string, limit int, layers []search.MatchKind) (search.Result, error) {
	if limit <= 0 {
		limit = d.cfg.Search.DefaultLimit
	}
	return d.engine.Search(ctx, raw, limit, layers)
}

func (d *Daemon) AddFolders(ctx context.Context, paths []string) ([]string, error) {
	var added []string
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			return added, fmt.Errorf("%w: %s is not an absolute path", ErrInvalidFolder, p)
		}
		p = filepath.Clean(p)
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			return added, fmt.Errorf("%w: %s is not a directory", ErrInvalidFolder, p)
		}
		isNew, err := d.store.AddFolder(ctx, p)
		if err != nil {
			return added, err
		}
		if isNew {
			added = append(added, p)
			if err := d.watcher.AddRoot(p); err != nil {
				d.log.Warn("cannot watch folder", "folder", p, "err", err)
			}
		}
		d.scan(d.runCtx, p)
	}
	return added, nil
}

func (d *Daemon) ForgetFolder(ctx context.Context, path string) (int64, error) {
	path = filepath.Clean(path)
	if slices.Contains(d.cfg.Index.Folders, path) {
		return 0, fmt.Errorf("%w: remove %s from index.folders first", ErrConfigFolder, path)
	}
	folders, err := d.store.Folders(ctx)
	if err != nil {
		return 0, err
	}
	if !slices.Contains(folders, path) {
		return 0, fmt.Errorf("%w: %s", ErrUnknownFolder, path)
	}
	d.watcher.RemoveRoot(path)
	return d.store.ForgetFolder(ctx, path)
}

func (d *Daemon) RetryFailed(ctx context.Context) (int64, error) {
	n, err := d.store.RetryFailed(ctx)
	if n > 0 {
		d.notify()
	}
	return n, err
}

func (d *Daemon) Status(ctx context.Context) (Status, error) {
	folders, err := d.store.Folders(ctx)
	if err != nil {
		return Status{}, err
	}
	counts, err := d.store.Counts(ctx)
	if err != nil {
		return Status{}, err
	}
	st := Status{
		Folders:      folders,
		Counts:       counts,
		Rate:         d.ocr.Meter.Rate(),
		Skipped:      d.indexer.Skipped(),
		OCRHealthy:   d.pool.Healthy(),
		OCRTotal:     len(d.pool.Workers),
		EmbedHealthy: d.embed.Available(),
		EnrichRate:   d.enrich.Meter.Rate(),
	}
	if queued := counts.QueuedLive + counts.QueuedBackfill; queued > 0 && st.Rate > 0 {
		st.ETA = time.Duration(float64(queued) / st.Rate * float64(time.Second))
	}
	return st, nil
}
