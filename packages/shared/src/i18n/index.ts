import type { Locale } from "../schemas/common";
import { en, type UiDictionary } from "./en";
import { fr } from "./fr";

export { en, fr };
export type { UiDictionary };

/**
 * Dictionaries keyed by the product locales (same values as the Go
 * event.Locale — "en-CA" / "fr-CA"). Ready to feed i18next resources:
 * `{ "en-CA": { translation: dictionaries["en-CA"] }, ... }`.
 */
export const dictionaries: Record<Locale, UiDictionary> = {
  "en-CA": en,
  "fr-CA": fr,
};

/** Default UI locale (the product is EN-first with full FR-CA parity). */
export const defaultUiLocale: Locale = "en-CA";
