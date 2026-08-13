package domain

import (
	"errors"
	"fmt"
	"time"
)

// Appointment is a scheduled visit booked by the agent (or manually from the
// dashboard). Calendar sync (Google Calendar) happens elsewhere; this is the
// tenant-facing record.
type Appointment struct {
	ID string `json:"id"`
	// CallID links back to the call that produced the booking; empty for
	// manual entries.
	CallID string `json:"callId,omitempty"`

	CustomerName  string `json:"customerName"`
	CustomerPhone string `json:"customerPhone,omitempty"`
	// Service is what was requested ("water heater repair").
	Service string `json:"service"`
	// PostalCode is the visit location, checked against the coverage area.
	PostalCode string `json:"postalCode,omitempty"`

	StartsAt time.Time `json:"startsAt"`
	EndsAt   time.Time `json:"endsAt"`
	Notes    string    `json:"notes,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
}

// Validate checks the appointment invariants.
func (a Appointment) Validate() error {
	if a.ID == "" {
		return errors.New("appointment: missing id")
	}
	if a.CustomerName == "" {
		return errors.New("appointment: missing customerName")
	}
	if a.Service == "" {
		return errors.New("appointment: missing service")
	}
	if a.StartsAt.IsZero() || a.EndsAt.IsZero() {
		return errors.New("appointment: missing startsAt/endsAt")
	}
	if !a.StartsAt.Before(a.EndsAt) {
		return fmt.Errorf("appointment: startsAt %s must precede endsAt %s",
			a.StartsAt.Format(time.RFC3339), a.EndsAt.Format(time.RFC3339))
	}
	return nil
}
