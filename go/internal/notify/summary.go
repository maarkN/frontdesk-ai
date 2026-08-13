package notify

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/maarkn/frontdesk/internal/event"
)

// Well-known flow variable names assigned by the agent runtime (EPIC-003)
// via var.assigned events. They are the notifier's read contract over the
// fold's Vars map; values are JSON strings, already redacted when textual.
const (
	VarCallerName    = "caller_name"
	VarService       = "service"
	VarUrgency       = "urgency"
	VarCallbackPhone = "callback_phone"
	// VarAppointmentAt is an RFC 3339 timestamp when a job was booked.
	VarAppointmentAt = "appointment_at"
	// VarMessage is the structured message left when nothing was booked.
	VarMessage = "message"
)

// Outcome is what the call produced for the business.
type Outcome string

// Call outcomes, in notification priority order.
const (
	OutcomeBooked      Outcome = "booked"
	OutcomeMessage     Outcome = "message"
	OutcomeTransferred Outcome = "transferred"
	OutcomeNone        Outcome = "none"
)

// Summary is the post-call digest the notifications are rendered from. It is
// derived exclusively from the (already redacted) event stream: state =
// fold(events), ADR-003.
type Summary struct {
	CallID   string
	TenantID string
	// Locale is the caller's conversation locale (language.detected /
	// language.switched); the client SMS is rendered in it.
	Locale  event.Locale
	EndedAt time.Time

	// CallerPhone is the calling number from call.started.
	CallerPhone string
	CallerName  string
	Service     string
	Urgency     string
	// CallbackPhone is the number the owner should call back: an explicit
	// callback_phone var when captured, the caller's number otherwise.
	CallbackPhone string

	Outcome       Outcome
	AppointmentAt time.Time
	Message       string
}

// BuildSummary folds a call's events into a Summary. It tolerates sparse
// streams (e.g. a redelivered call.ended after a restart lost the buffer):
// missing fields stay empty and templates degrade gracefully.
func BuildSummary(events []event.Envelope) (Summary, error) {
	snap, err := event.Fold(events)
	if err != nil {
		return Summary{}, fmt.Errorf("fold call events: %w", err)
	}

	sum := Summary{
		CallID:   snap.CallID,
		TenantID: snap.TenantID,
		Locale:   snap.Locale,
		EndedAt:  snap.EndedAt,
	}

	// The fold keeps no caller number; read it from call.started directly.
	for i := range events {
		if events[i].Type != event.TypeCallStarted {
			continue
		}
		var p event.CallStarted
		if err := json.Unmarshal(events[i].Payload, &p); err == nil {
			sum.CallerPhone = p.From
		}
		break
	}

	sum.CallerName = stringVar(snap.Vars, VarCallerName)
	sum.Service = stringVar(snap.Vars, VarService)
	sum.Urgency = stringVar(snap.Vars, VarUrgency)
	sum.Message = stringVar(snap.Vars, VarMessage)

	sum.CallbackPhone = stringVar(snap.Vars, VarCallbackPhone)
	if sum.CallbackPhone == "" {
		sum.CallbackPhone = sum.CallerPhone
	}

	if raw := stringVar(snap.Vars, VarAppointmentAt); raw != "" {
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return Summary{}, fmt.Errorf("parse %s %q: %w", VarAppointmentAt, raw, err)
		}
		sum.AppointmentAt = at
	}

	switch {
	case snap.EndReason == event.EndTransferred:
		sum.Outcome = OutcomeTransferred
	case !sum.AppointmentAt.IsZero():
		sum.Outcome = OutcomeBooked
	case sum.Message != "":
		sum.Outcome = OutcomeMessage
	default:
		sum.Outcome = OutcomeNone
	}
	return sum, nil
}

// stringVar decodes a JSON string flow variable; anything else yields "".
func stringVar(vars map[string]json.RawMessage, name string) string {
	raw, ok := vars[name]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}
