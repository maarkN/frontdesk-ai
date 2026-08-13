package ses

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/maarkn/frontdesk/internal/notify"
)

func TestSendPostsSendEmail(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v2/email/outbound-emails", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New("no-reply@frontdesk.ai", "AKIA123", "secret",
		WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	require.NoError(t, c.Send(context.Background(), notify.Email{
		To:      "owner@plombex.ca",
		Subject: "Weekly report",
		Body:    "3 calls",
	}))

	assert.Equal(t, "no-reply@frontdesk.ai", got["FromEmailAddress"])
	dest := got["Destination"].(map[string]any)
	assert.Equal(t, []any{"owner@plombex.ca"}, dest["ToAddresses"])
}

func TestSendErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "throttled", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := New("no-reply@frontdesk.ai", "AKIA123", "secret",
		WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	err := c.Send(context.Background(), notify.Email{To: "x@y.ca"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 429")
}
