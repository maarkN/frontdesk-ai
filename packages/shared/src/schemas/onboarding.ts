import { z } from "zod";

import { isoDateTimeSchema } from "./common";

/** One question/answer pair grounding agent replies (Go: domain.FAQ). */
export const faqSchema = z.object({
  question: z.string(),
  answer: z.string(),
});
export type FAQ = z.infer<typeof faqSchema>;

/**
 * "Import from my website" result: business facts and FAQ extracted for
 * review before activation (Go: domain.OnboardingProfile).
 */
export const onboardingProfileSchema = z.object({
  websiteUrl: z.string(),
  businessName: z.string().optional(),
  services: z.array(z.string()).optional(),
  faqs: z.array(faqSchema).optional(),
  extractedAt: isoDateTimeSchema,
});
export type OnboardingProfile = z.infer<typeof onboardingProfileSchema>;
