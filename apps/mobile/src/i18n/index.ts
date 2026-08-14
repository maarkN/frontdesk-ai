import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import * as SecureStore from "expo-secure-store";

import {
  defaultUiLocale,
  dictionaries,
  localeSchema,
  type Locale,
} from "@frontdesk/shared";

import { mobileEn, mobileFr } from "./mobile";

const LOCALE_STORE_KEY = "frontdesk.uiLocale";

const resources = {
  "en-CA": { translation: dictionaries["en-CA"], mobile: mobileEn },
  "fr-CA": { translation: dictionaries["fr-CA"], mobile: mobileFr },
} as const;

/**
 * Initializes i18next once with the shared EN/FR-CA dictionaries plus the
 * mobile namespace, restoring the persisted language choice. Awaited by the
 * root layout before rendering.
 */
export async function initI18n(): Promise<void> {
  if (i18n.isInitialized) {
    return;
  }
  let lng: Locale = defaultUiLocale;
  try {
    const stored = await SecureStore.getItemAsync(LOCALE_STORE_KEY);
    const parsed = localeSchema.safeParse(stored);
    if (parsed.success) {
      lng = parsed.data;
    }
  } catch {
    // First launch or store unavailable — fall back to the default locale.
  }
  await i18n.use(initReactI18next).init({
    resources,
    lng,
    fallbackLng: defaultUiLocale,
    ns: ["translation", "mobile"],
    defaultNS: "translation",
    interpolation: { escapeValue: false },
  });
}

/** Switches the UI language and persists the choice. */
export async function setUiLocale(locale: Locale): Promise<void> {
  await i18n.changeLanguage(locale);
  await SecureStore.setItemAsync(LOCALE_STORE_KEY, locale);
}

/** The active UI locale (always one of the two product locales). */
export function currentUiLocale(): Locale {
  const parsed = localeSchema.safeParse(i18n.language);
  return parsed.success ? parsed.data : defaultUiLocale;
}
