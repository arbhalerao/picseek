package indexer

import (
	"path/filepath"
	"strings"
)

type Filter struct {
	extensions map[string]struct{}
	exclude    []string
	maxBytes   int64
}

func NewFilter(extensions, exclude []string, maxFileMB int) Filter {
	exts := make(map[string]struct{}, len(extensions))
	for _, e := range extensions {
		exts["."+strings.TrimPrefix(strings.ToLower(e), ".")] = struct{}{}
	}
	return Filter{extensions: exts, exclude: exclude, maxBytes: int64(maxFileMB) << 20}
}

func (f Filter) Image(path string) bool {
	_, ok := f.extensions[strings.ToLower(filepath.Ext(path))]
	return ok
}

func (f Filter) TooLarge(size int64) bool {
	return size > f.maxBytes
}

func (f Filter) Excluded(path string) bool {
	name := filepath.Base(path)
	for _, pattern := range f.exclude {
		if component, ok := strings.CutPrefix(pattern, "**/"); ok {
			if matched, _ := filepath.Match(component, name); matched {
				return true
			}
			continue
		}
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
	}
	return false
}
