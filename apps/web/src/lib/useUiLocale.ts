import { localeSchema, type Locale } from "@frontdesk/shared";
import { useTranslation } from "react-i18next";

/** Current UI locale as the shared Locale union (falls back to en-CA). */
export function useUiLocale(): Locale {
  const { i18n } = useTranslation();
  const parsed = localeSchema.safeParse(i18n.language);
  return parsed.success ? parsed.data : "en-CA";
}
