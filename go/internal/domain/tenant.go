// Package domain holds the core-api business types: tenants and their
// configuration, DIDs, calls (projections of the event stream), appointments,
// structured messages and the self-service onboarding profile.
//
// Types here are storage- and transport-agnostic. Tenant identity never
// travels as a function parameter below the resolution point (ADR-005): it
// lives in the context via internal/tenantctx, so domain types carry no
// tenant id field except where the aggregate IS the tenant.
package domain

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/maarkn/frontdesk/internal/event"
)

// Sentinel errors shared by every store implementation. They live in domain
// so that consumers (api, billing) never import a concrete store package.
var (
	// ErrNotFound reports that the aggregate does not exist for the
	// current tenant. Cross-tenant reads MUST look identical to a miss.
	ErrNotFound = errors.New("not found")
	// ErrConflict reports a uniqueness violation (duplicate id, DID
	// number already provisioned, ...).
	ErrConflict = errors.New("conflict")
)

// NewID returns a ULID string for use as an aggregate id.
func NewID(now time.Time) string {
	return ulid.MustNew(ulid.Timestamp(now.UTC()), rand.Reader).String()
}

// Plan is the subscription plan of a tenant.
type Plan string

// Subscription plans (pricing hypothesis in CONTEXT.md).
const (
	PlanStarter Plan = "starter"
	PlanPro     Plan = "pro"
	PlanGrowth  Plan = "growth"
)

// Valid reports whether p is a known plan.
func (p Plan) Valid() bool {
	switch p {
	case PlanStarter, PlanPro, PlanGrowth:
		return true
	default:
		return false
	}
}

// Mode is how the AI answers the tenant's line.
type Mode string

// Answering modes: overflow (AI only if nobody answers in OverflowSeconds)
// and always-AI. String values match event.CallMode on the bus.
const (
	ModeOverflow Mode = Mode(event.ModeOverflow)
	ModeAlwaysAI Mode = Mode(event.ModeAlwaysAI)
)

// Valid reports whether m is a known mode.
func (m Mode) Valid() bool {
	switch m {
	case ModeOverflow, ModeAlwaysAI:
		return true
	default:
		return false
	}
}

// TimeRange is an open interval within a day, in local "15:04" format.
type TimeRange struct {
	Open  string `json:"open"`
	Close string `json:"close"`
}

// Validate checks that both bounds parse and open precedes close.
func (r TimeRange) Validate() error {
	open, err := time.Parse("15:04", r.Open)
	if err != nil {
		return fmt.Errorf("open %q: %w", r.Open, err)
	}
	close, err := time.Parse("15:04", r.Close)
	if err != nil {
		return fmt.Errorf("close %q: %w", r.Close, err)
	}
	if !open.Before(close) {
		return fmt.Errorf("open %q must precede close %q", r.Open, r.Close)
	}
	return nil
}

// contains reports whether the clock time hhmm ("15:04") falls in the range.
func (r TimeRange) contains(hhmm string) bool {
	return r.Open <= hhmm && hhmm < r.Close
}

// BusinessHours maps weekday keys ("sun".."sat") to open ranges. A day with
// no ranges is closed; the zero value means always closed (always-AI treats
// everything as after-hours until configured).
type BusinessHours map[string][]TimeRange

// weekdayKeys indexes time.Weekday (Sunday = 0) into the JSON day keys.
var weekdayKeys = [...]string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// Validate checks day keys and every range.
func (h BusinessHours) Validate() error {
	for day, ranges := range h {
		if !validDayKey(day) {
			return fmt.Errorf("unknown day key %q", day)
		}
		for _, r := range ranges {
			if err := r.Validate(); err != nil {
				return fmt.Errorf("day %s: %w", day, err)
			}
		}
	}
	return nil
}

// IsOpen reports whether t (interpreted in its own location) falls inside
// business hours.
func (h BusinessHours) IsOpen(t time.Time) bool {
	ranges := h[weekdayKeys[t.Weekday()]]
	hhmm := t.Format("15:04")
	for _, r := range ranges {
		if r.contains(hhmm) {
			return true
		}
	}
	return false
}

func validDayKey(day string) bool {
	for _, k := range weekdayKeys {
		if day == k {
			return true
		}
	}
	return false
}

// CoverageArea restricts service by Canadian postal code prefixes (FSA, e.g.
// "H2X"). An empty prefix list means no restriction.
type CoverageArea struct {
	PostalPrefixes []string `json:"postalPrefixes,omitempty"`
}

// Covers reports whether the postal code (any spacing/case, e.g. "h2x 1y4")
// is inside the coverage area.
func (c CoverageArea) Covers(postalCode string) bool {
	if len(c.PostalPrefixes) == 0 {
		return true
	}
	normalized := strings.ToUpper(strings.ReplaceAll(postalCode, " ", ""))
	for _, prefix := range c.PostalPrefixes {
		p := strings.ToUpper(strings.ReplaceAll(prefix, " ", ""))
		if p != "" && strings.HasPrefix(normalized, p) {
			return true
		}
	}
	return false
}

// Tenant is one FrontDesk AI customer (a service SMB) and its agent
// configuration.
type Tenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Plan Plan   `json:"plan"`

	// Mode selects overflow vs always-AI; OverflowSeconds is how long the
	// line rings before the AI picks up in overflow mode.
	Mode            Mode `json:"mode"`
	OverflowSeconds int  `json:"overflowSeconds,omitempty"`

	Hours    BusinessHours `json:"hours,omitempty"`
	Coverage CoverageArea  `json:"coverage,omitzero"`

	// OwnerMobile receives warm transfers and post-call SMS (E.164).
	OwnerMobile   string       `json:"ownerMobile"`
	DefaultLocale event.Locale `json:"defaultLocale"`

	// APIKeyHash is the SHA-256 (hex) of the tenant API key. The plain key
	// is shown once at onboarding and never stored.
	APIKeyHash string `json:"-"`

	// TrialEndsAt is the end of the 14-day Stripe trial.
	TrialEndsAt time.Time `json:"trialEndsAt,omitzero"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Validate checks the tenant configuration invariants.
func (t Tenant) Validate() error {
	if t.ID == "" {
		return errors.New("tenant: missing id")
	}
	if t.Name == "" {
		return errors.New("tenant: missing name")
	}
	if !t.Plan.Valid() {
		return fmt.Errorf("tenant: unknown plan %q", t.Plan)
	}
	if !t.Mode.Valid() {
		return fmt.Errorf("tenant: unknown mode %q", t.Mode)
	}
	if t.Mode == ModeOverflow && t.OverflowSeconds <= 0 {
		return errors.New("tenant: overflow mode requires overflowSeconds > 0")
	}
	if t.DefaultLocale != event.LocaleENCA && t.DefaultLocale != event.LocaleFRCA {
		return fmt.Errorf("tenant: unknown locale %q", t.DefaultLocale)
	}
	if err := t.Hours.Validate(); err != nil {
		return fmt.Errorf("tenant: hours: %w", err)
	}
	return nil
}
