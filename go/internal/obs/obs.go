// Package obs is the observability layer of the FrontDesk Go services
// (EPIC-010): structured JSON logging (slog), the OpenTelemetry SDK setup
// (traces + metrics over OTLP gRPC) and the product metric adapters that
// implement the metric interfaces already defined by the consumer packages
// (notify.Metrics, degrade ladder/breaker hooks, turn.Hooks).
//
// Design rules (ADR-008 / uber-go): configuration through functional options,
// zero-value-safe fallbacks (no endpoint → telemetry disabled, logging still
// works), graceful shutdown owned by the caller, and instrumentation injected
// into existing hook points instead of scattering OTel calls through the
// domain packages.
package obs

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/maarkn/frontdesk/internal/tenantctx"
)

// LoggerOption configures NewLogger.
type LoggerOption func(*loggerConfig)

type loggerConfig struct {
	writer  io.Writer
	level   slog.Leveler
	service string
	version string
}

// WithLogWriter sets the log destination (default os.Stderr).
func WithLogWriter(w io.Writer) LoggerOption {
	return func(c *loggerConfig) { c.writer = w }
}

// WithLogLevel sets the minimum level, overriding the LOG_LEVEL env var.
func WithLogLevel(l slog.Leveler) LoggerOption {
	return func(c *loggerConfig) { c.level = l }
}

// WithLogService stamps service/version attributes on every record.
func WithLogService(name, version string) LoggerOption {
	return func(c *loggerConfig) {
		c.service = name
		c.version = version
	}
}

// NewLogger returns a JSON slog.Logger. The level comes from LOG_LEVEL
// (debug|info|warn|error, default info) unless WithLogLevel overrides it.
// Records carry service/version attributes when configured, and the tenant
// attribute whenever the log call's context carries one (tenantctx, ADR-005).
func NewLogger(opts ...LoggerOption) *slog.Logger {
	cfg := loggerConfig{writer: os.Stderr, level: LevelFromEnv()}
	for _, o := range opts {
		o(&cfg)
	}
	var h slog.Handler = slog.NewJSONHandler(cfg.writer, &slog.HandlerOptions{Level: cfg.level})
	h = tenantHandler{inner: h}
	logger := slog.New(h)
	if cfg.service != "" {
		attrs := []any{slog.String("service", cfg.service)}
		if cfg.version != "" {
			attrs = append(attrs, slog.String("version", cfg.version))
		}
		logger = logger.With(attrs...)
	}
	return logger
}

// LevelFromEnv parses LOG_LEVEL (debug|info|warn|error, case-insensitive)
// into a slog.Level, defaulting to info on empty or unknown values.
func LevelFromEnv() slog.Level {
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// tenantHandler decorates records with the tenant id carried by the context.
// It never fails when the tenant is absent — logs outside a call (startup,
// shutdown) simply have no tenant attribute.
type tenantHandler struct {
	inner slog.Handler
}

var _ slog.Handler = tenantHandler{}

// Enabled implements slog.Handler.
func (h tenantHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle implements slog.Handler.
func (h tenantHandler) Handle(ctx context.Context, r slog.Record) error {
	if id, err := tenantctx.FromContext(ctx); err == nil {
		r.AddAttrs(slog.String("tenant", id))
	}
	return h.inner.Handle(ctx, r)
}

// WithAttrs implements slog.Handler, keeping the tenant decoration.
func (h tenantHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return tenantHandler{inner: h.inner.WithAttrs(attrs)}
}

// WithGroup implements slog.Handler, keeping the tenant decoration.
func (h tenantHandler) WithGroup(name string) slog.Handler {
	return tenantHandler{inner: h.inner.WithGroup(name)}
}
