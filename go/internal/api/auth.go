package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/maarkn/frontdesk/internal/domain"
	"github.com/maarkn/frontdesk/internal/tenantctx"
)

// apiKeyPrefix marks FrontDesk API keys ("fdk_<48 hex chars>").
const apiKeyPrefix = "fdk_"

// NewAPIKey generates a tenant API key. The plain key is returned exactly
// once, at onboarding; only its hash is stored.
func NewAPIKey() (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("api: generate key: %w", err)
	}
	return apiKeyPrefix + hex.EncodeToString(raw[:]), nil
}

// HashAPIKey returns the SHA-256 hex digest of a plain API key.
func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// withTenant authenticates the request by API key and injects the resolved
// tenant into the context (tenantctx). This is the single resolution point
// of the HTTP surface: an unknown or missing key is a 401, NEVER a fallback
// to a default tenant (ADR-005).
func (s *Server) withTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := bearerToken(r)
		if key == "" {
			writeError(w, http.StatusUnauthorized, "missing API key")
			return
		}
		tenant, err := s.cfg.Tenants.TenantByAPIKeyHash(r.Context(), HashAPIKey(key))
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "unknown API key")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "auth failed")
			return
		}
		ctx := tenantctx.WithTenant(r.Context(), tenant.ID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// bearerToken extracts the API key from "Authorization: Bearer <key>" or the
// X-API-Key header.
func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if rest, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(r.Header.Get("X-API-Key"))
}
