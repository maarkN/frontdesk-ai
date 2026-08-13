package telnyx

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

func TestSendPostsV2Message(t *testing.T) {
	var got struct {
		auth string
		body map[string]string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.auth = r.Header.Get("Authorization")
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v2/messages", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got.body))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New("key-123", "+15140009999", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	require.NoError(t, c.Send(context.Background(), notify.SMS{To: "+15145551234", Body: "hello"}))

	assert.Equal(t, "Bearer key-123", got.auth)
	assert.Equal(t, map[string]string{
		"from": "+15140009999",
		"to":   "+15145551234",
		"text": "hello",
	}, got.body)
}

func TestSendCarrierErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "quota exceeded", http.StatusPaymentRequired)
	}))
	defer srv.Close()

	c := New("key", "+15140009999", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	err := c.Send(context.Background(), notify.SMS{To: "+15145551234", Body: "hi"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 402")
}
