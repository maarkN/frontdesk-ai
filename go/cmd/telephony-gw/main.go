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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/maarkn/frontdesk/internal/asterisk"
	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/obs"
	"github.com/maarkn/frontdesk/internal/telnyx"
)

// serviceName identifies this binary in logs and telemetry resources.
const serviceName = "telephony-gw"

func main() {
	logger := obs.NewLogger(obs.WithLogService(serviceName, os.Getenv("SERVICE_VERSION")))
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

	// MediaBackend selects the carrier/media implementation (ADR-009):
	// "telnyx" (MVP default) or "asterisk" (own media backend, v2 margin
	// lever). Env: MEDIA_BACKEND.
	MediaBackend string
	// Asterisk configures the ARI backend (used when MediaBackend is
	// "asterisk"). Envs: ASTERISK_ARI_URL, ASTERISK_ARI_WS_URL (optional,
	// derived from URL when empty), ASTERISK_APP, ARI_USERNAME,
	// ARI_PASSWORD (same names the asterisk/ container entrypoint reads),
	// ASTERISK_TRANSFER_CONTEXT, ASTERISK_EXTERNAL_MEDIA_ADDR (this
	// gateway's External Media UDP socket as reachable FROM Asterisk).
	Asterisk asterisk.Config
	// MediaListenAddr is the local UDP bind of the External Media server
	// (asterisk backend only). Env: MEDIA_LISTEN_ADDR.
	MediaListenAddr string
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

	cfg.MediaBackend = envOr(getenv, "MEDIA_BACKEND", backendTelnyx)
	switch cfg.MediaBackend {
	case backendTelnyx:
	case backendAsterisk:
		cfg.Asterisk = asterisk.Config{
			URL:          getenv("ASTERISK_ARI_URL"),
			WebsocketURL: getenv("ASTERISK_ARI_WS_URL"),
			// Default matches extensions.conf's ACTIVE_APP global; each
			// blue/green deploy color overrides with its own app name.
			Application: envOr(getenv, "ASTERISK_APP", "frontdesk-v1"),
			// ARI_USERNAME/ARI_PASSWORD deliberately mirror the env names
			// the asterisk/docker-entrypoint.sh materializes into
			// ari_secret.conf — one pair of values on both sides.
			Username:          envOr(getenv, "ARI_USERNAME", "frontdesk"),
			Password:          getenv("ARI_PASSWORD"),
			TransferContext:   envOr(getenv, "ASTERISK_TRANSFER_CONTEXT", "transfer-owner"),
			ExternalMediaAddr: getenv("ASTERISK_EXTERNAL_MEDIA_ADDR"),
		}
		cfg.MediaListenAddr = envOr(getenv, "MEDIA_LISTEN_ADDR", ":4000")
		if cfg.Asterisk.URL == "" {
			return config{}, fmt.Errorf("MEDIA_BACKEND=asterisk requires ASTERISK_ARI_URL")
		}
	default:
		return config{}, fmt.Errorf("unknown MEDIA_BACKEND %q (want %q or %q)",
			cfg.MediaBackend, backendTelnyx, backendAsterisk)
	}
	return cfg, nil
}

// Media backend names accepted in MEDIA_BACKEND (ADR-009).
const (
	backendTelnyx   = "telnyx"
	backendAsterisk = "asterisk"
)

func envOr(getenv func(string) string, key, def string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return def
}

// run wires the gateway and blocks until ctx is done, then drains.
func run(ctx context.Context, cfg config, logger *slog.Logger) error {
	// Telemetry (EPIC-010): OTLP gRPC traces+metrics when
	// OTEL_EXPORTER_OTLP_ENDPOINT is set; disabled (no-op) otherwise.
	tel, err := obs.Setup(ctx, obs.WithServiceName(serviceName), obs.WithInsecure())
	if err != nil {
		return fmt.Errorf("telemetry setup: %w", err)
	}
	defer func() {
		if err := tel.Shutdown(context.Background()); err != nil {
			logger.Warn("telemetry shutdown", "err", err)
		}
	}()
	metrics, err := obs.NewMetrics(tel.Meter(serviceName))
	if err != nil {
		return fmt.Errorf("telemetry metrics: %w", err)
	}

	publisher, closeBus, err := newPublisher(cfg)
	if err != nil {
		return err
	}
	defer closeBus()

	// Media backend selection (ADR-009): both implement the carrier-neutral
	// telnyx.CallControl surface, so the gateway below is identical.
	var callControl telnyx.CallControl
	mediaErrc := make(chan error, 1)
	switch cfg.MediaBackend {
	case backendAsterisk:
		backend, err := asterisk.New(ctx, cfg.Asterisk, cfg.MediaListenAddr)
		if err != nil {
			return fmt.Errorf("asterisk backend: %w", err)
		}
		defer func() {
			if cerr := backend.Close(); cerr != nil {
				logger.Warn("asterisk backend close", "err", cerr)
			}
		}()
		// The External Media RTP server is the audio path of every call:
		// if its loop dies while the gateway is up, that is fatal.
		go func() {
			if serr := backend.Media.Serve(ctx); serr != nil && ctx.Err() == nil {
				mediaErrc <- serr
			}
		}()
		callControl = backend.Control
		logger.Info("media backend: asterisk",
			"ari", cfg.Asterisk.URL,
			"app", cfg.Asterisk.Application,
			"externalMediaAddr", cfg.Asterisk.ExternalMediaAddr,
			"mediaListenAddr", cfg.MediaListenAddr)
	default:
		callControl = telnyx.NewClient(cfg.TelnyxAPIKey)
		logger.Info("media backend: telnyx")
	}

	gw := &gateway{
		cfg:      cfg,
		logger:   logger,
		events:   publisher,
		control:  callControl,
		sessions: newSessionRegistry(),
		metrics:  metrics,
	}

	// ready flips to false when shutdown starts, so the k8s readiness probe
	// (/readyz) removes the pod from the Service while active calls drain.
	var ready atomic.Bool
	ready.Store(true)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !ready.Load() {
			http.Error(w, "draining", http.StatusServiceUnavailable)
			return
		}
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
	case err := <-mediaErrc:
		return fmt.Errorf("external media server: %w", err)
	case <-ctx.Done():
	}

	// Graceful shutdown: stop accepting webhooks, then DRAIN active calls —
	// a deploy must never hang up on a caller mid-sentence.
	ready.Store(false)
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
