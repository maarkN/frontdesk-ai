package obs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/maarkn/frontdesk/internal/obs"
	"github.com/maarkn/frontdesk/internal/tenantctx"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

func TestNewLoggerEmitsJSONWithServiceAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := obs.NewLogger(
		obs.WithLogWriter(&buf),
		obs.WithLogLevel(slog.LevelInfo),
		obs.WithLogService("telephony-gw", "1.2.3"),
	)
	logger.Info("hello", "k", "v")

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	assert.Equal(t, "hello", rec["msg"])
	assert.Equal(t, "telephony-gw", rec["service"])
	assert.Equal(t, "1.2.3", rec["version"])
	assert.Equal(t, "v", rec["k"])
}

func TestNewLoggerAddsTenantFromContext(t *testing.T) {
	var buf bytes.Buffer
	logger := obs.NewLogger(obs.WithLogWriter(&buf), obs.WithLogLevel(slog.LevelInfo))

	ctx := tenantctx.WithTenant(context.Background(), "tenant-a")
	logger.InfoContext(ctx, "in call")
	logger.InfoContext(context.Background(), "no tenant")

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	require.Len(t, lines, 2)

	var withTenant, without map[string]any
	require.NoError(t, json.Unmarshal(lines[0], &withTenant))
	require.NoError(t, json.Unmarshal(lines[1], &without))
	assert.Equal(t, "tenant-a", withTenant["tenant"])
	_, ok := without["tenant"]
	assert.False(t, ok, "records outside a call must have no tenant attribute")
}

func TestNewLoggerRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := obs.NewLogger(obs.WithLogWriter(&buf), obs.WithLogLevel(slog.LevelWarn))
	logger.Info("dropped")
	logger.Warn("kept")
	assert.NotContains(t, buf.String(), "dropped")
	assert.Contains(t, buf.String(), "kept")
}

func TestSetupDisabledWithoutEndpoint(t *testing.T) {
	tel, err := obs.Setup(context.Background(), obs.WithoutGlobal())
	require.NoError(t, err)
	assert.False(t, tel.Enabled())
	// No-op providers still hand out working instruments.
	_, span := tel.Tracer("t").Start(context.Background(), "noop")
	span.End()
	require.NoError(t, tel.Shutdown(context.Background()))
}

func TestSetupExportsSpansWithServiceResource(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	reader := sdkmetric.NewManualReader()

	tel, err := obs.Setup(context.Background(),
		obs.WithServiceName("core-api"),
		obs.WithServiceVersion("0.9.0"),
		obs.WithTraceExporter(exporter),
		obs.WithMetricReader(reader),
		obs.WithoutGlobal(),
	)
	require.NoError(t, err)
	require.True(t, tel.Enabled())

	t.Cleanup(func() { _ = tel.Shutdown(context.Background()) })

	_, span := tel.Tracer("test").Start(context.Background(), "handle-request")
	span.End()
	// Flush (not Shutdown): the in-memory exporter resets its spans on Shutdown.
	require.NoError(t, tel.ForceFlush(context.Background()))

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "handle-request", spans[0].Name)
	attrs := spans[0].Resource.Attributes()
	assert.Contains(t, attrs, semconv.ServiceName("core-api"))
	assert.Contains(t, attrs, semconv.ServiceVersion("0.9.0"))
}
