/** Locale-aware formatting helpers (dates, durations, money, phone). */
import type { Locale } from "@frontdesk/shared";

export function formatDateTime(iso: string, locale: Locale): string {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(iso));
}

export function formatTime(iso: string, locale: Locale): string {
  return new Intl.DateTimeFormat(locale, { timeStyle: "short" }).format(
    new Date(iso),
  );
}

export function formatDate(iso: string, locale: Locale): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium" }).format(
    new Date(iso),
  );
}

/** "3:24" from start/end timestamps; "—" when either side is missing. */
export function formatCallDuration(
  startedAt: string | undefined,
  endedAt: string | undefined,
): string {
  if (startedAt === undefined || endedAt === undefined) {
    return "—";
  }
  const totalS = Math.max(
    0,
    Math.round((Date.parse(endedAt) - Date.parse(startedAt)) / 1000),
  );
  const m = Math.floor(totalS / 60);
  const s = totalS % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}

export function formatMs(ms: number): string {
  return ms >= 1000 ? `${(ms / 1000).toFixed(2)} s` : `${ms} ms`;
}

export function formatCad(cents: number, locale: Locale): string {
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency: "CAD",
  }).format(cents / 100);
}

export function formatPercent(ratio: number, locale: Locale): string {
  return new Intl.NumberFormat(locale, {
    style: "percent",
    maximumFractionDigits: 0,
  }).format(ratio);
}

/** "+15145550100" -> "+1 514 555 0100" (display only; API stays E.164). */
export function formatPhone(e164: string): string {
  const m = /^\+1(\d{3})(\d{3})(\d{4})$/.exec(e164);
  return m !== null ? `+1 ${m[1]} ${m[2]} ${m[3]}` : e164;
}

/** p50 of a non-empty list; null for an empty one. */
export function p50(values: number[]): number | null {
  if (values.length === 0) {
    return null;
  }
  const sorted = [...values].sort((a, b) => a - b);
  const mid = Math.floor(sorted.length / 2);
  const value =
    sorted.length % 2 === 1
      ? sorted[mid]
      : ((sorted[mid - 1] ?? 0) + (sorted[mid] ?? 0)) / 2;
  return value ?? null;
}
