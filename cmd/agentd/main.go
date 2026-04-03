package main

import (
	"log/slog"
	"os"

	"go-agent/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("bootstrap complete", "addr", cfg.Addr, "data_dir", cfg.DataDir)
}
