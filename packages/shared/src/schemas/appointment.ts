import { z } from "zod";

import { isoDateTimeSchema } from "./common";

/**
 * Scheduled visit booked by the agent or manually from the dashboard
 * (Go: domain.Appointment).
 */
export const appointmentSchema = z.object({
  id: z.string(),
  /** Links back to the call that produced the booking; absent for manual entries. */
  callId: z.string().optional(),
  customerName: z.string(),
  customerPhone: z.string().optional(),
  /** What was requested ("water heater repair"). */
  service: z.string(),
  /** Visit location, checked against the coverage area. */
  postalCode: z.string().optional(),
  startsAt: isoDateTimeSchema,
  endsAt: isoDateTimeSchema,
  notes: z.string().optional(),
  createdAt: isoDateTimeSchema,
});
export type Appointment = z.infer<typeof appointmentSchema>;
