package main

import (
	"flag"
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
	go runScanner(store, cfg.Libraries, thumbCh, rescanCh)
	go runThumbnailer(store, thumbCh)

	runServer(cfg, store, rescanCh)
}

