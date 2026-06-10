package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
)

func main() {
	configPath := flag.String("config", "", "path to config file")
	verbose := flag.Bool("verbose", false, "enable debug logging")
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	cfg, err := loadConfig(*configPath)
	if err != nil {
		if errors.Is(err, ErrNoConfig) {
			if werr := os.WriteFile("mangoo.toml", exampleConfig, 0644); werr != nil {
				slog.Error("could not write mangoo.toml", "err", werr)
				os.Exit(1)
			}
			fmt.Println("No config found. Created mangoo.toml — edit it and run again.")
			os.Exit(0)
		}
		slog.Error("config error", "err", err)
		os.Exit(1)
	}

	store, err := openStore(cfg.DBPath)
	if err != nil {
		slog.Error("db error", "err", err)
		os.Exit(1)
	}

	thumbCh := make(chan struct{}, 2)
	rescanCh := make(chan struct{}, 1)
	if cfg.Thumbnailer {
		go runThumbnailer(store, thumbCh)
	}
	go runScanner(store, cfg.Libraries, thumbCh, rescanCh)

	runServer(cfg, store, rescanCh)
}

