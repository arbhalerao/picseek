package indexer

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/arbhalerao/picseek/internal/store"
)

const (
	settleDelay  = 300 * time.Millisecond
	flushEvery   = 100 * time.Millisecond
	maxPendingAt = 4096
)

type Watcher struct {
	ix      *Indexer
	fs      *fsnotify.Watcher
	mu      sync.Mutex
	roots   map[string]struct{}
	dirs    map[string]struct{}
	pending map[string]time.Time
}

func NewWatcher(ix *Indexer) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		ix:      ix,
		fs:      w,
		roots:   make(map[string]struct{}),
		dirs:    make(map[string]struct{}),
		pending: make(map[string]time.Time),
	}, nil
}

func (w *Watcher) Close() error {
	return w.fs.Close()
}

func (w *Watcher) AddRoot(root string) error {
	w.mu.Lock()
	w.roots[root] = struct{}{}
	w.mu.Unlock()
	return w.addTree(root)
}

func (w *Watcher) RemoveRoot(root string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.roots, root)
	for dir := range w.dirs {
		if dir == root || strings.HasPrefix(dir, root+string(filepath.Separator)) {
			_ = w.fs.Remove(dir)
			delete(w.dirs, dir)
		}
	}
}

func (w *Watcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(flushEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-w.fs.Errors:
			if !ok {
				return nil
			}
			w.ix.log.Warn("watcher error", "err", err)
		case ev, ok := <-w.fs.Events:
			if !ok {
				return nil
			}
			w.handle(ctx, ev)
		case <-ticker.C:
			w.flush(ctx)
		}
	}
}

func (w *Watcher) handle(ctx context.Context, ev fsnotify.Event) {
	root, ok := w.rootOf(ev.Name)
	if !ok {
		return
	}
	switch {
	case ev.Has(fsnotify.Create):
		info, err := os.Lstat(ev.Name)
		if err != nil {
			return
		}
		if info.IsDir() {
			if w.ix.filter.Excluded(ev.Name) {
				return
			}
			if err := w.addTree(ev.Name); err != nil {
				w.ix.log.Warn("watch new directory", "dir", ev.Name, "err", err)
			}
			if err := w.ix.IndexTree(ctx, root, ev.Name, store.LaneLive); err != nil {
				w.ix.log.Warn("index new directory", "dir", ev.Name, "err", err)
			}
			return
		}
		w.touch(ev.Name)
	case ev.Has(fsnotify.Write):
		w.touch(ev.Name)
	case ev.Has(fsnotify.Remove), ev.Has(fsnotify.Rename):
		w.forget(ctx, ev.Name)
	}
}

func (w *Watcher) touch(path string) {
	if !w.ix.filter.Image(path) || w.ix.filter.Excluded(path) {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) < maxPendingAt {
		w.pending[path] = time.Now()
	}
}

func (w *Watcher) forget(ctx context.Context, path string) {
	w.mu.Lock()
	delete(w.pending, path)
	_, wasDir := w.dirs[path]
	if wasDir {
		for dir := range w.dirs {
			if dir == path || strings.HasPrefix(dir, path+string(filepath.Separator)) {
				_ = w.fs.Remove(dir)
				delete(w.dirs, dir)
			}
		}
	}
	w.mu.Unlock()

	var err error
	if wasDir {
		err = w.ix.store.RemovePathsUnder(ctx, path)
	} else {
		err = w.ix.store.RemovePaths(ctx, []string{path})
	}
	if err != nil {
		w.ix.log.Warn("remove path", "path", path, "err", err)
	}
}

func (w *Watcher) flush(ctx context.Context) {
	now := time.Now()
	var ready []string
	w.mu.Lock()
	for path, at := range w.pending {
		if now.Sub(at) >= settleDelay {
			ready = append(ready, path)
			delete(w.pending, path)
		}
	}
	w.mu.Unlock()
	if len(ready) == 0 {
		return
	}

	files := make([]candidate, 0, len(ready))
	for _, path := range ready {
		root, ok := w.rootOf(path)
		if !ok {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if w.ix.filter.TooLarge(info.Size()) {
			w.ix.skipped.Add(1)
			continue
		}
		files = append(files, candidate{path: path, root: root, size: info.Size(), mtime: info.ModTime(), lane: store.LaneLive})
	}
	if err := w.ix.IndexFiles(ctx, files); err != nil {
		w.ix.log.Warn("index changed files", "count", len(files), "err", err)
	}
}

func (w *Watcher) addTree(dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if path != dir && w.ix.filter.Excluded(path) {
			return filepath.SkipDir
		}
		if err := w.fs.Add(path); err != nil {
			return err
		}
		w.mu.Lock()
		w.dirs[path] = struct{}{}
		w.mu.Unlock()
		return nil
	})
}

func (w *Watcher) rootOf(path string) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	best := ""
	for root := range w.roots {
		if (path == root || strings.HasPrefix(path, root+string(filepath.Separator))) && len(root) > len(best) {
			best = root
		}
	}
	return best, best != ""
}
