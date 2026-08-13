package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/maarkn/frontdesk/internal/billing"
	"github.com/maarkn/frontdesk/internal/domain"
	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/tenantctx"
)

// createTenantRequest is the self-service onboarding payload (RF4).
type createTenantRequest struct {
	Name            string               `json:"name"`
	Plan            domain.Plan          `json:"plan"`
	Mode            domain.Mode          `json:"mode,omitempty"`
	OverflowSeconds int                  `json:"overflowSeconds,omitempty"`
	WebsiteURL      string               `json:"websiteUrl,omitempty"`
	OwnerMobile     string               `json:"ownerMobile"`
	DefaultLocale   event.Locale         `json:"defaultLocale,omitempty"`
	Hours           domain.BusinessHours `json:"hours,omitempty"`
	PostalPrefixes  []string             `json:"postalPrefixes,omitempty"`
}

// createTenantResponse returns the tenant, its API key (shown exactly once),
// the extracted onboarding profile and the trialing subscription.
type createTenantResponse struct {
	Tenant       domain.Tenant             `json:"tenant"`
	APIKey       string                    `json:"apiKey"`
	Profile      *domain.OnboardingProfile `json:"profile,omitempty"`
	Subscription billing.Subscription      `json:"subscription"`
}

// handleCreateTenant runs the self-service onboarding: create the tenant,
// mint its API key, extract the website profile ("import from my site") and
// start the 14-day trial subscription.
func (s *Server) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	var req createTenantRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if req.Mode == "" {
		req.Mode = domain.ModeOverflow
	}
	if req.Mode == domain.ModeOverflow && req.OverflowSeconds == 0 {
		req.OverflowSeconds = 15
	}
	if req.DefaultLocale == "" {
		req.DefaultLocale = event.LocaleENCA
	}

	apiKey, err := NewAPIKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	now := s.now().UTC()
	tenant := domain.Tenant{
		ID:              domain.NewID(now),
		Name:            req.Name,
		Plan:            req.Plan,
		Mode:            req.Mode,
		OverflowSeconds: req.OverflowSeconds,
		Hours:           req.Hours,
		Coverage:        domain.CoverageArea{PostalPrefixes: req.PostalPrefixes},
		OwnerMobile:     req.OwnerMobile,
		DefaultLocale:   req.DefaultLocale,
		APIKeyHash:      HashAPIKey(apiKey),
		TrialEndsAt:     now.AddDate(0, 0, billing.TrialDays),
		CreatedAt:       now,
	}
	if err := tenant.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// "Import from my website": extracted before activation, reviewable by
	// the owner. A failure must not block onboarding.
	var profile *domain.OnboardingProfile
	if req.WebsiteURL != "" {
		if p, err := s.cfg.Extractor.ExtractProfile(r.Context(), req.WebsiteURL); err == nil {
			profile = &p
		}
	}

	if err := s.cfg.Tenants.CreateTenant(r.Context(), tenant); err != nil {
		writeStoreError(w, err, "tenant not found")
		return
	}

	// The tenant now exists: from here on it travels in the context only.
	ctx := tenantctx.WithTenant(r.Context(), tenant.ID)
	sub, err := s.cfg.Billing.CreateSubscription(ctx, tenant.Plan)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "subscription failed")
		return
	}

	writeJSON(w, http.StatusCreated, createTenantResponse{
		Tenant:       tenant,
		APIKey:       apiKey,
		Profile:      profile,
		Subscription: sub,
	})
}

// handleGetTenant returns the authenticated tenant.
func (s *Server) handleGetTenant(w http.ResponseWriter, r *http.Request) {
	tenant, err := s.cfg.Tenants.CurrentTenant(r.Context())
	if err != nil {
		writeStoreError(w, err, "tenant not found")
		return
	}
	writeJSON(w, http.StatusOK, tenant)
}

// updateModeRequest toggles overflow / always-AI (RF4 dashboard toggle).
type updateModeRequest struct {
	Mode            domain.Mode `json:"mode"`
	OverflowSeconds int         `json:"overflowSeconds,omitempty"`
}

// handleUpdateMode switches the answering mode of the tenant.
func (s *Server) handleUpdateMode(w http.ResponseWriter, r *http.Request) {
	var req updateModeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if !req.Mode.Valid() {
		writeError(w, http.StatusBadRequest, "unknown mode")
		return
	}
	if req.Mode == domain.ModeOverflow && req.OverflowSeconds <= 0 {
		req.OverflowSeconds = 15
	}
	if err := s.cfg.Tenants.UpdateTenantMode(r.Context(), req.Mode, req.OverflowSeconds); err != nil {
		writeStoreError(w, err, "tenant not found")
		return
	}
	tenant, err := s.cfg.Tenants.CurrentTenant(r.Context())
	if err != nil {
		writeStoreError(w, err, "tenant not found")
		return
	}
	writeJSON(w, http.StatusOK, tenant)
}

// addDIDRequest provisions a number for the tenant.
type addDIDRequest struct {
	Number   string `json:"number"`
	Provider string `json:"provider,omitempty"`
}

// handleAddDID provisions a DID.
func (s *Server) handleAddDID(w http.ResponseWriter, r *http.Request) {
	var req addDIDRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if req.Provider == "" {
		req.Provider = "telnyx"
	}
	did := domain.DID{
		Number:    req.Number,
		Provider:  req.Provider,
		Active:    true,
		CreatedAt: s.now().UTC(),
	}
	if err := did.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.cfg.DIDs.AddDID(r.Context(), did); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			writeError(w, http.StatusConflict, "number already provisioned")
			return
		}
		writeStoreError(w, err, "did not found")
		return
	}
	writeJSON(w, http.StatusCreated, did)
}

// handleListDIDs lists the tenant's DIDs.
func (s *Server) handleListDIDs(w http.ResponseWriter, r *http.Request) {
	dids, err := s.cfg.DIDs.ListDIDs(r.Context())
	if err != nil {
		writeStoreError(w, err, "dids not found")
		return
	}
	writeJSON(w, http.StatusOK, dids)
}

// callSummary is the list-view projection of a call.
type callSummary struct {
	CallID      string           `json:"callId"`
	Status      event.CallStatus `json:"status"`
	StartedAt   time.Time        `json:"startedAt,omitzero"`
	EndedAt     time.Time        `json:"endedAt,omitzero"`
	EndReason   event.EndReason  `json:"endReason,omitempty"`
	Locale      event.Locale     `json:"locale,omitempty"`
	CallMinutes int64            `json:"callMinutes"`
	Recorded    bool             `json:"recorded"`
}

// handleListCalls lists call summaries, most recent first.
func (s *Server) handleListCalls(w http.ResponseWriter, r *http.Request) {
	calls, err := s.cfg.Calls.ListCalls(r.Context())
	if err != nil {
		writeStoreError(w, err, "calls not found")
		return
	}
	summaries := make([]callSummary, 0, len(calls))
	for _, c := range calls {
		summaries = append(summaries, callSummary{
			CallID:      c.Snapshot.CallID,
			Status:      c.Snapshot.Status,
			StartedAt:   c.Snapshot.StartedAt,
			EndedAt:     c.Snapshot.EndedAt,
			EndReason:   c.Snapshot.EndReason,
			Locale:      c.Snapshot.Locale,
			CallMinutes: c.Snapshot.Usage.CallMinutes,
			Recorded:    c.RecordingURL != "",
		})
	}
	writeJSON(w, http.StatusOK, summaries)
}

// handleGetCall returns the call detail: full folded snapshot (transcript,
// locale history, latencies, usage) plus the recording URL.
func (s *Server) handleGetCall(w http.ResponseWriter, r *http.Request) {
	call, err := s.cfg.Calls.GetCall(r.Context(), chi.URLParam(r, "callID"))
	if err != nil {
		writeStoreError(w, err, "call not found")
		return
	}
	writeJSON(w, http.StatusOK, call)
}

// createAppointmentRequest books a visit manually from the dashboard.
type createAppointmentRequest struct {
	CallID        string    `json:"callId,omitempty"`
	CustomerName  string    `json:"customerName"`
	CustomerPhone string    `json:"customerPhone,omitempty"`
	Service       string    `json:"service"`
	PostalCode    string    `json:"postalCode,omitempty"`
	StartsAt      time.Time `json:"startsAt"`
	EndsAt        time.Time `json:"endsAt"`
	Notes         string    `json:"notes,omitempty"`
}

// handleCreateAppointment stores an appointment.
func (s *Server) handleCreateAppointment(w http.ResponseWriter, r *http.Request) {
	var req createAppointmentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	apt := domain.Appointment{
		ID:            domain.NewID(s.now()),
		CallID:        req.CallID,
		CustomerName:  req.CustomerName,
		CustomerPhone: req.CustomerPhone,
		Service:       req.Service,
		PostalCode:    req.PostalCode,
		StartsAt:      req.StartsAt,
		EndsAt:        req.EndsAt,
		Notes:         req.Notes,
		CreatedAt:     s.now().UTC(),
	}
	if err := apt.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.cfg.Appointments.CreateAppointment(r.Context(), apt); err != nil {
		writeStoreError(w, err, "appointment not found")
		return
	}
	writeJSON(w, http.StatusCreated, apt)
}

// handleListAppointments lists appointments by start time.
func (s *Server) handleListAppointments(w http.ResponseWriter, r *http.Request) {
	apts, err := s.cfg.Appointments.ListAppointments(r.Context())
	if err != nil {
		writeStoreError(w, err, "appointments not found")
		return
	}
	writeJSON(w, http.StatusOK, apts)
}

// createMessageRequest records a structured message: who called, what they
// need, urgency and callback number (RF3).
type createMessageRequest struct {
	CallID        string         `json:"callId,omitempty"`
	Who           string         `json:"who"`
	What          string         `json:"what"`
	Urgency       domain.Urgency `json:"urgency,omitempty"`
	CallbackPhone string         `json:"callbackPhone"`
}

// handleCreateMessage stores a structured message.
func (s *Server) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	var req createMessageRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if req.Urgency == "" {
		req.Urgency = domain.UrgencyNormal
	}
	msg := domain.Message{
		ID:            domain.NewID(s.now()),
		CallID:        req.CallID,
		Who:           req.Who,
		What:          req.What,
		Urgency:       req.Urgency,
		CallbackPhone: req.CallbackPhone,
		CreatedAt:     s.now().UTC(),
	}
	if err := msg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.cfg.Messages.CreateMessage(r.Context(), msg); err != nil {
		writeStoreError(w, err, "message not found")
		return
	}
	writeJSON(w, http.StatusCreated, msg)
}

// handleListMessages lists messages, most recent first.
func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	msgs, err := s.cfg.Messages.ListMessages(r.Context())
	if err != nil {
		writeStoreError(w, err, "messages not found")
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}

// usageResponse is the current-period usage and its invoice preview.
type usageResponse struct {
	Usage   event.Usage     `json:"usage"`
	Invoice billing.Invoice `json:"invoice"`
}

// handleGetUsage sums the usage folded into the tenant's call projections
// (billing is a fold over the same stream, ADR-003) and prices it with the
// tenant's plan; while the trial runs the total is zero.
func (s *Server) handleGetUsage(w http.ResponseWriter, r *http.Request) {
	tenant, err := s.cfg.Tenants.CurrentTenant(r.Context())
	if err != nil {
		writeStoreError(w, err, "tenant not found")
		return
	}
	calls, err := s.cfg.Calls.ListCalls(r.Context())
	if err != nil {
		writeStoreError(w, err, "calls not found")
		return
	}
	snapshots := make([]event.CallSnapshot, 0, len(calls))
	for _, c := range calls {
		snapshots = append(snapshots, c.Snapshot)
	}
	usage := billing.SumUsage(snapshots)
	inTrial := s.now().UTC().Before(tenant.TrialEndsAt)
	invoice, err := billing.Compute(tenant.Plan, usage, inTrial)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, usageResponse{Usage: usage, Invoice: invoice})
}
