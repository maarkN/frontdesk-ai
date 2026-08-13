package domain

import (
	"fmt"
	"strings"
	"time"
)

// DID is a phone number provisioned for a tenant. The DID is the tenant
// resolution key at call start (ADR-005): unknown DID means hangup, never a
// default tenant.
type DID struct {
	// Number is the E.164 phone number, e.g. "+15145550100".
	Number string `json:"number"`
	// Provider is the carrier that owns the number ("telnyx" in the MVP).
	Provider string `json:"provider"`
	// Active gates whether calls to this number reach the agent.
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"createdAt"`
}

// Validate checks the DID shape (E.164-lite: "+" followed by 8-15 digits).
func (d DID) Validate() error {
	if !strings.HasPrefix(d.Number, "+") {
		return fmt.Errorf("did: number %q must be E.164 (start with +)", d.Number)
	}
	digits := d.Number[1:]
	if len(digits) < 8 || len(digits) > 15 {
		return fmt.Errorf("did: number %q must have 8-15 digits", d.Number)
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return fmt.Errorf("did: number %q has non-digit characters", d.Number)
		}
	}
	if d.Provider == "" {
		return fmt.Errorf("did: missing provider")
	}
	return nil
}
