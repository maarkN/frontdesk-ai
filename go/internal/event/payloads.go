package event

import (
	"encoding/json"
	"fmt"
	"time"
)

// CallMode is how the AI enters the call.
type CallMode string

// Call answering modes.
const (
	ModeOverflow CallMode = "overflow"
	ModeAlwaysAI CallMode = "always_ai"
)

// CallStarted is the payload of call.started.
type CallStarted struct {
	From string   `json:"from"`
	To   string   `json:"to"`
	Mode CallMode `json:"mode"`
}

// CallAnswered is the payload of call.answered.
type CallAnswered struct {
	AnsweredBy    string `json:"answeredBy"`
	AnswerDelayMs int    `json:"answerDelayMs,omitempty"`
}

// EndReason is why a call ended.
type EndReason string

// Call end reasons.
const (
	EndCompleted    EndReason = "completed"
	EndCallerHangup EndReason = "caller_hangup"
	EndTransferred  EndReason = "transferred"
	EndVoicemail    EndReason = "voicemail"
	EndError        EndReason = "error"
)

// CallEnded is the payload of call.ended.
type CallEnded struct {
	Reason     EndReason `json:"reason"`
	DurationMs int64     `json:"durationMs"`
}

// ConsentCaptured is the payload of consent.captured. Recording and the agent
// are gated on it (PIPEDA / Quebec Law 25).
type ConsentCaptured struct {
	Granted bool   `json:"granted"`
	Method  string `json:"method,omitempty"`
}

// LanguageDetected is the payload of language.detected.
type LanguageDetected struct {
	Locale     Locale  `json:"locale"`
	Confidence float64 `json:"confidence,omitempty"`
}

// LanguageSwitched is the payload of language.switched.
type LanguageSwitched struct {
	From Locale `json:"from"`
	To   Locale `json:"to"`
}

// NodeEntered is the payload of node.entered; the node id is on the envelope.
type NodeEntered struct {
	NodeType string `json:"nodeType,omitempty"`
}

// NodeExited is the payload of node.exited; the node id is on the envelope.
type NodeExited struct {
	Outcome string `json:"outcome,omitempty"`
}

// STTFinal is the payload of stt.final. Text is already redacted; the spans
// live in Envelope.Redactions.
type STTFinal struct {
	Text            string  `json:"text"`
	Locale          Locale  `json:"locale"`
	Confidence      float64 `json:"confidence,omitempty"`
	AudioMs         int64   `json:"audioMs,omitempty"`
	VadEndToFinalMs int64   `json:"vadEndToFinalMs,omitempty"`
}

// TurnLatency decomposes the perceived latency of one agent turn.
type TurnLatency struct {
	VadEndToSTTFinalMs             int64 `json:"vadEndToSttFinalMs,omitempty"`
	STTFinalToLLMFirstTokenMs      int64 `json:"sttFinalToLlmFirstTokenMs,omitempty"`
	LLMFirstTokenToTTSFirstChunkMs int64 `json:"llmFirstTokenToTtsFirstChunkMs,omitempty"`
	PerceivedMs                    int64 `json:"perceivedMs,omitempty"`
}

// AgentTurnCompleted is the payload of agent.turn.completed. Text is what the
// caller actually HEARD (PlayoutTracker truncation; barge-in cut marked "—").
type AgentTurnCompleted struct {
	Text         string      `json:"text"`
	Interrupted  bool        `json:"interrupted,omitempty"`
	LLMTokensIn  int64       `json:"llmTokensIn,omitempty"`
	LLMTokensOut int64       `json:"llmTokensOut,omitempty"`
	TTSChars     int64       `json:"ttsChars,omitempty"`
	Latency      TurnLatency `json:"latency,omitempty"`
}

// ToolInvoked is the payload of tool.invoked.
type ToolInvoked struct {
	Tool string          `json:"tool"`
	Args json.RawMessage `json:"args,omitempty"`
}

// ToolCompleted is the payload of tool.completed.
type ToolCompleted struct {
	Tool       string          `json:"tool"`
	OK         bool            `json:"ok"`
	DurationMs int64           `json:"durationMs,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
}

// VarAssigned is the payload of var.assigned. Value is already redacted when
// textual.
type VarAssigned struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

// DTMFReceived is the payload of dtmf.received.
type DTMFReceived struct {
	Digits string `json:"digits"`
}

// TransferReason is why a transfer was initiated.
type TransferReason string

// Transfer reasons.
const (
	TransferEmergency     TransferReason = "emergency"
	TransferKeyword       TransferReason = "keyword"
	TransferDegradation   TransferReason = "degradation"
	TransferCallerRequest TransferReason = "caller_request"
)

// TransferInitiated is the payload of transfer.initiated.
type TransferInitiated struct {
	Target string         `json:"target"`
	Reason TransferReason `json:"reason"`
}

// TransferAnswered is the payload of transfer.answered.
type TransferAnswered struct {
	Target string `json:"target"`
	WaitMs int64  `json:"waitMs,omitempty"`
}

// RecordingStarted is the payload of recording.started.
type RecordingStarted struct {
	RecordingID string `json:"recordingId"`
}

// ErrorRaised is the payload of error.raised.
type ErrorRaised struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Stage   string `json:"stage,omitempty"`
}

// New assembles an Envelope for a typed payload, marshaling it to JSON.
// Redactions must describe spans already applied to the payload text.
func New(
	callID string,
	seq uint64,
	ts time.Time,
	tenantID, flowID string,
	flowVersion int,
	typ Type,
	nodeID string,
	payload any,
	redactions []Redaction,
) (Envelope, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("marshal %s payload: %w", typ, err)
	}
	return Envelope{
		EventID:     NewEventID(ts),
		CallID:      callID,
		Seq:         seq,
		TS:          ts.UTC(),
		TenantID:    tenantID,
		FlowID:      flowID,
		FlowVersion: flowVersion,
		Type:        typ,
		NodeID:      nodeID,
		Payload:     body,
		Redactions:  redactions,
	}, nil
}
