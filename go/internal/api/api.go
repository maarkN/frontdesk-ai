// Package api is the core-api HTTP surface (REST /v1, EPIC-004): tenant
// onboarding, DIDs, calls, appointments, structured messages, usage and the
// answering-mode toggle.
//
// Auth is a per-tenant API key. The middleware resolves the key to a tenant
// and injects it into the context via tenantctx — the single resolution
// point of this service (ADR-005). Unknown key → 401; there is never a
// default tenant. Store interfaces are small, per aggregate, and defined
// here, in the consumer (uber-go/guide).
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/maarkn/frontdesk/internal/billing"
	"github.com/maarkn/frontdesk/internal/domain"
)

// TenantStore persists tenants. CreateTenant and TenantByAPIKeyHash operate
// above the resolution point; every other method takes the tenant from the
// context only.
type TenantStore interface {
	CreateTenant(ctx context.Context, t domain.Tenant) error
	TenantByAPIKeyHash(ctx context.Context, keyHash string) (domain.Tenant, error)
	CurrentTenant(ctx context.Context) (domain.Tenant, error)
	UpdateTenantMode(ctx context.Context, mode domain.Mode, overflowSeconds int) error
}

// DIDStore persists the context tenant's DIDs.
type DIDStore interface {
	AddDID(ctx context.Context, d domain.DID) error
	ListDIDs(ctx context.Context) ([]domain.DID, error)
}

// CallStore reads the context tenant's call projections.
type CallStore interface {
	ListCalls(ctx context.Context) ([]domain.Call, error)
	GetCall(ctx context.Context, callID string) (domain.Call, error)
}

// AppointmentStore persists the context tenant's appointments.
type AppointmentStore interface {
	CreateAppointment(ctx context.Context, a domain.Appointment) error
	ListAppointments(ctx context.Context) ([]domain.Appointment, error)
}

// MessageStore persists the context tenant's structured messages.
type MessageStore interface {
	CreateMessage(ctx context.Context, m domain.Message) error
	ListMessages(ctx context.Context) ([]domain.Message, error)
}

// Config carries the Server dependencies. All fields are required unless
// noted.
type Config struct {
	Tenants      TenantStore
	DIDs         DIDStore
	Calls        CallStore
	Appointments AppointmentStore
	Messages     MessageStore

	// Extractor powers "import from my website" during onboarding.
	Extractor domain.SiteExtractor
	// Billing creates trial subscriptions and reports them.
	Billing billing.Gateway

	// Now is the clock; time.Now when nil.
	Now func() time.Time
}

// Server is the core-api HTTP handler set.
type Server struct {
	cfg Config
	now func() time.Time
}

// New wires a Server from its dependencies.
func New(cfg Config) *Server {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Server{cfg: cfg, now: now}
}

// Router builds the chi router with the /v1 REST surface.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Route("/v1", func(r chi.Router) {
		// Self-service onboarding happens before any credential exists.
		r.Post("/tenants", s.handleCreateTenant)

		r.Group(func(r chi.Router) {
			r.Use(s.withTenant)

			r.Get("/tenants/me", s.handleGetTenant)
			r.Put("/tenants/me/mode", s.handleUpdateMode)

			r.Post("/dids", s.handleAddDID)
			r.Get("/dids", s.handleListDIDs)

			r.Get("/calls", s.handleListCalls)
			r.Get("/calls/{callID}", s.handleGetCall)

			r.Post("/appointments", s.handleCreateAppointment)
			r.Get("/appointments", s.handleListAppointments)

			r.Post("/messages", s.handleCreateMessage)
			r.Get("/messages", s.handleListMessages)

			r.Get("/usage", s.handleGetUsage)
		})
	})
	return r
}
