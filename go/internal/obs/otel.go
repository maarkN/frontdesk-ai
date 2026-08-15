package obs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// shutdownTimeout bounds the final flush of each provider.
const shutdownTimeout = 5 * time.Second

// Telemetry owns the OTel providers of one process. The zero value is a
// disabled telemetry: Tracer/Meter return no-op implementations and Shutdown
// is a no-op, so callers wire it unconditionally.
type Telemetry struct {
	tp *sdktrace.TracerProvider
	mp *sdkmetric.MeterProvider
}

// Option configures Setup.
type Option func(*setupConfig)

type setupConfig struct {
	serviceName    string
	serviceVersion string
	endpoint       string
	insecure       bool
	traceExporter  sdktrace.SpanExporter
	metricReader   sdkmetric.Reader
	setGlobal      bool
}

// WithServiceName sets resource service.name (mandatory for real exports).
func WithServiceName(name string) Option {
	return func(c *setupConfig) { c.serviceName = name }
}

// WithServiceVersion sets resource service.version.
func WithServiceVersion(v string) Option {
	return func(c *setupConfig) { c.serviceVersion = v }
}

// WithOTLPEndpoint sets the OTLP gRPC endpoint (host:port), overriding
// OTEL_EXPORTER_OTLP_ENDPOINT. Empty disables exporting entirely.
func WithOTLPEndpoint(ep string) Option {
	return func(c *setupConfig) { c.endpoint = ep }
}

// WithInsecure disables TLS on the OTLP connection (local collector).
func WithInsecure() Option {
	return func(c *setupConfig) { c.insecure = true }
}

// WithTraceExporter injects a span exporter (tests: tracetest.InMemoryExporter).
func WithTraceExporter(exp sdktrace.SpanExporter) Option {
	return func(c *setupConfig) { c.traceExporter = exp }
}

// WithMetricReader injects a metric reader (tests: sdkmetric.ManualReader).
func WithMetricReader(r sdkmetric.Reader) Option {
	return func(c *setupConfig) { c.metricReader = r }
}

// WithoutGlobal keeps the providers off the otel global registry (tests).
func WithoutGlobal() Option {
	return func(c *setupConfig) { c.setGlobal = false }
}

// Setup builds the process telemetry: a resource with service.name/version,
// a TracerProvider and a MeterProvider exporting over OTLP gRPC. When no
// endpoint is configured (option or OTEL_EXPORTER_OTLP_ENDPOINT) and no
// exporter/reader was injected, it returns a disabled Telemetry — services
// run fine without a collector.
func Setup(ctx context.Context, opts ...Option) (*Telemetry, error) {
	cfg := setupConfig{
		endpoint:  os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		setGlobal: true,
	}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.serviceVersion == "" {
		cfg.serviceVersion = os.Getenv("SERVICE_VERSION")
	}

	if cfg.endpoint == "" && cfg.traceExporter == nil && cfg.metricReader == nil {
		return &Telemetry{}, nil
	}

	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(cfg.serviceName),
		semconv.ServiceVersion(cfg.serviceVersion),
	))
	if err != nil {
		return nil, fmt.Errorf("obs: build resource: %w", err)
	}

	t := &Telemetry{}

	traceExp := cfg.traceExporter
	if traceExp == nil {
		traceExp, err = newTraceExporter(ctx, cfg)
		if err != nil {
			return nil, err
		}
	}
	t.tp = sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExp),
	)

	reader := cfg.metricReader
	if reader == nil {
		metricExp, err := newMetricExporter(ctx, cfg)
		if err != nil {
			// Roll back the already-built tracer provider before failing.
			shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
			_ = t.tp.Shutdown(shutdownCtx)
			return nil, err
		}
		reader = sdkmetric.NewPeriodicReader(metricExp)
	}
	t.mp = sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
	)

	if cfg.setGlobal {
		otel.SetTracerProvider(t.tp)
		otel.SetMeterProvider(t.mp)
	}
	return t, nil
}

// newTraceExporter dials the OTLP gRPC span exporter.
func newTraceExporter(ctx context.Context, cfg setupConfig) (sdktrace.SpanExporter, error) {
	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.endpoint)}
	if cfg.insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	exp, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("obs: otlp trace exporter: %w", err)
	}
	return exp, nil
}

// newMetricExporter dials the OTLP gRPC metric exporter.
func newMetricExporter(ctx context.Context, cfg setupConfig) (sdkmetric.Exporter, error) {
	opts := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(cfg.endpoint)}
	if cfg.insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}
	exp, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("obs: otlp metric exporter: %w", err)
	}
	return exp, nil
}

// Enabled reports whether real providers were built.
func (t *Telemetry) Enabled() bool {
	return t != nil && (t.tp != nil || t.mp != nil)
}

// Tracer returns a tracer, no-op when telemetry is disabled.
func (t *Telemetry) Tracer(name string) trace.Tracer {
	if t == nil || t.tp == nil {
		return noop.NewTracerProvider().Tracer(name)
	}
	return t.tp.Tracer(name)
}

// Meter returns a meter, no-op when telemetry is disabled.
func (t *Telemetry) Meter(name string) metric.Meter {
	if t == nil || t.mp == nil {
		return metricnoop.NewMeterProvider().Meter(name)
	}
	return t.mp.Meter(name)
}

// ForceFlush drains pending spans and metrics without stopping the providers.
func (t *Telemetry) ForceFlush(ctx context.Context) error {
	if t == nil {
		return nil
	}
	var errs []error
	if t.tp != nil {
		if err := t.tp.ForceFlush(ctx); err != nil {
			errs = append(errs, fmt.Errorf("trace provider: %w", err))
		}
	}
	if t.mp != nil {
		if err := t.mp.ForceFlush(ctx); err != nil {
			errs = append(errs, fmt.Errorf("meter provider: %w", err))
		}
	}
	return errors.Join(errs...)
}

// Shutdown flushes and stops both providers. Safe on a disabled Telemetry.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()
	var errs []error
	if t.tp != nil {
		if err := t.tp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("trace provider: %w", err))
		}
	}
	if t.mp != nil {
		if err := t.mp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("meter provider: %w", err))
		}
	}
	return errors.Join(errs...)
}
