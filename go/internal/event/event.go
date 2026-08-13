// Package event defines the business-event envelope, the typed payload
// catalog, PII redaction at emission time, and the deterministic fold that
// derives call state from the stream (state = fold(events), ADR-003).
//
// The transport (NATS JetStream) is at-least-once and unordered, so every
// consumer deduplicates by (CallID, Seq) and orders by Seq before folding.
// stt.partial events never reach the bus: partials live only on the hot path
// between the gateway and the agent runtime.
package event

import (
	"crypto/rand"
	"encoding/json"
	"time"

	"github.com/oklog/ulid/v2"
)

// Type identifies an event in the v1 catalog.
type Type string

// Catalog of business events (contracts/events/v1). stt.partial is absent by
// decision (ADR-003): it must never be published to the bus.
const (
	TypeCallStarted        Type = "call.started"
	TypeCallAnswered       Type = "call.answered"
	TypeCallEnded          Type = "call.ended"
	TypeConsentCaptured    Type = "consent.captured"
	TypeLanguageDetected   Type = "language.detected"
	TypeLanguageSwitched   Type = "language.switched"
	TypeNodeEntered        Type = "node.entered"
	TypeNodeExited         Type = "node.exited"
	TypeSTTFinal           Type = "stt.final"
	TypeAgentTurnCompleted Type = "agent.turn.completed"
	TypeToolInvoked        Type = "tool.invoked"
	TypeToolCompleted      Type = "tool.completed"
	TypeVarAssigned        Type = "var.assigned"
	TypeDTMFReceived       Type = "dtmf.received"
	TypeTransferInitiated  Type = "transfer.initiated"
	TypeTransferAnswered   Type = "transfer.answered"
	TypeRecordingStarted   Type = "recording.started"
	TypeErrorRaised        Type = "error.raised"
)

// Locale is a supported conversation locale.
type Locale string

// Supported locales.
const (
	LocaleENCA Locale = "en-CA"
	LocaleFRCA Locale = "fr-CA"
)

// RedactionKind categorizes a redacted PII span.
type RedactionKind string

// PII categories detected at emission time.
const (
	KindCard       RedactionKind = "card"
	KindSIN        RedactionKind = "sin"
	KindPostalCode RedactionKind = "postal_code"
	KindDOB        RedactionKind = "dob"
)

// Redaction marks a PII span replaced in the emitted text. Start/End are byte
// offsets into the ORIGINAL text; Vault references the original content stored
// encrypted under the call DEK (crypto-shredding: deletion destroys the DEK).
type Redaction struct {
	Start int           `json:"start"`
	End   int           `json:"end"`
	Kind  RedactionKind `json:"kind"`
	Vault string        `json:"vault"`
}

// Envelope is the mandatory wrapper of every business event
// (contracts/events/v1/envelope.schema.json).
type Envelope struct {
	// EventID is a ULID: time-ordered, and the transport dedup key
	// (published as Nats-Msg-Id).
	EventID string `json:"eventId"`
	// CallID groups the event stream of one call.
	CallID string `json:"callId"`
	// Seq is monotonic per call, starting at 1. It is the consumer-side
	// dedup and ordering mechanism — NATS is at-least-once and unordered.
	Seq uint64 `json:"seq"`
	// TS is the emission instant (UTC).
	TS time.Time `json:"ts"`
	// TenantID is present on every event; billing is a fold per tenant.
	TenantID string `json:"tenantId"`
	// FlowID and FlowVersion identify the published flow that produced
	// the event (per-version analysis, comparable reprocessing).
	FlowID      string `json:"flowId"`
	FlowVersion int    `json:"flowVersion"`
	// Type selects the payload shape from the catalog.
	Type Type `json:"type"`
	// NodeID is the flow node where the event happened (empty when not
	// applicable).
	NodeID string `json:"nodeId,omitempty"`
	// Payload is the type-specific body, already redacted.
	Payload json.RawMessage `json:"payload"`
	// Redactions are the PII spans removed from Payload text at emission.
	Redactions []Redaction `json:"redactions,omitempty"`
}

// NewEventID returns a new ULID for use as Envelope.EventID.
func NewEventID(now time.Time) string {
	return ulid.MustNew(ulid.Timestamp(now.UTC()), rand.Reader).String()
}
