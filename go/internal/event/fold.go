package event

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// CallStatus is the derived lifecycle state of a call.
type CallStatus string

// Call lifecycle states derived by the fold.
const (
	StatusUnknown      CallStatus = ""
	StatusStarted      CallStatus = "started"
	StatusAnswered     CallStatus = "answered"
	StatusTransferring CallStatus = "transferring"
	StatusTransferred  CallStatus = "transferred"
	StatusEnded        CallStatus = "ended"
)

// Speaker identifies who produced a transcript entry.
type Speaker string

// Transcript speakers.
const (
	SpeakerCaller Speaker = "caller"
	SpeakerAgent  Speaker = "agent"
)

// TranscriptEntry is one turn of the conversation as heard: caller turns come
// from stt.final (already redacted), agent turns from agent.turn.completed
// (PlayoutTracker truncation — only what the caller actually heard).
type TranscriptEntry struct {
	Seq         uint64      `json:"seq"`
	TS          time.Time   `json:"ts"`
	Speaker     Speaker     `json:"speaker"`
	Text        string      `json:"text"`
	Redactions  []Redaction `json:"redactions,omitempty"`
	Interrupted bool        `json:"interrupted,omitempty"`
}

// Budget tracks where the call sits on the degradation ladder (ADR-006).
type Budget struct {
	// Stage is the last reported degradation stage; "normal" by default.
	Stage string `json:"stage"`
	// ErrorCount is the number of error.raised events folded so far.
	ErrorCount int `json:"errorCount"`
}

// Usage is the billable usage of a call, derived by fold over the same stream
// that feeds everything else — billing has no separate pipeline (ADR-003).
type Usage struct {
	// CallMinutes is the billed duration, rounded up to whole minutes.
	CallMinutes int64 `json:"callMinutes"`
	// STTSeconds is the total transcribed audio, in seconds.
	STTSeconds   float64 `json:"sttSeconds"`
	LLMTokensIn  int64   `json:"llmTokensIn"`
	LLMTokensOut int64   `json:"llmTokensOut"`
	TTSChars     int64   `json:"ttsChars"`
}

// CallSnapshot is the state derived from a call's event stream. It is also
// the rehydration mechanism after a restart: fold the stream, resume the call.
type CallSnapshot struct {
	CallID      string     `json:"callId"`
	TenantID    string     `json:"tenantId"`
	FlowID      string     `json:"flowId"`
	FlowVersion int        `json:"flowVersion"`
	Status      CallStatus `json:"status"`

	StartedAt time.Time `json:"startedAt,omitzero"`
	EndedAt   time.Time `json:"endedAt,omitzero"`
	EndReason EndReason `json:"endReason,omitempty"`

	ConsentCaptured bool `json:"consentCaptured"`
	ConsentGranted  bool `json:"consentGranted"`
	Recording       bool `json:"recording"`

	// Locale is the active conversation locale; LocaleHistory records every
	// detection and mid-call switch, in order.
	Locale        Locale   `json:"locale,omitempty"`
	LocaleHistory []Locale `json:"localeHistory,omitempty"`

	// Cursor is the current flow node; Path records every node entered.
	Cursor string   `json:"cursor,omitempty"`
	Path   []string `json:"path,omitempty"`

	// Vars holds flow variables, values already redacted when textual.
	Vars map[string]json.RawMessage `json:"vars,omitempty"`

	Transcript []TranscriptEntry `json:"transcript,omitempty"`

	// TurnLatencies collects the latency decomposition of each completed
	// agent turn (VadEndToSTTFinalMs etc.), in turn order.
	TurnLatencies []TurnLatency `json:"turnLatencies,omitempty"`

	Budget Budget `json:"budget"`
	Usage  Usage  `json:"usage"`

	// LastSeq is the highest folded sequence number (dedup floor for live
	// consumers resuming from a snapshot).
	LastSeq uint64 `json:"lastSeq"`
}

// Fold derives a CallSnapshot from a call's events. It is pure and
// deterministic: events are ordered by Seq (ties broken by EventID) and
// duplicated Seqs are dropped, so the same set of events — in any arrival
// order, with any duplication — always yields the same snapshot. This is the
// consumer-side contract for an at-least-once, unordered bus.
func Fold(events []Envelope) (CallSnapshot, error) {
	ordered := make([]Envelope, len(events))
	copy(ordered, events)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Seq != ordered[j].Seq {
			return ordered[i].Seq < ordered[j].Seq
		}
		return ordered[i].EventID < ordered[j].EventID
	})

	snap := CallSnapshot{Budget: Budget{Stage: "normal"}}
	var lastApplied uint64
	for i := range ordered {
		ev := &ordered[i]
		if ev.Seq == lastApplied && lastApplied != 0 {
			continue // duplicate delivery
		}
		if err := snap.apply(ev); err != nil {
			return CallSnapshot{}, err
		}
		lastApplied = ev.Seq
		snap.LastSeq = ev.Seq
	}
	return snap, nil
}

// apply folds one event into the snapshot.
func (s *CallSnapshot) apply(ev *Envelope) error {
	// Envelope-level identity: first event fixes it, later events agree by
	// construction (same call stream).
	if s.CallID == "" {
		s.CallID = ev.CallID
		s.TenantID = ev.TenantID
		s.FlowID = ev.FlowID
		s.FlowVersion = ev.FlowVersion
	}

	switch ev.Type {
	case TypeCallStarted:
		s.Status = StatusStarted
		s.StartedAt = ev.TS

	case TypeCallAnswered:
		s.Status = StatusAnswered

	case TypeCallEnded:
		var p CallEnded
		if err := unmarshalPayload(ev, &p); err != nil {
			return err
		}
		s.Status = StatusEnded
		s.EndedAt = ev.TS
		s.EndReason = p.Reason
		s.Usage.CallMinutes = ceilMinutes(p.DurationMs)

	case TypeConsentCaptured:
		var p ConsentCaptured
		if err := unmarshalPayload(ev, &p); err != nil {
			return err
		}
		s.ConsentCaptured = true
		s.ConsentGranted = p.Granted

	case TypeLanguageDetected:
		var p LanguageDetected
		if err := unmarshalPayload(ev, &p); err != nil {
			return err
		}
		s.Locale = p.Locale
		s.LocaleHistory = append(s.LocaleHistory, p.Locale)

	case TypeLanguageSwitched:
		var p LanguageSwitched
		if err := unmarshalPayload(ev, &p); err != nil {
			return err
		}
		s.Locale = p.To
		s.LocaleHistory = append(s.LocaleHistory, p.To)

	case TypeNodeEntered:
		s.Cursor = ev.NodeID
		s.Path = append(s.Path, ev.NodeID)

	case TypeNodeExited:
		// Cursor moves on the next node.entered; nothing to fold here.

	case TypeSTTFinal:
		var p STTFinal
		if err := unmarshalPayload(ev, &p); err != nil {
			return err
		}
		s.Transcript = append(s.Transcript, TranscriptEntry{
			Seq:        ev.Seq,
			TS:         ev.TS,
			Speaker:    SpeakerCaller,
			Text:       p.Text,
			Redactions: ev.Redactions,
		})
		s.Usage.STTSeconds += float64(p.AudioMs) / 1000

	case TypeAgentTurnCompleted:
		var p AgentTurnCompleted
		if err := unmarshalPayload(ev, &p); err != nil {
			return err
		}
		s.Transcript = append(s.Transcript, TranscriptEntry{
			Seq:         ev.Seq,
			TS:          ev.TS,
			Speaker:     SpeakerAgent,
			Text:        p.Text,
			Redactions:  ev.Redactions,
			Interrupted: p.Interrupted,
		})
		s.TurnLatencies = append(s.TurnLatencies, p.Latency)
		s.Usage.LLMTokensIn += p.LLMTokensIn
		s.Usage.LLMTokensOut += p.LLMTokensOut
		s.Usage.TTSChars += p.TTSChars

	case TypeToolInvoked, TypeToolCompleted, TypeDTMFReceived:
		// Folded into no snapshot field yet; kept in the stream for
		// timeline and analysis consumers.

	case TypeVarAssigned:
		var p VarAssigned
		if err := unmarshalPayload(ev, &p); err != nil {
			return err
		}
		if s.Vars == nil {
			s.Vars = make(map[string]json.RawMessage)
		}
		s.Vars[p.Name] = p.Value

	case TypeTransferInitiated:
		s.Status = StatusTransferring

	case TypeTransferAnswered:
		s.Status = StatusTransferred

	case TypeRecordingStarted:
		s.Recording = true

	case TypeErrorRaised:
		var p ErrorRaised
		if err := unmarshalPayload(ev, &p); err != nil {
			return err
		}
		s.Budget.ErrorCount++
		if p.Stage != "" {
			s.Budget.Stage = p.Stage
		}

	default:
		// Unknown types are skipped, not fatal: newer producers may emit
		// compatible additions within the same contract version.
	}
	return nil
}

func unmarshalPayload(ev *Envelope, dst any) error {
	if err := json.Unmarshal(ev.Payload, dst); err != nil {
		return fmt.Errorf("fold %s seq %d: unmarshal payload: %w", ev.Type, ev.Seq, err)
	}
	return nil
}

// ceilMinutes converts a duration in milliseconds to billed whole minutes,
// rounding up (per-minute billing).
func ceilMinutes(durationMs int64) int64 {
	if durationMs <= 0 {
		return 0
	}
	return (durationMs + 59_999) / 60_000
}
