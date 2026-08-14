import { z } from "zod";

import { isoDateTimeSchema, localeSchema } from "./common";

/** Subscription plan (Go: domain.Plan). */
export const planSchema = z.enum(["starter", "pro", "growth"]);
export type Plan = z.infer<typeof planSchema>;

/**
 * Answering mode (Go: domain.Mode, string values shared with event.CallMode):
 * overflow — the AI answers only if nobody picks up within `overflowSeconds`;
 * always_ai — the AI answers every call.
 */
export const modeSchema = z.enum(["overflow", "always_ai"]);
export type Mode = z.infer<typeof modeSchema>;

/** Open interval within a day, local "15:04" format (Go: domain.TimeRange). */
export const timeRangeSchema = z.object({
  open: z.string().regex(/^([01]\d|2[0-3]):[0-5]\d$/, "expected HH:MM"),
  close: z.string().regex(/^([01]\d|2[0-3]):[0-5]\d$/, "expected HH:MM"),
});
export type TimeRange = z.infer<typeof timeRangeSchema>;

/** Weekday keys used by BusinessHours (Go: domain.weekdayKeys). */
export const dayKeySchema = z.enum([
  "sun",
  "mon",
  "tue",
  "wed",
  "thu",
  "fri",
  "sat",
]);
export type DayKey = z.infer<typeof dayKeySchema>;

/**
 * Weekday -> open ranges; a missing/empty day is closed
 * (Go: domain.BusinessHours).
 */
export const businessHoursSchema = z.partialRecord(
  dayKeySchema,
  z.array(timeRangeSchema),
);
export type BusinessHours = z.infer<typeof businessHoursSchema>;

/**
 * Service restriction by Canadian FSA postal prefixes, e.g. "H2X"
 * (Go: domain.CoverageArea). Empty/absent prefixes means no restriction.
 */
export const coverageAreaSchema = z.object({
  postalPrefixes: z.array(z.string()).optional(),
});
export type CoverageArea = z.infer<typeof coverageAreaSchema>;

/**
 * One FrontDesk AI customer and its agent configuration
 * (Go: domain.Tenant — APIKeyHash is json:"-" and never leaves the server).
 */
export const tenantSchema = z.object({
  id: z.string(),
  name: z.string(),
  plan: planSchema,
  mode: modeSchema,
  /** Ring time before the AI picks up; only meaningful in overflow mode. */
  overflowSeconds: z.number().int().optional(),
  hours: businessHoursSchema.optional(),
  coverage: coverageAreaSchema.optional(),
  /** Receives warm transfers and post-call SMS (E.164). */
  ownerMobile: z.string(),
  defaultLocale: localeSchema,
  /** End of the 14-day Stripe trial; absent when not set. */
  trialEndsAt: isoDateTimeSchema.optional(),
  createdAt: isoDateTimeSchema,
});
export type Tenant = z.infer<typeof tenantSchema>;
