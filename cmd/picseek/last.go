package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	seekdv1 "github.com/arbhalerao/picseek/gen/go/picseek/seekd/v1"
)

func lastPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "picseek", "last-search.json"), nil
}

func saveLast(hits []*seekdv1.Hit) error {
	p, err := lastPath()
	if err != nil {
		return err
	}
	paths := make([]string, len(hits))
	for i, h := range hits {
		paths[i] = h.GetPath()
	}
	data, err := json.Marshal(paths)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

func loadLast() ([]string, error) {
	p, err := lastPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var paths []string
	return paths, json.Unmarshal(data, &paths)
}
