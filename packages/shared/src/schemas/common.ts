import { z } from "zod";

/**
 * RFC 3339 timestamp as emitted by Go's `time.Time` JSON marshalling
 * (e.g. "2026-08-13T14:03:00Z" or with a numeric offset / fractional seconds).
 */
export const isoDateTimeSchema = z.iso.datetime({ offset: true });
export type IsoDateTime = z.infer<typeof isoDateTimeSchema>;

/**
 * Supported conversation locales (Go: event.Locale — LocaleENCA/LocaleFRCA).
 */
export const localeSchema = z.enum(["en-CA", "fr-CA"]);
export type Locale = z.infer<typeof localeSchema>;
