package domain

import (
	"errors"
	"fmt"
	"time"
)

// Urgency classifies a message for the owner's triage. Emergency messages
// also trigger warm transfer rules during the call (RF1).
type Urgency string

// Urgency levels.
const (
	UrgencyLow       Urgency = "low"
	UrgencyNormal    Urgency = "normal"
	UrgencyHigh      Urgency = "high"
	UrgencyEmergency Urgency = "emergency"
)

// Valid reports whether u is a known urgency level.
func (u Urgency) Valid() bool {
	switch u {
	case UrgencyLow, UrgencyNormal, UrgencyHigh, UrgencyEmergency:
		return true
	default:
		return false
	}
}

// Message is the structured note the agent takes when it does not book:
// who called, what they need, how urgent, and how to call back. It is the
// payload of the <60s owner SMS (RF3).
type Message struct {
	ID string `json:"id"`
	// CallID links back to the originating call.
	CallID string `json:"callId,omitempty"`

	// Who is the caller's name as captured.
	Who string `json:"who"`
	// What is the reason for the call.
	What string `json:"what"`
	// Urgency is the triage level.
	Urgency Urgency `json:"urgency"`
	// CallbackPhone is the number to call back (E.164 when available).
	CallbackPhone string `json:"callbackPhone"`

	CreatedAt time.Time `json:"createdAt"`
}

// Validate checks the structured-message invariants.
func (m Message) Validate() error {
	if m.ID == "" {
		return errors.New("message: missing id")
	}
	if m.Who == "" {
		return errors.New("message: missing who")
	}
	if m.What == "" {
		return errors.New("message: missing what")
	}
	if !m.Urgency.Valid() {
		return fmt.Errorf("message: unknown urgency %q", m.Urgency)
	}
	if m.CallbackPhone == "" {
		return errors.New("message: missing callbackPhone")
	}
	return nil
}
