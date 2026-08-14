import { z } from "zod";

import { isoDateTimeSchema } from "./common";

/**
 * Triage level of a structured message (Go: domain.Urgency). Emergency also
 * triggers warm transfer rules during the call.
 */
export const urgencySchema = z.enum(["low", "normal", "high", "emergency"]);
export type Urgency = z.infer<typeof urgencySchema>;

/**
 * Structured note the agent takes when it does not book: who called, what
 * they need, how urgent, and how to call back (Go: domain.Message). It is
 * the payload of the <60s owner SMS.
 */
export const messageSchema = z.object({
  id: z.string(),
  /** Links back to the originating call. */
  callId: z.string().optional(),
  /** Caller's name as captured. */
  who: z.string(),
  /** Reason for the call. */
  what: z.string(),
  urgency: urgencySchema,
  /** Number to call back (E.164 when available). */
  callbackPhone: z.string(),
  createdAt: isoDateTimeSchema,
});
export type Message = z.infer<typeof messageSchema>;
