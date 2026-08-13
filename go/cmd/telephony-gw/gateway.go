package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/telnyx"
	"github.com/maarkn/frontdesk/internal/tenantctx"
)

// gateway handles Telnyx webhooks and owns the per-call sessions.
type gateway struct {
	cfg      config
	logger   *slog.Logger
	events   event.Publisher
	control  telnyx.CallControl
	sessions *sessionRegistry
}

// telnyxWebhook is the (subset of the) Telnyx event envelope we consume.
type telnyxWebhook struct {
	Data struct {
		EventType string `json:"event_type"`
		Payload   struct {
			CallControlID string `json:"call_control_id"`
			From          string `json:"from"`
			To            string `json:"to"`
		} `json:"payload"`
	} `json:"data"`
}

// handleWebhook is the call-control entry point. Tenant resolution happens
// HERE, once, from the DID; below this point the tenant travels only in the
// context (ADR-005 — no function takes tenantID as a parameter).
func (g *gateway) handleWebhook(w http.ResponseWriter, r *http.Request) {
	var hook telnyxWebhook
	if err := json.NewDecoder(r.Body).Decode(&hook); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}
	payload := hook.Data.Payload

	switch hook.Data.EventType {
	case "call.initiated":
		tenantID, ok := g.cfg.TenantByDID[payload.To]
		if !ok {
			// Unknown DID: hang up with a structured log, never route a
			// call without a tenant (ADR-005).
			g.logger.Warn("unknown DID, hanging up",
				"did", payload.To, "callControlID", payload.CallControlID)
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := g.control.Hangup(ctx, payload.CallControlID); err != nil {
				g.logger.Error("hangup failed", "err", err)
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		ctx := tenantctx.WithTenant(context.Background(), tenantID)
		g.startCall(ctx, payload.CallControlID, payload.From, payload.To)

	case "call.hangup":
		g.sessions.end(payload.CallControlID)
	}
	w.WriteHeader(http.StatusOK)
}

// startCall answers and registers the session. The turn.Machine attaches
// when the media WS for this call connects (the machine needs the frame
// stream; call control alone can answer, emit events and transfer).
func (g *gateway) startCall(ctx context.Context, callControlID, from, to string) {
	callCtx, cancel := context.WithCancel(ctx)
	sess := &session{id: callControlID, cancel: cancel}
	g.sessions.add(sess)

	g.sessions.wg.Go(func() {
		defer g.sessions.remove(sess.id)

		tenantID, err := tenantctx.FromContext(callCtx)
		if err != nil {
			g.logger.Error("session without tenant", "err", err)
			return
		}
		answerCtx, cancelAnswer := context.WithTimeout(callCtx, 2*time.Second)
		defer cancelAnswer()
		if err := g.control.Answer(answerCtx, callControlID); err != nil {
			g.logger.Error("answer failed", "callControlID", callControlID, "err", err)
			// Product floor (ADR-006 §5): if we cannot take the call, send
			// it to the owner's phone rather than leaving it ringing.
			if g.cfg.OwnerPhone != "" {
				if terr := g.control.Transfer(answerCtx, callControlID, g.cfg.OwnerPhone); terr != nil {
					g.logger.Error("failover transfer failed", "err", terr)
				}
			}
			return
		}

		now := time.Now()
		ev, err := event.New(callControlID, 1, now, tenantID, "", 0,
			event.TypeCallStarted, "",
			event.CallStarted{From: from, To: to, Mode: event.ModeOverflow}, nil)
		if err == nil {
			if perr := g.events.Publish(callCtx, ev); perr != nil {
				g.logger.Error("publish call.started", "err", perr)
			}
		}

		// Hold the session open until hangup or shutdown drain.
		<-callCtx.Done()
	})
}

// session is one active call.
type session struct {
	id     string
	cancel context.CancelFunc
}

// sessionRegistry tracks active calls so shutdown can drain them.
type sessionRegistry struct {
	mu       sync.Mutex
	sessions map[string]*session
	wg       sync.WaitGroup
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{sessions: make(map[string]*session)}
}

func (r *sessionRegistry) add(s *session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[s.id] = s
}

func (r *sessionRegistry) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
}

// end cancels one call's session (carrier reported hangup).
func (r *sessionRegistry) end(id string) {
	r.mu.Lock()
	s, ok := r.sessions[id]
	r.mu.Unlock()
	if ok {
		s.cancel()
	}
}

func (r *sessionRegistry) active() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sessions)
}

// drain waits for active calls to finish; on timeout it cancels the rest
// and reports false.
func (r *sessionRegistry) drain(ctx context.Context) bool {
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
	}
	// Timeout: cancel stragglers, then wait for their goroutines.
	r.mu.Lock()
	for _, s := range r.sessions {
		s.cancel()
	}
	r.mu.Unlock()
	<-done
	return false
}
