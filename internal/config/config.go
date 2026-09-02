package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Index    Index    `toml:"index"`
	OCR      OCR      `toml:"ocr"`
	Search   Search   `toml:"search"`
	Services Services `toml:"services"`
}

type Index struct {
	Folders    []string `toml:"folders"`
	Exclude    []string `toml:"exclude"`
	Extensions []string `toml:"extensions"`
	MaxFileMB  int      `toml:"max_file_mb"`
}

type OCR struct {
	Workers          int `toml:"workers"`
	ThreadsPerWorker int `toml:"threads_per_worker"`
}

type Search struct {
	DefaultLimit   int     `toml:"default_limit"`
	SemanticMin    float64 `toml:"semantic_min"`
	SemanticMargin float64 `toml:"semantic_margin"`
	VisualMin      float64 `toml:"visual_min"`
	VisualMargin   float64 `toml:"visual_margin"`
}

type Services struct {
	SocketDir string `toml:"socket_dir"`
}

func Default() Config {
	return Config{
		Index: Index{
			Folders:    []string{"~/Pictures/Screenshots"},
			Exclude:    []string{"**/node_modules", "**/.*"},
			Extensions: []string{"png", "jpg", "jpeg", "webp", "bmp", "tiff", "gif"},
			MaxFileMB:  50,
		},
		OCR: OCR{
			Workers:          4,
			ThreadsPerWorker: 2,
		},
		Search: Search{
			DefaultLimit:   20,
			SemanticMin:    0.45,
			SemanticMargin: 0.15,
			VisualMin:      0.22,
			VisualMargin:   0.05,
		},
		Services: Services{
			SocketDir: filepath.Join(runtimeDir(), "picseek"),
		},
	}
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "picseek", "config.toml"), nil
}

func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return Config{}, err
	default:
		if err := toml.Unmarshal(data, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	if err := cfg.resolve(); err != nil {
		return Config{}, err
	}
	return cfg, cfg.validate()
}

func (c Config) SocketPath(service string) string {
	return filepath.Join(c.Services.SocketDir, service+".sock")
}

func (c *Config) resolve() error {
	for i, folder := range c.Index.Folders {
		p, err := expandPath(folder)
		if err != nil {
			return err
		}
		c.Index.Folders[i] = p
	}
	p, err := expandPath(c.Services.SocketDir)
	if err != nil {
		return err
	}
	c.Services.SocketDir = p
	return nil
}

func (c Config) validate() error {
	var errs []error
	if c.OCR.Workers < 1 {
		errs = append(errs, errors.New("ocr.workers must be at least 1"))
	}
	if c.OCR.ThreadsPerWorker < 1 {
		errs = append(errs, errors.New("ocr.threads_per_worker must be at least 1"))
	}
	if c.Index.MaxFileMB < 1 {
		errs = append(errs, errors.New("index.max_file_mb must be at least 1"))
	}
	if c.Search.DefaultLimit < 1 {
		errs = append(errs, errors.New("search.default_limit must be at least 1"))
	}
	for name, v := range map[string]float64{
		"search.semantic_min":    c.Search.SemanticMin,
		"search.semantic_margin": c.Search.SemanticMargin,
		"search.visual_min":      c.Search.VisualMin,
		"search.visual_margin":   c.Search.VisualMargin,
	} {
		if v < 0 || v > 1 {
			errs = append(errs, fmt.Errorf("%s must be between 0 and 1", name))
		}
	}
	if c.Services.SocketDir == "" {
		errs = append(errs, errors.New("services.socket_dir must not be empty"))
	}
	return errors.Join(errs...)
}

func expandPath(p string) (string, error) {
	p = os.ExpandEnv(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return filepath.Clean(p), nil
}

func runtimeDir() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("picseek-%d", os.Getuid()))
}
