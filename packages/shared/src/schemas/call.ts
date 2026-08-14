import { z } from "zod";

import { isoDateTimeSchema, localeSchema } from "./common";
import { usageSchema } from "./usage";

/** PII category of a redacted span (Go: event.RedactionKind). */
export const redactionKindSchema = z.enum(["card", "sin", "postal_code", "dob"]);
export type RedactionKind = z.infer<typeof redactionKindSchema>;

/**
 * PII span replaced in the emitted text (Go: event.Redaction). Start/End are
 * byte offsets into the ORIGINAL text; vault references the encrypted
 * original (crypto-shredding: deletion destroys the call DEK).
 */
export const redactionSchema = z.object({
  start: z.number().int(),
  end: z.number().int(),
  kind: redactionKindSchema,
  vault: z.string(),
});
export type Redaction = z.infer<typeof redactionSchema>;

/**
 * Derived call lifecycle state (Go: event.CallStatus; "" is StatusUnknown).
 */
export const callStatusSchema = z.enum([
  "",
  "started",
  "answered",
  "transferring",
  "transferred",
  "ended",
]);
export type CallStatus = z.infer<typeof callStatusSchema>;

/** Why a call ended (Go: event.EndReason). */
export const endReasonSchema = z.enum([
  "completed",
  "caller_hangup",
  "transferred",
  "voicemail",
  "error",
]);
export type EndReason = z.infer<typeof endReasonSchema>;

/** Who produced a transcript entry (Go: event.Speaker). */
export const speakerSchema = z.enum(["caller", "agent"]);
export type Speaker = z.infer<typeof speakerSchema>;

/**
 * One conversation turn as heard (Go: event.TranscriptEntry): caller turns
 * from stt.final (already redacted), agent turns from agent.turn.completed
 * (PlayoutTracker truncation — only what the caller actually heard).
 */
export const transcriptEntrySchema = z.object({
  seq: z.number().int(),
  ts: isoDateTimeSchema,
  speaker: speakerSchema,
  text: z.string(),
  redactions: z.array(redactionSchema).optional(),
  interrupted: z.boolean().optional(),
});
export type TranscriptEntry = z.infer<typeof transcriptEntrySchema>;

/** Latency decomposition of one agent turn, in ms (Go: event.TurnLatency). */
export const turnLatencySchema = z.object({
  vadEndToSttFinalMs: z.number().int().optional(),
  sttFinalToLlmFirstTokenMs: z.number().int().optional(),
  llmFirstTokenToTtsFirstChunkMs: z.number().int().optional(),
  perceivedMs: z.number().int().optional(),
});
export type TurnLatency = z.infer<typeof turnLatencySchema>;

/** Position on the degradation ladder (Go: event.Budget). */
export const budgetSchema = z.object({
  /** Last reported degradation stage; "normal" by default. */
  stage: z.string(),
  errorCount: z.number().int(),
});
export type Budget = z.infer<typeof budgetSchema>;

/**
 * State derived from a call's event stream — state = fold(events)
 * (Go: event.CallSnapshot).
 */
export const callSnapshotSchema = z.object({
  callId: z.string(),
  tenantId: z.string(),
  flowId: z.string(),
  flowVersion: z.number().int(),
  status: callStatusSchema,
  startedAt: isoDateTimeSchema.optional(),
  endedAt: isoDateTimeSchema.optional(),
  endReason: endReasonSchema.optional(),
  consentCaptured: z.boolean(),
  consentGranted: z.boolean(),
  recording: z.boolean(),
  /** Active conversation locale. */
  locale: localeSchema.optional(),
  /** Every detection and mid-call switch, in order. */
  localeHistory: z.array(localeSchema).optional(),
  /** Current flow node. */
  cursor: z.string().optional(),
  /** Every node entered, in order. */
  path: z.array(z.string()).optional(),
  /** Flow variables; textual values already redacted. */
  vars: z.record(z.string(), z.unknown()).optional(),
  transcript: z.array(transcriptEntrySchema).optional(),
  /** Latency decomposition per completed agent turn, in turn order. */
  turnLatencies: z.array(turnLatencySchema).optional(),
  budget: budgetSchema,
  usage: usageSchema,
  /** Highest folded sequence number. */
  lastSeq: z.number().int(),
});
export type CallSnapshot = z.infer<typeof callSnapshotSchema>;

/**
 * Read model of one phone call: folded snapshot + recording location
 * (Go: domain.Call).
 */
export const callSchema = z.object({
  snapshot: callSnapshotSchema,
  /** DEK-encrypted audio in object storage; absent when not recorded. */
  recordingUrl: z.string().optional(),
});
export type Call = z.infer<typeof callSchema>;

/** List-view projection of a call (Go: api.callSummary). */
export const callSummarySchema = z.object({
  callId: z.string(),
  status: callStatusSchema,
  startedAt: isoDateTimeSchema.optional(),
  endedAt: isoDateTimeSchema.optional(),
  endReason: endReasonSchema.optional(),
  locale: localeSchema.optional(),
  callMinutes: z.number().int(),
  recorded: z.boolean(),
});
export type CallSummary = z.infer<typeof callSummarySchema>;
