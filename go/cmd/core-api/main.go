// Command core-api serves the FrontDesk AI platform API (EPIC-004): tenant
// onboarding, DIDs, call projections, appointments, structured messages,
// usage/billing preview and the answering-mode toggle.
//
// Configuration comes from the environment:
//
//	CORE_API_ADDR    listen address (default ":8080")
//	DATABASE_URL     Postgres DSN; empty selects the in-memory store
//	                 (local development and tests only)
//
// Shutdown is graceful: SIGINT/SIGTERM stops accepting connections and
// drains in-flight requests for up to 10 seconds.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maarkn/frontdesk/internal/api"
	"github.com/maarkn/frontdesk/internal/billing"
	"github.com/maarkn/frontdesk/internal/domain"
	"github.com/maarkn/frontdesk/internal/store"
)

// shutdownGrace is how long in-flight requests may drain on shutdown.
const shutdownGrace = 10 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("core-api exited", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := os.Getenv("CORE_API_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	cfg := api.Config{
		// MVP fakes: the LLM site extractor and Stripe live behind
		// interfaces; production implementations swap in here.
		Extractor: &domain.StaticExtractor{},
		Billing:   &billing.FakeStripe{},
	}

	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		pool, err := pgxpool.New(ctx, databaseURL)
		if err != nil {
			return fmt.Errorf("connect postgres: %w", err)
		}
		defer pool.Close()
		pg := store.NewPG(pool)
		cfg.Tenants, cfg.DIDs, cfg.Calls, cfg.Appointments, cfg.Messages = pg, pg, pg, pg, pg
		slog.Info("store: postgres (RLS enforced)")
	} else {
		mem := store.NewMem()
		cfg.Tenants, cfg.DIDs, cfg.Calls, cfg.Appointments, cfg.Messages = mem, mem, mem, mem, mem
		slog.Warn("store: in-memory (DATABASE_URL empty; data is volatile)")
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           api.New(cfg).Router(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	slog.Info("core-api listening", "addr", addr)

	select {
	case err := <-errCh:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	slog.Info("shutting down", "grace", shutdownGrace)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errCh; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
