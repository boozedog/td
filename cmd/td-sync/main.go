package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/marcus/td/internal/api"
	"github.com/marcus/td/internal/serverdb"
)

func main() {
	cfg := api.LoadConfig()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	store, err := serverdb.Open(cfg.ServerDBPath)
	if err != nil {
		slog.Error("open server db", "err", err)
		os.Exit(1)
	}
	defer store.Close()

	srv, err := api.NewServer(cfg, store)
	if err != nil {
		slog.Error("create server", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Start(); err != nil {
		slog.Error("start server", "err", err)
		os.Exit(1)
	}
	slog.Info("server started", "addr", cfg.ListenAddr)

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown", "err", err)
	}
}
