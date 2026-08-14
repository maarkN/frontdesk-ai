import { z } from "zod";

import { isoDateTimeSchema } from "./common";

/**
 * Phone number provisioned for a tenant (Go: domain.DID). The DID is the
 * tenant resolution key at call start; unknown DID means hangup.
 */
export const didSchema = z.object({
  /** E.164 number, e.g. "+15145550100". */
  number: z.string().regex(/^\+\d{8,15}$/, "expected E.164 (+ then 8-15 digits)"),
  /** Carrier that owns the number ("telnyx" in the MVP). */
  provider: z.string(),
  /** Gates whether calls to this number reach the agent. */
  active: z.boolean(),
  createdAt: isoDateTimeSchema,
});
export type DID = z.infer<typeof didSchema>;
