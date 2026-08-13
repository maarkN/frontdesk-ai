package notify

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/maarkn/frontdesk/internal/tenantctx"
)

// SendWeeklyReports sends the weekly e-mail report to every known tenant.
// It is idempotent per (tenant, ISO week in the tenant's timezone): calling
// it repeatedly — e.g. from an hourly ticker — sends at most one report per
// tenant per week, at the first invocation of that week. Tenants without
// activity receive the "no calls" variant (US-5.3). Per-tenant failures are
// joined so one bad tenant does not block the sweep.
func (n *Notifier) SendWeeklyReports(ctx context.Context) error {
	ids, err := n.tenants.TenantIDs(ctx)
	if err != nil {
		return fmt.Errorf("list tenants: %w", err)
	}

	var errs []error
	for _, id := range ids {
		if err := n.sendWeeklyReport(tenantctx.WithTenant(ctx, id), id); err != nil {
			errs = append(errs, fmt.Errorf("weekly report tenant %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

// sendWeeklyReport renders and sends one tenant's report, then resets its
// counters.
func (n *Notifier) sendWeeklyReport(ctx context.Context, tenantID string) error {
	info, err := n.tenants.Lookup(ctx)
	if err != nil {
		return fmt.Errorf("lookup tenant: %w", err)
	}
	if info.OwnerEmail == "" {
		return nil
	}

	tz := loadLocation(info.Timezone)
	year, week := n.now().In(tz).ISOWeek()
	key := fmt.Sprintf("weekly:%s:%d-W%02d", tenantID, year, week)
	seen, err := n.dedup.Seen(ctx, key)
	if err != nil {
		return fmt.Errorf("dedup weekly report: %w", err)
	}
	if seen {
		return nil
	}

	n.mu.Lock()
	stats := n.weekly[tenantID]
	n.mu.Unlock()

	loc := info.locale()
	out, err := render(n.tmpl, templateName("weekly_email", loc), weeklyData{
		Business:       info.Name,
		CallsAnswered:  stats.CallsAnswered,
		JobsBooked:     stats.JobsBooked,
		Messages:       stats.Messages,
		EstimatedValue: float64(stats.JobsBooked) * info.AvgJobValueCAD,
	})
	if err != nil {
		return err
	}
	// Template convention: first line is the subject, the rest the body.
	subject, body, _ := strings.Cut(out, "\n")
	msg := Email{
		To:      info.OwnerEmail,
		Subject: strings.TrimSpace(subject),
		Body:    strings.TrimSpace(body),
	}
	if err := n.email.Send(ctx, msg); err != nil {
		return fmt.Errorf("send weekly e-mail: %w", err)
	}
	if err := n.dedup.Mark(ctx, key); err != nil {
		return fmt.Errorf("mark weekly report: %w", err)
	}

	n.mu.Lock()
	delete(n.weekly, tenantID)
	n.mu.Unlock()
	return nil
}
