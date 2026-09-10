// Command gateway starts the Go AI Gateway HTTP server.
//
// Configuration is read from environment variables (see .env.example).
// The process shuts down gracefully on SIGINT/SIGTERM, draining in-flight
// requests for up to SHUTDOWN_TIMEOUT.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/ninjadiego/go-ai-gateway/internal/config"
	"github.com/ninjadiego/go-ai-gateway/internal/database"
	"github.com/ninjadiego/go-ai-gateway/internal/server"
)

func main() {
	if err := run(); err != nil {
		log.Fatal().Err(err).Msg("gateway exited with error")
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	setupLogger(cfg)

	db, err := database.Open(cfg.DB)
	if err != nil {
		return err
	}
	defer db.Close()

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           server.New(cfg, db).Router(),
		ReadHeaderTimeout: 10 * time.Second,
		// Streaming responses can be long-lived, so no WriteTimeout here;
		// the upstream timeout bounds each proxied call instead.
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info().Int("port", cfg.Port).Str("env", cfg.Env).Msg("gateway listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info().Msg("shutdown signal received, draining connections")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	log.Info().Msg("gateway stopped")
	return nil
}

// setupLogger configures zerolog: JSON in production, pretty console in dev.
func setupLogger(cfg *config.Config) {
	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)
	zerolog.TimeFieldFormat = time.RFC3339

	if cfg.Env == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen})
	}
}
