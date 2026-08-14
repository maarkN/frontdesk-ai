/**
 * i18next setup: the shared cross-platform dictionaries from
 * @frontdesk/shared feed the default "translation" namespace; web-only
 * strings live in the "app" namespace (this directory). UI is EN-first with
 * full FR-CA parity; the choice persists in localStorage.
 */
import { defaultUiLocale, dictionaries, localeSchema, type Locale } from "@frontdesk/shared";
import i18next from "i18next";
import { initReactI18next } from "react-i18next";

import { appEn } from "./app-en";
import { appFr } from "./app-fr";

const STORAGE_KEY = "frontdesk.uiLocale";

function storedLocale(): Locale {
  const parsed = localeSchema.safeParse(localStorage.getItem(STORAGE_KEY));
  return parsed.success ? parsed.data : defaultUiLocale;
}

export function setUiLocale(locale: Locale): void {
  localStorage.setItem(STORAGE_KEY, locale);
  void i18next.changeLanguage(locale);
  document.documentElement.lang = locale;
}

export const i18n = i18next;

void i18next.use(initReactI18next).init({
  lng: storedLocale(),
  fallbackLng: defaultUiLocale,
  interpolation: { escapeValue: false },
  resources: {
    "en-CA": { translation: dictionaries["en-CA"], app: appEn },
    "fr-CA": { translation: dictionaries["fr-CA"], app: appFr },
  },
});

document.documentElement.lang = storedLocale();
