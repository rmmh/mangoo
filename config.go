package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Port        int      `toml:"port"`
	DBPath      string   `toml:"db_path"`
	Thumbnailer bool     `toml:"thumbnailer"`
	Libraries   []string `toml:"libraries"`
}

var ErrNoConfig = errors.New("no config file found")

func loadConfig(path string) (*Config, error) {
	cfg := &Config{
		Port:        8080,
		DBPath:      "mangoo.db",
		Thumbnailer: true,
	}

	if path == "" {
		candidates := []string{
			"mangoo.toml",
		}
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			candidates = append(candidates, filepath.Join(xdg, "mangoo", "config.toml"))
		}
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates, filepath.Join(home, ".config", "mangoo", "config.toml"))
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				path = c
				break
			}
		}
	}

	if path == "" {
		return nil, ErrNoConfig
	}

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}
	if len(cfg.Libraries) == 0 {
		return nil, errors.New("config: at least one library path required")
	}
	return cfg, nil
}
