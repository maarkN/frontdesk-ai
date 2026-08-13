package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maarkn/frontdesk/internal/domain"
	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/tenantctx"
)

// uniqueViolation is the Postgres SQLSTATE for unique constraint errors.
const uniqueViolation = "23505"

// PG is the Postgres store. Isolation is enforced by row-level security
// (ADR-005): every transaction that touches tenant data first runs
// `set_config('app.tenant_id', <tenant>, true)` — the `true` makes it
// transaction-local (equivalent to SET LOCAL), mandatory with a connection
// pool — and every tenant table has ENABLE + FORCE ROW LEVEL SECURITY (see
// migrations/0001_init.sql). Queries never filter by tenant explicitly: the
// policy is the filter, so a forgotten WHERE clause yields zero rows, not a
// leak.
type PG struct {
	pool *pgxpool.Pool
}

// NewPG returns a store backed by the given pool.
func NewPG(pool *pgxpool.Pool) *PG {
	return &PG{pool: pool}
}

// inTxAs runs fn inside a transaction with app.tenant_id set to tenantID.
// Only resolution-point operations (CreateTenant) may call it with an id
// that did not come from the context.
func (p *PG) inTxAs(ctx context.Context, tenantID string, fn func(pgx.Tx) error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after commit

	if _, err := tx.Exec(ctx, `select set_config('app.tenant_id', $1, true)`, tenantID); err != nil {
		return fmt.Errorf("set app.tenant_id: %w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// inTenantTx is inTxAs with the tenant taken from the context — the only
// source of tenant identity below the resolution point.
func (p *PG) inTenantTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tenantID, err := tenantctx.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("resolve tenant: %w", err)
	}
	return p.inTxAs(ctx, tenantID, fn)
}

// CreateTenant inserts the tenant and its API key hash. It sets
// app.tenant_id to the id of the tenant being created, so the RLS policies
// admit the insert without any bypass role.
func (p *PG) CreateTenant(ctx context.Context, t domain.Tenant) error {
	if err := t.Validate(); err != nil {
		return fmt.Errorf("create tenant: %w", err)
	}
	hours, err := json.Marshal(t.Hours)
	if err != nil {
		return fmt.Errorf("create tenant: marshal hours: %w", err)
	}
	coverage, err := json.Marshal(t.Coverage)
	if err != nil {
		return fmt.Errorf("create tenant: marshal coverage: %w", err)
	}
	err = p.inTxAs(ctx, t.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			insert into tenants
				(id, name, plan, mode, overflow_seconds, hours, coverage,
				 owner_mobile, default_locale, trial_ends_at, created_at)
			values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			t.ID, t.Name, string(t.Plan), string(t.Mode), t.OverflowSeconds,
			hours, coverage, t.OwnerMobile, string(t.DefaultLocale),
			nullableTime(t.TrialEndsAt), t.CreatedAt)
		if err != nil {
			return fmt.Errorf("insert tenant: %w", err)
		}
		if t.APIKeyHash != "" {
			if _, err := tx.Exec(ctx, `
				insert into tenant_api_keys (key_hash, tenant_id, created_at)
				values ($1,$2,$3)`, t.APIKeyHash, t.ID, t.CreatedAt); err != nil {
				return fmt.Errorf("insert api key: %w", err)
			}
		}
		return nil
	})
	if isUnique(err) {
		return fmt.Errorf("create tenant %s: %w", t.ID, domain.ErrConflict)
	}
	return err
}

// TenantByAPIKeyHash resolves an API key hash to its tenant. The lookup
// table tenant_api_keys is the resolution edge and deliberately has no RLS;
// the tenant row itself is then read under that tenant's policy.
func (p *PG) TenantByAPIKeyHash(ctx context.Context, keyHash string) (domain.Tenant, error) {
	var tenantID string
	err := p.pool.QueryRow(ctx,
		`select tenant_id from tenant_api_keys where key_hash = $1`, keyHash).Scan(&tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Tenant{}, fmt.Errorf("tenant by api key: %w", domain.ErrNotFound)
	}
	if err != nil {
		return domain.Tenant{}, fmt.Errorf("tenant by api key: %w", err)
	}

	var t domain.Tenant
	err = p.inTxAs(ctx, tenantID, func(tx pgx.Tx) error {
		var scanErr error
		t, scanErr = scanTenant(ctx, tx)
		return scanErr
	})
	if err != nil {
		return domain.Tenant{}, err
	}
	t.APIKeyHash = keyHash
	return t, nil
}

// CurrentTenant returns the tenant carried by the context.
func (p *PG) CurrentTenant(ctx context.Context) (domain.Tenant, error) {
	var t domain.Tenant
	err := p.inTenantTx(ctx, func(tx pgx.Tx) error {
		var scanErr error
		t, scanErr = scanTenant(ctx, tx)
		return scanErr
	})
	return t, err
}

// scanTenant reads the single tenant row visible under the current policy.
func scanTenant(ctx context.Context, tx pgx.Tx) (domain.Tenant, error) {
	var (
		t               domain.Tenant
		plan, mode, loc string
		hours, coverage []byte
		trialEndsAt     *time.Time
	)
	err := tx.QueryRow(ctx, `
		select id, name, plan, mode, overflow_seconds, hours, coverage,
		       owner_mobile, default_locale, trial_ends_at, created_at
		from tenants
		where id = current_setting('app.tenant_id', true)`).
		Scan(&t.ID, &t.Name, &plan, &mode, &t.OverflowSeconds, &hours,
			&coverage, &t.OwnerMobile, &loc, &trialEndsAt, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Tenant{}, fmt.Errorf("scan tenant: %w", domain.ErrNotFound)
	}
	if err != nil {
		return domain.Tenant{}, fmt.Errorf("scan tenant: %w", err)
	}
	t.Plan, t.Mode, t.DefaultLocale = domain.Plan(plan), domain.Mode(mode), event.Locale(loc)
	if trialEndsAt != nil {
		t.TrialEndsAt = *trialEndsAt
	}
	if err := json.Unmarshal(hours, &t.Hours); err != nil {
		return domain.Tenant{}, fmt.Errorf("scan tenant: unmarshal hours: %w", err)
	}
	if err := json.Unmarshal(coverage, &t.Coverage); err != nil {
		return domain.Tenant{}, fmt.Errorf("scan tenant: unmarshal coverage: %w", err)
	}
	return t, nil
}

// UpdateTenantMode toggles overflow / always-AI for the context tenant.
func (p *PG) UpdateTenantMode(ctx context.Context, mode domain.Mode, overflowSeconds int) error {
	if !mode.Valid() {
		return fmt.Errorf("update mode: unknown mode %q", mode)
	}
	return p.inTenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			update tenants set mode = $1, overflow_seconds = $2
			where id = current_setting('app.tenant_id', true)`,
			string(mode), overflowSeconds)
		if err != nil {
			return fmt.Errorf("update mode: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("update mode: %w", domain.ErrNotFound)
		}
		return nil
	})
}

// AddDID provisions a DID for the context tenant. The global unique index on
// dids.number spans tenants (indexes are not subject to RLS).
func (p *PG) AddDID(ctx context.Context, d domain.DID) error {
	if err := d.Validate(); err != nil {
		return fmt.Errorf("add did: %w", err)
	}
	err := p.inTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			insert into dids (number, tenant_id, provider, active, created_at)
			values ($1, current_setting('app.tenant_id', true), $2, $3, $4)`,
			d.Number, d.Provider, d.Active, d.CreatedAt)
		return err
	})
	if isUnique(err) {
		return fmt.Errorf("add did %s: %w", d.Number, domain.ErrConflict)
	}
	if err != nil {
		return fmt.Errorf("add did: %w", err)
	}
	return nil
}

// ListDIDs returns the context tenant's DIDs, ordered by number.
func (p *PG) ListDIDs(ctx context.Context) ([]domain.DID, error) {
	var out []domain.DID
	err := p.inTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`select number, provider, active, created_at from dids order by number`)
		if err != nil {
			return fmt.Errorf("list dids: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.DID, error) {
			var d domain.DID
			err := row.Scan(&d.Number, &d.Provider, &d.Active, &d.CreatedAt)
			return d, err
		})
		return err
	})
	return out, err
}

// PutCall upserts a call projection (fold snapshot + recording URL).
func (p *PG) PutCall(ctx context.Context, c domain.Call) error {
	if c.ID() == "" {
		return fmt.Errorf("put call: missing call id")
	}
	snapshot, err := json.Marshal(c.Snapshot)
	if err != nil {
		return fmt.Errorf("put call: marshal snapshot: %w", err)
	}
	return p.inTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			insert into calls (call_id, tenant_id, started_at, snapshot, recording_url)
			values ($1, current_setting('app.tenant_id', true), $2, $3, $4)
			on conflict (tenant_id, call_id)
			do update set started_at = excluded.started_at,
			              snapshot = excluded.snapshot,
			              recording_url = excluded.recording_url`,
			c.ID(), nullableTime(c.Snapshot.StartedAt), snapshot, c.RecordingURL)
		if err != nil {
			return fmt.Errorf("put call: %w", err)
		}
		return nil
	})
}

// ListCalls returns the context tenant's calls, most recent first.
func (p *PG) ListCalls(ctx context.Context) ([]domain.Call, error) {
	var out []domain.Call
	err := p.inTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`select snapshot, recording_url from calls order by started_at desc nulls last`)
		if err != nil {
			return fmt.Errorf("list calls: %w", err)
		}
		out, err = pgx.CollectRows(rows, scanCall)
		return err
	})
	return out, err
}

// GetCall returns one call of the context tenant.
func (p *PG) GetCall(ctx context.Context, callID string) (domain.Call, error) {
	var c domain.Call
	err := p.inTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`select snapshot, recording_url from calls where call_id = $1`, callID)
		var snapshot []byte
		if err := row.Scan(&snapshot, &c.RecordingURL); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("get call %s: %w", callID, domain.ErrNotFound)
			}
			return fmt.Errorf("get call: %w", err)
		}
		if err := json.Unmarshal(snapshot, &c.Snapshot); err != nil {
			return fmt.Errorf("get call: unmarshal snapshot: %w", err)
		}
		return nil
	})
	return c, err
}

// scanCall decodes one calls row.
func scanCall(row pgx.CollectableRow) (domain.Call, error) {
	var (
		c        domain.Call
		snapshot []byte
	)
	if err := row.Scan(&snapshot, &c.RecordingURL); err != nil {
		return domain.Call{}, err
	}
	if err := json.Unmarshal(snapshot, &c.Snapshot); err != nil {
		return domain.Call{}, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	return c, nil
}

// CreateAppointment stores an appointment for the context tenant.
func (p *PG) CreateAppointment(ctx context.Context, a domain.Appointment) error {
	if err := a.Validate(); err != nil {
		return fmt.Errorf("create appointment: %w", err)
	}
	err := p.inTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			insert into appointments
				(id, tenant_id, call_id, customer_name, customer_phone,
				 service, postal_code, starts_at, ends_at, notes, created_at)
			values ($1, current_setting('app.tenant_id', true), $2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			a.ID, a.CallID, a.CustomerName, a.CustomerPhone, a.Service,
			a.PostalCode, a.StartsAt, a.EndsAt, a.Notes, a.CreatedAt)
		return err
	})
	if isUnique(err) {
		return fmt.Errorf("create appointment %s: %w", a.ID, domain.ErrConflict)
	}
	if err != nil {
		return fmt.Errorf("create appointment: %w", err)
	}
	return nil
}

// ListAppointments returns the context tenant's appointments by start time.
func (p *PG) ListAppointments(ctx context.Context) ([]domain.Appointment, error) {
	var out []domain.Appointment
	err := p.inTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			select id, call_id, customer_name, customer_phone, service,
			       postal_code, starts_at, ends_at, notes, created_at
			from appointments order by starts_at`)
		if err != nil {
			return fmt.Errorf("list appointments: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Appointment, error) {
			var a domain.Appointment
			err := row.Scan(&a.ID, &a.CallID, &a.CustomerName, &a.CustomerPhone,
				&a.Service, &a.PostalCode, &a.StartsAt, &a.EndsAt, &a.Notes, &a.CreatedAt)
			return a, err
		})
		return err
	})
	return out, err
}

// CreateMessage stores a structured message for the context tenant.
func (p *PG) CreateMessage(ctx context.Context, msg domain.Message) error {
	if err := msg.Validate(); err != nil {
		return fmt.Errorf("create message: %w", err)
	}
	err := p.inTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			insert into messages
				(id, tenant_id, call_id, who, what, urgency, callback_phone, created_at)
			values ($1, current_setting('app.tenant_id', true), $2,$3,$4,$5,$6,$7)`,
			msg.ID, msg.CallID, msg.Who, msg.What, string(msg.Urgency),
			msg.CallbackPhone, msg.CreatedAt)
		return err
	})
	if isUnique(err) {
		return fmt.Errorf("create message %s: %w", msg.ID, domain.ErrConflict)
	}
	if err != nil {
		return fmt.Errorf("create message: %w", err)
	}
	return nil
}

// ListMessages returns the context tenant's messages, most recent first.
func (p *PG) ListMessages(ctx context.Context) ([]domain.Message, error) {
	var out []domain.Message
	err := p.inTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			select id, call_id, who, what, urgency, callback_phone, created_at
			from messages order by created_at desc`)
		if err != nil {
			return fmt.Errorf("list messages: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.Message, error) {
			var (
				m       domain.Message
				urgency string
			)
			err := row.Scan(&m.ID, &m.CallID, &m.Who, &m.What, &urgency,
				&m.CallbackPhone, &m.CreatedAt)
			m.Urgency = domain.Urgency(urgency)
			return m, err
		})
		return err
	})
	return out, err
}

// isUnique reports whether err is a Postgres unique constraint violation.
func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

// nullableTime maps the zero time to NULL.
func nullableTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
