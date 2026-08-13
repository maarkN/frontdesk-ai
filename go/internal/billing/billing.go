// Package billing turns the event stream into money: usage is a fold over
// the same events that feed every other projection (ADR-003 — billing has no
// separate pipeline), invoices apply the plan table (included minutes plus
// CAD 0.35/min overage), and Stripe sits behind a small consumer-defined
// interface with a deterministic fake for the MVP (14-day trial).
//
// All monetary amounts are integer cents (CAD) — never floats.
package billing

import (
	"fmt"

	"github.com/maarkn/frontdesk/internal/domain"
	"github.com/maarkn/frontdesk/internal/event"
)

// OverageCentsPerMinute is the per-minute overage price (CAD 0.35) shared by
// every plan.
const OverageCentsPerMinute int64 = 35

// PlanPricing is the monthly price and inclusions of one plan.
type PlanPricing struct {
	// MonthlyCents is the subscription price in CAD cents.
	MonthlyCents int64 `json:"monthlyCents"`
	// IncludedMinutes is the monthly allowance of call minutes.
	IncludedMinutes int64 `json:"includedMinutes"`
	// OverageCentsPerMinute prices each minute beyond the allowance.
	OverageCentsPerMinute int64 `json:"overageCentsPerMinute"`
}

// pricingByPlan is the pricing hypothesis from CONTEXT.md: Starter CAD
// 149/200 min, Pro CAD 249/500 min, Growth CAD 399/1000 min.
var pricingByPlan = map[domain.Plan]PlanPricing{
	domain.PlanStarter: {MonthlyCents: 14_900, IncludedMinutes: 200, OverageCentsPerMinute: OverageCentsPerMinute},
	domain.PlanPro:     {MonthlyCents: 24_900, IncludedMinutes: 500, OverageCentsPerMinute: OverageCentsPerMinute},
	domain.PlanGrowth:  {MonthlyCents: 39_900, IncludedMinutes: 1_000, OverageCentsPerMinute: OverageCentsPerMinute},
}

// PricingFor returns the pricing of a plan.
func PricingFor(plan domain.Plan) (PlanPricing, error) {
	p, ok := pricingByPlan[plan]
	if !ok {
		return PlanPricing{}, fmt.Errorf("billing: unknown plan %q", plan)
	}
	return p, nil
}

// UsageFromEvents folds a mixed stream (any number of calls, any order, with
// duplicates) into total billable usage. It groups by CallID and reuses
// event.Fold per call, so it inherits the fold's dedup/ordering guarantees.
func UsageFromEvents(events []event.Envelope) (event.Usage, error) {
	byCall := make(map[string][]event.Envelope)
	for _, ev := range events {
		byCall[ev.CallID] = append(byCall[ev.CallID], ev)
	}
	var total event.Usage
	for callID, callEvents := range byCall {
		snap, err := event.Fold(callEvents)
		if err != nil {
			return event.Usage{}, fmt.Errorf("billing: fold call %s: %w", callID, err)
		}
		total = AddUsage(total, snap.Usage)
	}
	return total, nil
}

// SumUsage totals the usage of already-folded call snapshots.
func SumUsage(snapshots []event.CallSnapshot) event.Usage {
	var total event.Usage
	for _, snap := range snapshots {
		total = AddUsage(total, snap.Usage)
	}
	return total
}

// AddUsage returns a + b, field by field.
func AddUsage(a, b event.Usage) event.Usage {
	return event.Usage{
		CallMinutes:  a.CallMinutes + b.CallMinutes,
		STTSeconds:   a.STTSeconds + b.STTSeconds,
		LLMTokensIn:  a.LLMTokensIn + b.LLMTokensIn,
		LLMTokensOut: a.LLMTokensOut + b.LLMTokensOut,
		TTSChars:     a.TTSChars + b.TTSChars,
	}
}

// Invoice is the monthly charge derived from a plan and folded usage.
type Invoice struct {
	Plan            domain.Plan `json:"plan"`
	Usage           event.Usage `json:"usage"`
	BaseCents       int64       `json:"baseCents"`
	IncludedMinutes int64       `json:"includedMinutes"`
	OverageMinutes  int64       `json:"overageMinutes"`
	OverageCents    int64       `json:"overageCents"`
	// TotalCents is zero while the tenant is in trial.
	TotalCents int64 `json:"totalCents"`
	Trial      bool  `json:"trial"`
}

// Compute prices one billing period: base plan price plus overage minutes at
// the plan's per-minute rate. During the trial the total is zero but the
// breakdown is still reported (the dashboard shows what the period would
// cost).
func Compute(plan domain.Plan, usage event.Usage, inTrial bool) (Invoice, error) {
	pricing, err := PricingFor(plan)
	if err != nil {
		return Invoice{}, err
	}
	overageMinutes := usage.CallMinutes - pricing.IncludedMinutes
	if overageMinutes < 0 {
		overageMinutes = 0
	}
	inv := Invoice{
		Plan:            plan,
		Usage:           usage,
		BaseCents:       pricing.MonthlyCents,
		IncludedMinutes: pricing.IncludedMinutes,
		OverageMinutes:  overageMinutes,
		OverageCents:    overageMinutes * pricing.OverageCentsPerMinute,
		Trial:           inTrial,
	}
	if !inTrial {
		inv.TotalCents = inv.BaseCents + inv.OverageCents
	}
	return inv, nil
}
