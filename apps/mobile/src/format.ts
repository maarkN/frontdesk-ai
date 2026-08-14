import type { CallSummary, Locale } from "@frontdesk/shared";

/** "14:03" (en-CA: "2:03 p.m.") in the UI locale. */
export function formatTime(iso: string, locale: Locale): string {
  return new Intl.DateTimeFormat(locale, {
    hour: "numeric",
    minute: "2-digit",
  }).format(new Date(iso));
}

/** "Wed, Aug 13" / "mer. 13 août" in the UI locale. */
export function formatDay(iso: string, locale: Locale): string {
  return new Intl.DateTimeFormat(locale, {
    weekday: "short",
    month: "short",
    day: "numeric",
  }).format(new Date(iso));
}

/** Day + time, for appointments and call detail. */
export function formatDayTime(iso: string, locale: Locale): string {
  return `${formatDay(iso, locale)} · ${formatTime(iso, locale)}`;
}

/** Wall-clock duration "3:42" from a call's start/end; empty when unknown. */
export function formatCallDuration(summary: Pick<CallSummary, "startedAt" | "endedAt">): string {
  if (summary.startedAt === undefined || summary.endedAt === undefined) {
    return "";
  }
  const ms = new Date(summary.endedAt).getTime() - new Date(summary.startedAt).getTime();
  if (!Number.isFinite(ms) || ms < 0) {
    return "";
  }
  const totalSeconds = Math.round(ms / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

/** "0:07" style clock for the audio player, from seconds. */
export function formatClock(seconds: number): string {
  const safe = Number.isFinite(seconds) && seconds > 0 ? Math.floor(seconds) : 0;
  const minutes = Math.floor(safe / 60);
  const rest = safe % 60;
  return `${minutes}:${rest.toString().padStart(2, "0")}`;
}

/** "+15145550199" -> "(514) 555-0199"; other shapes pass through untouched. */
export function formatPhone(e164: string): string {
  const nanp = /^\+1(\d{3})(\d{3})(\d{4})$/.exec(e164);
  if (nanp !== null) {
    return `(${nanp[1] ?? ""}) ${nanp[2] ?? ""}-${nanp[3] ?? ""}`;
  }
  return e164;
}

/** True when the timestamp falls on the local calendar day of `now`. */
export function isSameLocalDay(iso: string, now: Date): boolean {
  const d = new Date(iso);
  return (
    d.getFullYear() === now.getFullYear() &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate()
  );
}

/** "EN" / "FR" chip text from a conversation locale. */
export function localeChip(locale: Locale): string {
  return locale === "fr-CA" ? "FR" : "EN";
}
