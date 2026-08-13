package telnyx_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/maarkn/frontdesk/internal/telnyx"
)

// recordedReq captures one request the fake Telnyx API received.
type recordedReq struct {
	Path string
	Auth string
	Body map[string]any
}

// newFakeAPI returns an httptest server standing in for the Telnyx API — the
// test stays fully local (no network).
func newFakeAPI(t *testing.T, status int) (*httptest.Server, *[]recordedReq) {
	t.Helper()
	var reqs []recordedReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		body := map[string]any{}
		require.NoError(t, json.Unmarshal(raw, &body))
		reqs = append(reqs, recordedReq{Path: r.URL.Path, Auth: r.Header.Get("Authorization"), Body: body})
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &reqs
}

func TestClientActions(t *testing.T) {
	srv, reqs := newFakeAPI(t, http.StatusOK)
	c := telnyx.NewClient("key-123", telnyx.WithBaseURL(srv.URL))
	ctx := context.Background()

	require.NoError(t, c.Answer(ctx, "cc-1"))
	require.NoError(t, c.Transfer(ctx, "cc-1", "+15145550123"))
	require.NoError(t, c.FlushPlayback(ctx, "cc-1"))
	require.NoError(t, c.Hangup(ctx, "cc-1"))

	require.Len(t, *reqs, 4)
	require.Equal(t, "/calls/cc-1/actions/answer", (*reqs)[0].Path)
	require.Equal(t, "Bearer key-123", (*reqs)[0].Auth)
	require.Equal(t, "/calls/cc-1/actions/transfer", (*reqs)[1].Path)
	require.Equal(t, "+15145550123", (*reqs)[1].Body["to"])
	require.Equal(t, "/calls/cc-1/actions/playback_stop", (*reqs)[2].Path)
	require.Equal(t, "all", (*reqs)[2].Body["stop"])
	require.Equal(t, "/calls/cc-1/actions/hangup", (*reqs)[3].Path)
}

func TestClientSurfacesAPIErrors(t *testing.T) {
	srv, _ := newFakeAPI(t, http.StatusUnprocessableEntity)
	c := telnyx.NewClient("key-123", telnyx.WithBaseURL(srv.URL))

	err := c.Answer(context.Background(), "cc-1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "422")
}

func TestFakeCallControlRecords(t *testing.T) {
	f := &telnyx.FakeCallControl{}
	ctx := context.Background()

	require.NoError(t, f.Answer(ctx, "cc-9"))
	require.NoError(t, f.Transfer(ctx, "cc-9", "+15145550199"))
	require.NoError(t, f.FlushPlayback(ctx, "cc-9"))

	require.Equal(t, []string{"cc-9"}, f.Answered())
	require.Equal(t, []telnyx.FakeTransfer{{CallControlID: "cc-9", To: "+15145550199"}}, f.Transfers())
	require.Equal(t, []string{"cc-9"}, f.Flushes())
}
