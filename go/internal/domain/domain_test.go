package domain

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/maarkn/frontdesk/internal/event"
)

func TestBusinessHoursIsOpen(t *testing.T) {
	hours := BusinessHours{
		"mon": {{Open: "08:00", Close: "12:00"}, {Open: "13:00", Close: "17:00"}},
	}
	require.NoError(t, hours.Validate())

	monday := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC) // a Monday
	assert.True(t, hours.IsOpen(monday.Add(9*time.Hour)))
	assert.False(t, hours.IsOpen(monday.Add(12*time.Hour+30*time.Minute)), "lunch break is closed")
	assert.True(t, hours.IsOpen(monday.Add(16*time.Hour+59*time.Minute)))
	assert.False(t, hours.IsOpen(monday.Add(17*time.Hour)), "close bound is exclusive")
	assert.False(t, hours.IsOpen(monday.AddDate(0, 0, 1)), "unconfigured day is closed")

	assert.Error(t, BusinessHours{"monday": nil}.Validate(), "unknown day key")
	assert.Error(t, BusinessHours{"mon": {{Open: "18:00", Close: "09:00"}}}.Validate())
}

func TestCoverageAreaCovers(t *testing.T) {
	area := CoverageArea{PostalPrefixes: []string{"H2X", "H3A"}}
	assert.True(t, area.Covers("H2X 1Y4"))
	assert.True(t, area.Covers("h3a2b2"), "case and spacing are normalized")
	assert.False(t, area.Covers("J4W 2T4"))
	assert.True(t, CoverageArea{}.Covers("K1A 0A6"), "empty area means no restriction")
}

func TestTenantValidate(t *testing.T) {
	valid := Tenant{
		ID: "t1", Name: "Bob's Plumbing", Plan: PlanPro,
		Mode: ModeOverflow, OverflowSeconds: 15,
		OwnerMobile: "+15145550100", DefaultLocale: event.LocaleFRCA,
	}
	assert.NoError(t, valid.Validate())

	overflowWithoutDelay := valid
	overflowWithoutDelay.OverflowSeconds = 0
	assert.Error(t, overflowWithoutDelay.Validate())

	badPlan := valid
	badPlan.Plan = "enterprise"
	assert.Error(t, badPlan.Validate())
}

func TestMessageValidate(t *testing.T) {
	valid := Message{
		ID: "m1", Who: "Alice", What: "leak", Urgency: UrgencyHigh, CallbackPhone: "+15145550111",
	}
	assert.NoError(t, valid.Validate())

	noPhone := valid
	noPhone.CallbackPhone = ""
	assert.Error(t, noPhone.Validate(), "a structured message always carries a callback phone")

	badUrgency := valid
	badUrgency.Urgency = "asap"
	assert.Error(t, badUrgency.Validate())
}

func TestStaticExtractorIsDeterministicAndOffline(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	ex := &StaticExtractor{Now: func() time.Time { return now }}

	p, err := ex.ExtractProfile(context.Background(), "https://www.bobs-plumbing.ca/services")
	require.NoError(t, err)
	assert.Equal(t, "bobs-plumbing.ca", p.BusinessName)
	assert.NotEmpty(t, p.Services)
	assert.NotEmpty(t, p.FAQs)
	assert.Equal(t, now, p.ExtractedAt)

	_, err = ex.ExtractProfile(context.Background(), "")
	assert.Error(t, err)
}
