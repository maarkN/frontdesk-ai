// Command notifier runs the FrontDesk notification service (EPIC-005): it
// consumes call events from NATS JetStream and sends the post-call owner and
// client SMS plus the weekly per-tenant report e-mail.
//
// Configuration is environment-only (see loadConfig). Without provider
// credentials it falls back to log-only senders, which keeps local runs and
// demos free of external calls.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/notify"
	"github.com/maarkn/frontdesk/internal/notify/ses"
	"github.com/maarkn/frontdesk/internal/notify/telnyx"
	"github.com/maarkn/frontdesk/internal/obs"
)

// serviceName identifies this binary in logs and telemetry resources.
const serviceName = "notifier"

func main() {
	slog.SetDefault(obs.NewLogger(obs.WithLogService(serviceName, os.Getenv("SERVICE_VERSION"))))
	if err := run(); err != nil {
		slog.Error("notifier exited", "err", err)
		os.Exit(1)
	}
}

// config is the environment configuration of the service.
type config struct {
	natsURL       string
	stream        string
	consumer      string
	subjectPrefix string

	telnyxAPIKey string
	telnyxFrom   string

	sesFrom      string
	awsAccessKey string
	awsSecretKey string

	transcriptBaseURL string
	tenantsFile       string
	weeklyInterval    time.Duration
}

// loadConfig reads configuration from the environment, with local-friendly
// defaults.
func loadConfig() (config, error) {
	cfg := config{
		natsURL:           envOr("NOTIFIER_NATS_URL", nats.DefaultURL),
		stream:            envOr("NOTIFIER_STREAM", "FRONTDESK_EVENTS"),
		consumer:          envOr("NOTIFIER_CONSUMER", "notifier"),
		subjectPrefix:     envOr("NOTIFIER_SUBJECT_PREFIX", "events"),
		telnyxAPIKey:      os.Getenv("TELNYX_API_KEY"),
		telnyxFrom:        os.Getenv("TELNYX_FROM"),
		sesFrom:           os.Getenv("SES_FROM"),
		awsAccessKey:      os.Getenv("AWS_ACCESS_KEY_ID"),
		awsSecretKey:      os.Getenv("AWS_SECRET_ACCESS_KEY"),
		transcriptBaseURL: os.Getenv("NOTIFIER_TRANSCRIPT_BASE_URL"),
		tenantsFile:       os.Getenv("NOTIFIER_TENANTS_FILE"),
	}
	interval := envOr("NOTIFIER_WEEKLY_CHECK_INTERVAL", "1h")
	d, err := time.ParseDuration(interval)
	if err != nil {
		return config{}, fmt.Errorf("parse NOTIFIER_WEEKLY_CHECK_INTERVAL %q: %w", interval, err)
	}
	cfg.weeklyInterval = d
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	tenants, err := loadTenants(cfg.tenantsFile)
	if err != nil {
		return err
	}

	// Telemetry (EPIC-010): the OTel metrics adapter implements
	// notify.Metrics; without a collector endpoint the log-only slaMetrics
	// keeps the SLA visible.
	tel, err := obs.Setup(ctx, obs.WithServiceName(serviceName), obs.WithInsecure())
	if err != nil {
		return fmt.Errorf("telemetry setup: %w", err)
	}
	defer func() {
		if err := tel.Shutdown(context.Background()); err != nil {
			slog.Warn("telemetry shutdown", "err", err)
		}
	}()
	var metrics notify.Metrics = slaMetrics{}
	if tel.Enabled() {
		m, err := obs.NewMetrics(tel.Meter(serviceName))
		if err != nil {
			return fmt.Errorf("telemetry metrics: %w", err)
		}
		metrics = m
	}

	n, err := notify.New(notify.Deps{
		SMS:     newSMSSender(cfg),
		Email:   newEmailSender(cfg),
		Dedup:   &notify.MemoryDedup{},
		Tenants: tenants,
		Metrics: metrics,
	}, notify.WithTranscriptBaseURL(cfg.transcriptBaseURL))
	if err != nil {
		return err
	}

	nc, err := nats.Connect(cfg.natsURL, nats.Name("frontdesk-notifier"))
	if err != nil {
		return fmt.Errorf("connect to NATS %s: %w", cfg.natsURL, err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("open JetStream: %w", err)
	}
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     cfg.stream,
		Subjects: []string{cfg.subjectPrefix + ".>"},
	})
	if err != nil {
		return fmt.Errorf("ensure stream %s: %w", cfg.stream, err)
	}
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       cfg.consumer,
		FilterSubject: cfg.subjectPrefix + ".>",
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return fmt.Errorf("ensure consumer %s: %w", cfg.consumer, err)
	}

	// Weekly report ticker: owned here, joined before exit (ADR-008 — every
	// goroutine has an owner and a lifecycle).
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(cfg.weeklyInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := n.SendWeeklyReports(ctx); err != nil {
					slog.Error("weekly reports", "err", err)
				}
			}
		}
	}()

	slog.Info("notifier consuming events", "subjectPrefix", cfg.subjectPrefix, "natsURL", cfg.natsURL)
	err = event.NewJetStreamSubscriber(consumer).Subscribe(ctx, n.HandleEvent)
	wg.Wait()
	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("subscribe: %w", err)
	}
	slog.Info("notifier stopped")
	return nil
}

// loadTenants builds the tenant directory from an optional JSON file
// (map[tenantID]TenantInfo) until the core-api-backed directory lands
// (EPIC-004 integration).
func loadTenants(path string) (*notify.MemoryDirectory, error) {
	dir := notify.NewMemoryDirectory()
	if path == "" {
		return dir, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tenants file: %w", err)
	}
	var tenants map[string]notify.TenantInfo
	if err := json.Unmarshal(raw, &tenants); err != nil {
		return nil, fmt.Errorf("parse tenants file %s: %w", path, err)
	}
	for id, info := range tenants {
		dir.Put(id, info)
	}
	return dir, nil
}

// newSMSSender wires Telnyx when credentials are present, a log-only sender
// otherwise.
func newSMSSender(cfg config) notify.SMSSender {
	if cfg.telnyxAPIKey != "" && cfg.telnyxFrom != "" {
		return telnyx.New(cfg.telnyxAPIKey, cfg.telnyxFrom)
	}
	slog.Warn("TELNYX_API_KEY/TELNYX_FROM unset; SMS in log-only mode")
	return logSMS{}
}

// newEmailSender wires SES when credentials are present, a log-only sender
// otherwise.
func newEmailSender(cfg config) notify.EmailSender {
	if cfg.sesFrom != "" && cfg.awsAccessKey != "" {
		return ses.New(cfg.sesFrom, cfg.awsAccessKey, cfg.awsSecretKey)
	}
	slog.Warn("SES_FROM/AWS credentials unset; e-mail in log-only mode")
	return logEmail{}
}

// logSMS logs instead of sending. Message content is safe to log: it is
// assembled from events already PII-redacted at emission (ADR-003).
type logSMS struct{}

var _ notify.SMSSender = logSMS{}

// Send implements notify.SMSSender.
func (logSMS) Send(ctx context.Context, msg notify.SMS) error {
	slog.InfoContext(ctx, "SMS (log-only)", "to", msg.To, "body", msg.Body)
	return nil
}

// logEmail logs instead of sending.
type logEmail struct{}

var _ notify.EmailSender = logEmail{}

// Send implements notify.EmailSender.
func (logEmail) Send(ctx context.Context, msg notify.Email) error {
	slog.InfoContext(ctx, "EMAIL (log-only)", "to", msg.To, "subject", msg.Subject, "body", msg.Body)
	return nil
}

// slaMetrics logs sms_owner_latency_seconds and flags SLA breaches until the
// real exporter lands (EPIC-010).
type slaMetrics struct{}

var _ notify.Metrics = slaMetrics{}

// ObserveOwnerSMSLatency implements notify.Metrics.
func (slaMetrics) ObserveOwnerSMSLatency(d time.Duration) {
	slog.Info("metric", "sms_owner_latency_seconds", d.Seconds())
	if d > 60*time.Second {
		slog.Warn("SLA breach: owner SMS over 60s", "seconds", d.Seconds())
	}
}
