import { z } from "zod";

import { isoDateTimeSchema } from "./common";
import { planSchema } from "./tenant";

/**
 * Billable usage of a call (or a summed period), derived by fold over the
 * event stream (Go: event.Usage).
 */
export const usageSchema = z.object({
  /** Billed duration, rounded up to whole minutes. */
  callMinutes: z.number().int(),
  /** Total transcribed audio, in seconds. */
  sttSeconds: z.number(),
  llmTokensIn: z.number().int(),
  llmTokensOut: z.number().int(),
  ttsChars: z.number().int(),
});
export type Usage = z.infer<typeof usageSchema>;

/** Stripe lifecycle states the MVP cares about (Go: billing.SubscriptionStatus). */
export const subscriptionStatusSchema = z.enum(["trialing", "active", "canceled"]);
export type SubscriptionStatus = z.infer<typeof subscriptionStatusSchema>;

/** Platform view of a payment-provider subscription (Go: billing.Subscription). */
export const subscriptionSchema = z.object({
  id: z.string(),
  plan: planSchema,
  status: subscriptionStatusSchema,
  trialEndsAt: isoDateTimeSchema.optional(),
});
export type Subscription = z.infer<typeof subscriptionSchema>;

/** Invoice preview for the current period (Go: billing.Invoice). */
export const invoiceSchema = z.object({
  plan: planSchema,
  usage: usageSchema,
  baseCents: z.number().int(),
  includedMinutes: z.number().int(),
  overageMinutes: z.number().int(),
  overageCents: z.number().int(),
  totalCents: z.number().int(),
  /** True while the 14-day trial runs (total is zero). */
  trial: z.boolean(),
});
export type Invoice = z.infer<typeof invoiceSchema>;

/** GET /v1/usage response (Go: api.usageResponse). */
export const usageResponseSchema = z.object({
  usage: usageSchema,
  invoice: invoiceSchema,
});
export type UsageResponse = z.infer<typeof usageResponseSchema>;
