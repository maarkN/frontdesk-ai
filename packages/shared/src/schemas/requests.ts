import { z } from "zod";

import { isoDateTimeSchema, localeSchema } from "./common";
import { onboardingProfileSchema } from "./onboarding";
import { businessHoursSchema, modeSchema, planSchema, tenantSchema } from "./tenant";
import { subscriptionSchema } from "./usage";
import { urgencySchema } from "./message";

/** POST /v1/tenants body (Go: api.createTenantRequest). */
export const createTenantRequestSchema = z.object({
  name: z.string().min(1),
  plan: planSchema,
  mode: modeSchema.optional(),
  overflowSeconds: z.number().int().positive().optional(),
  websiteUrl: z.string().optional(),
  ownerMobile: z.string().min(1),
  defaultLocale: localeSchema.optional(),
  hours: businessHoursSchema.optional(),
  postalPrefixes: z.array(z.string()).optional(),
});
export type CreateTenantRequest = z.infer<typeof createTenantRequestSchema>;

/**
 * POST /v1/tenants response (Go: api.createTenantResponse). The apiKey is
 * shown exactly once — it is never retrievable again.
 */
export const createTenantResponseSchema = z.object({
  tenant: tenantSchema,
  apiKey: z.string(),
  profile: onboardingProfileSchema.optional(),
  subscription: subscriptionSchema,
});
export type CreateTenantResponse = z.infer<typeof createTenantResponseSchema>;

/** PUT /v1/tenants/me/mode body (Go: api.updateModeRequest). */
export const updateModeRequestSchema = z.object({
  mode: modeSchema,
  overflowSeconds: z.number().int().positive().optional(),
});
export type UpdateModeRequest = z.infer<typeof updateModeRequestSchema>;

/** POST /v1/dids body (Go: api.addDIDRequest). */
export const addDidRequestSchema = z.object({
  number: z.string().regex(/^\+\d{8,15}$/, "expected E.164 (+ then 8-15 digits)"),
  provider: z.string().optional(),
});
export type AddDidRequest = z.infer<typeof addDidRequestSchema>;

/** POST /v1/appointments body (Go: api.createAppointmentRequest). */
export const createAppointmentRequestSchema = z.object({
  callId: z.string().optional(),
  customerName: z.string().min(1),
  customerPhone: z.string().optional(),
  service: z.string().min(1),
  postalCode: z.string().optional(),
  startsAt: isoDateTimeSchema,
  endsAt: isoDateTimeSchema,
  notes: z.string().optional(),
});
export type CreateAppointmentRequest = z.infer<typeof createAppointmentRequestSchema>;

/** POST /v1/messages body (Go: api.createMessageRequest). */
export const createMessageRequestSchema = z.object({
  callId: z.string().optional(),
  who: z.string().min(1),
  what: z.string().min(1),
  urgency: urgencySchema.optional(),
  callbackPhone: z.string().min(1),
});
export type CreateMessageRequest = z.infer<typeof createMessageRequestSchema>;

/** Uniform error body of every non-2xx response (Go: api.errorBody). */
export const apiErrorBodySchema = z.object({
  error: z.string(),
});
export type ApiErrorBody = z.infer<typeof apiErrorBodySchema>;
