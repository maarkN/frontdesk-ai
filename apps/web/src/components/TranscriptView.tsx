/**
 * Transcript with redacted-PII highlighting. The server stores caller text
 * already redacted at emission (event.Mask): each PII span is replaced by a
 * literal "[redacted:<kind>]" token, and entry.redactions carries the kinds.
 * We render those tokens as labeled chips — the original never reaches the
 * browser (crypto-shredding keeps it server-side).
 */
import type { Locale, RedactionKind, TranscriptEntry } from "@frontdesk/shared";
import { Fragment, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { formatTime } from "../lib/format";

const TOKEN_RE = /\[redacted:(card|sin|postal_code|dob)\]/g;

function RedactedText({ text }: { text: string }) {
  const { t } = useTranslation();
  const parts: ReactNode[] = [];
  let last = 0;
  for (const match of text.matchAll(TOKEN_RE)) {
    if (match.index > last) {
      parts.push(text.slice(last, match.index));
    }
    const kind = match[1] as RedactionKind;
    parts.push(
      <mark
        key={`${match.index}`}
        className="mx-0.5 rounded bg-redaction px-1.5 py-0.5 text-xs font-medium text-redaction-ink"
        title={t("transcript.redacted")}
      >
        {t(`transcript.redactionKind.${kind}`)}
      </mark>,
    );
    last = match.index + match[0].length;
  }
  if (last < text.length) {
    parts.push(text.slice(last));
  }
  return (
    <>
      {parts.map((p, i) => (
        <Fragment key={i}>{p}</Fragment>
      ))}
    </>
  );
}

export function TranscriptView({
  entries,
  locale,
}: {
  entries: TranscriptEntry[];
  locale: Locale;
}) {
  const { t } = useTranslation();

  if (entries.length === 0) {
    return <p className="py-6 text-center text-sm text-muted">{t("transcript.empty")}</p>;
  }

  return (
    <ol className="space-y-3">
      {entries.map((entry) => {
        const isAgent = entry.speaker === "agent";
        return (
          <li key={entry.seq} className={`flex ${isAgent ? "justify-start" : "justify-end"}`}>
            <div
              className={`max-w-[85%] rounded-lg px-3 py-2 text-sm leading-relaxed ${
                isAgent ? "bg-surface-2" : "bg-accent/10"
              }`}
            >
              <div className="mb-0.5 flex items-baseline gap-2 text-xs text-muted">
                <span className="font-medium">
                  {t(isAgent ? "transcript.agent" : "transcript.caller")}
                </span>
                <time>{formatTime(entry.ts, locale)}</time>
                {entry.interrupted === true ? (
                  <span className="italic">({t("transcript.interrupted")})</span>
                ) : null}
              </div>
              <p className="whitespace-pre-wrap">
                <RedactedText text={entry.text} />
              </p>
            </div>
          </li>
        );
      })}
    </ol>
  );
}
