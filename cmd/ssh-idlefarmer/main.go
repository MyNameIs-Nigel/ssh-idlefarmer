package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/ssh"

	"github.com/mynameis-nigel/ssh-idlefarmer/internal/config"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/content"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/game"
	applog "github.com/mynameis-nigel/ssh-idlefarmer/internal/log"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/server"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "error", err)
		os.Exit(1)
	}

	logger := applog.New(cfg.LogLevel, cfg.LogFormat)
	slog.SetDefault(logger)

	gameContent, err := content.Load(cfg.DataDir)
	if err != nil {
		logger.Error("content load failed", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	st, err := store.Open(ctx, cfg.DBPath)
	cancel()
	if err != nil {
		logger.Error("store open failed", "path", cfg.DBPath, "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := st.Close(); err != nil {
			logger.Error("store close failed", "error", err)
		}
	}()

	games := game.NewManager(st, gameContent, logger, cfg.AutosaveInterval, game.Policy(cfg.SessionPolicy))
	// Fulfill the Task 1 shutdown hook: on SIGTERM/SIGINT every active save
	// is flushed before the process exits, so redeploys lose nothing.
	server.RegisterShutdownHook(games.Shutdown)

	srv, err := server.New(cfg, logger, games)
	if err != nil {
		logger.Error("server init failed", "error", err)
		os.Exit(1)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			logger.Error("listen failed", "error", err)
			done <- syscall.SIGTERM
		}
	}()

	sig := <-done
	logger.Info("shutdown signal received", "signal", sig.String())

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()

	if err := server.RunShutdownHooks(shutdownCtx); err != nil {
		logger.Error("shutdown hook failed", "error", err)
	}

	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		logger.Error("shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("shutdown complete")
}
