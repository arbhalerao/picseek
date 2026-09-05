package indexer

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cespare/xxhash/v2"
	"golang.org/x/sync/errgroup"

	"github.com/arbhalerao/picseek/internal/store"
)

const (
	batchSize     = 200
	batchInterval = 250 * time.Millisecond
	queueDepth    = 256
)

type Indexer struct {
	store       *store.Store
	filter      Filter
	hashWorkers int
	onNewJobs   func()
	log         *slog.Logger
	skipped     atomic.Int64
}

type ScanStats struct {
	Seen      int
	Changed   int
	NewImages int
	Reused    int
	Removed   int
	Duration  time.Duration
}

type candidate struct {
	path  string
	root  string
	size  int64
	mtime time.Time
	lane  store.Lane
}

func New(st *store.Store, filter Filter, hashWorkers int, onNewJobs func(), log *slog.Logger) *Indexer {
	return &Indexer{store: st, filter: filter, hashWorkers: hashWorkers, onNewJobs: onNewJobs, log: log}
}

func (ix *Indexer) Skipped() int64 {
	return ix.skipped.Load()
}

func (ix *Indexer) Scan(ctx context.Context, root string) (ScanStats, error) {
	started := time.Now()
	stamps, err := ix.store.Stamps(ctx, root)
	if err != nil {
		return ScanStats{}, err
	}

	var stats ScanStats
	seen := make(map[string]struct{}, len(stamps))
	candidates := make(chan candidate, queueDepth)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		defer close(candidates)
		return ix.walk(gctx, root, root, store.LaneBackfill, func(c candidate) error {
			seen[c.path] = struct{}{}
			stats.Seen++
			if stamp, ok := stamps[c.path]; ok && stamp == (store.Stamp{Size: c.size, Mtime: c.mtime.UnixNano()}) {
				return nil
			}
			stats.Changed++
			select {
			case candidates <- c:
				return nil
			case <-gctx.Done():
				return gctx.Err()
			}
		})
	})

	var result store.UpsertResult
	g.Go(func() error {
		var err error
		result, err = ix.process(gctx, candidates)
		return err
	})
	if err := g.Wait(); err != nil {
		return stats, err
	}

	var removed []string
	for p := range stamps {
		if _, ok := seen[p]; !ok {
			removed = append(removed, p)
		}
	}
	if err := ix.store.RemovePaths(ctx, removed); err != nil {
		return stats, err
	}

	stats.NewImages = result.NewImages
	stats.Reused = result.Reused
	stats.Removed = len(removed)
	stats.Duration = time.Since(started)
	return stats, nil
}

func (ix *Indexer) IndexTree(ctx context.Context, root, dir string, lane store.Lane) error {
	candidates := make(chan candidate, queueDepth)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		defer close(candidates)
		return ix.walk(gctx, root, dir, lane, func(c candidate) error {
			select {
			case candidates <- c:
				return nil
			case <-gctx.Done():
				return gctx.Err()
			}
		})
	})
	g.Go(func() error {
		_, err := ix.process(gctx, candidates)
		return err
	})
	return g.Wait()
}

func (ix *Indexer) IndexFiles(ctx context.Context, files []candidate) error {
	candidates := make(chan candidate, len(files))
	for _, c := range files {
		candidates <- c
	}
	close(candidates)
	_, err := ix.process(ctx, candidates)
	return err
}

func (ix *Indexer) walk(ctx context.Context, root, dir string, lane store.Lane, emit func(candidate) error) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir {
				return err
			}
			ix.log.Warn("skipping unreadable path", "path", path, "err", err)
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if path != dir && ix.filter.Excluded(path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() || !ix.filter.Image(path) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if ix.filter.TooLarge(info.Size()) {
			ix.skipped.Add(1)
			return nil
		}
		return emit(candidate{path: path, root: root, size: info.Size(), mtime: info.ModTime(), lane: lane})
	})
}

func (ix *Indexer) process(ctx context.Context, candidates <-chan candidate) (store.UpsertResult, error) {
	records := make(chan store.FileRecord, queueDepth)
	g, gctx := errgroup.WithContext(ctx)

	var hashers sync.WaitGroup
	for range ix.hashWorkers {
		hashers.Add(1)
		g.Go(func() error {
			defer hashers.Done()
			for c := range candidates {
				rec, err := ix.record(c)
				if err != nil {
					ix.log.Warn("skipping file", "path", c.path, "err", err)
					continue
				}
				select {
				case records <- rec:
				case <-gctx.Done():
					return gctx.Err()
				}
			}
			return nil
		})
	}
	go func() {
		hashers.Wait()
		close(records)
	}()

	var total store.UpsertResult
	g.Go(func() error {
		batch := make([]store.FileRecord, 0, batchSize)
		ticker := time.NewTicker(batchInterval)
		defer ticker.Stop()
		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			res, err := ix.store.UpsertFiles(gctx, batch)
			if err != nil {
				return err
			}
			total.NewImages += res.NewImages
			total.Reused += res.Reused
			batch = batch[:0]
			if res.NewImages > 0 && ix.onNewJobs != nil {
				ix.onNewJobs()
			}
			return nil
		}
		for {
			select {
			case rec, ok := <-records:
				if !ok {
					return flush()
				}
				batch = append(batch, rec)
				if len(batch) >= batchSize {
					if err := flush(); err != nil {
						return err
					}
				}
			case <-ticker.C:
				if err := flush(); err != nil {
					return err
				}
			case <-gctx.Done():
				return gctx.Err()
			}
		}
	})

	err := g.Wait()
	return total, err
}

func (ix *Indexer) record(c candidate) (store.FileRecord, error) {
	hash, err := hashFile(c.path)
	if err != nil {
		return store.FileRecord{}, err
	}
	return store.FileRecord{
		Path:    c.path,
		Root:    c.root,
		Dir:     filepath.Dir(c.path),
		Size:    c.size,
		Mtime:   c.mtime.UnixNano(),
		Hash:    hash,
		TakenAt: takenAt(c.path, c.mtime),
		Lane:    c.lane,
	}, nil
}

func hashFile(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	h := xxhash.New()
	if _, err := io.Copy(h, f); err != nil {
		return 0, err
	}
	return h.Sum64(), nil
}
