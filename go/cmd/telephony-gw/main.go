// Command telephony-gw is the telephony gateway of FrontDesk AI (EPIC-002):
// it terminates Telnyx call control + media, runs one turn.Machine per call,
// and emits business events to NATS JetStream. This binary is the wiring —
// all behavior lives in internal/{turn,media,degrade,telnyx,audiobank}.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/telnyx"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		logger.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("telephony-gw exited with error", "err", err)
		os.Exit(1)
	}
}

// config is everything the gateway takes from the environment.
type config struct {
	// HTTPAddr is the webhook/health listen address.
	HTTPAddr string
	// NATSURL enables JetStream publishing when non-empty.
	NATSURL string
	// EventSubjectPrefix prefixes the per-call event subjects.
	EventSubjectPrefix string
	// TelnyxAPIKey authenticates call-control commands.
	TelnyxAPIKey string
	// TenantByDID maps inbound DIDs to tenant ids ("+1514...=tenant-a,...").
	// Unknown DID → hangup (ADR-005). CRUD config comes from core-api later;
	// static env config is the EPIC-002 starting point.
	TenantByDID map[string]string
	// OwnerPhone is the product failover target: the call must ring there,
	// never die in silence (ADR-006 §5).
	OwnerPhone string
	// DrainTimeout bounds how long shutdown waits for active calls.
	DrainTimeout time.Duration
}

// loadConfig reads the environment through getenv (injectable for tests).
func loadConfig(getenv func(string) string) (config, error) {
	cfg := config{
		HTTPAddr:           envOr(getenv, "HTTP_ADDR", ":8080"),
		NATSURL:            getenv("NATS_URL"),
		EventSubjectPrefix: envOr(getenv, "EVENT_SUBJECT_PREFIX", "frontdesk.events"),
		TelnyxAPIKey:       getenv("TELNYX_API_KEY"),
		OwnerPhone:         getenv("OWNER_PHONE"),
		TenantByDID:        map[string]string{},
		DrainTimeout:       30 * time.Second,
	}
	if raw := getenv("DRAIN_TIMEOUT"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return config{}, fmt.Errorf("parse DRAIN_TIMEOUT: %w", err)
		}
		cfg.DrainTimeout = d
	}
	if raw := getenv("TENANT_DIDS"); raw != "" {
		for _, pair := range strings.Split(raw, ",") {
			did, tenant, ok := strings.Cut(strings.TrimSpace(pair), "=")
			if !ok || did == "" || tenant == "" {
				return config{}, fmt.Errorf("malformed TENANT_DIDS entry %q", pair)
			}
			cfg.TenantByDID[did] = tenant
		}
	}
	return cfg, nil
}

func envOr(getenv func(string) string, key, def string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return def
}

// run wires the gateway and blocks until ctx is done, then drains.
func run(ctx context.Context, cfg config, logger *slog.Logger) error {
	publisher, closeBus, err := newPublisher(cfg)
	if err != nil {
		return err
	}
	defer closeBus()

	callControl := telnyx.NewClient(cfg.TelnyxAPIKey)
	gw := &gateway{
		cfg:      cfg,
		logger:   logger,
		events:   publisher,
		control:  callControl,
		sessions: newSessionRegistry(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /telnyx/webhook", gw.handleWebhook)
	// The bidirectional media WS attaches per call here. The transport
	// implementation lands with the Telnyx integration test bench; until
	// then the route documents the contract.
	mux.HandleFunc("GET /telnyx/media/{callID}", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "media transport not yet wired", http.StatusNotImplemented)
	})

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	errc := make(chan error, 1)
	go func() {
		logger.Info("telephony-gw listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	// Graceful shutdown: stop accepting webhooks, then DRAIN active calls —
	// a deploy must never hang up on a caller mid-sentence.
	logger.Info("shutting down: draining active calls", "timeout", cfg.DrainTimeout)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.DrainTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("http shutdown", "err", err)
	}
	if drained := gw.sessions.drain(shutdownCtx); !drained {
		logger.Warn("drain timeout: remaining calls were cancelled",
			"remaining", gw.sessions.active())
	}
	return ctx.Err()
}

// newPublisher returns the event publisher: JetStream when NATS_URL is set,
// otherwise an in-memory bus (local development).
func newPublisher(cfg config) (event.Publisher, func(), error) {
	if cfg.NATSURL == "" {
		return &event.MemoryBus{}, func() {}, nil
	}
	nc, err := nats.Connect(cfg.NATSURL, nats.Name("telephony-gw"))
	if err != nil {
		return nil, nil, fmt.Errorf("connect nats: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("jetstream: %w", err)
	}
	return event.NewJetStreamPublisher(js, cfg.EventSubjectPrefix), nc.Close, nil
}
