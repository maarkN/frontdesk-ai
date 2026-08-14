/**
 * Call list with typed search-param filters (outcome / language / urgency).
 * GET /v1/calls has no query params yet, so filtering is client-side; the
 * urgency filter joins calls to their structured message (callId link),
 * since urgency lives on domain.Message, not on the call.
 */
import { endReasonSchema, localeSchema, urgencySchema } from "@frontdesk/shared";
import type { EndReason, Locale, Urgency } from "@frontdesk/shared";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import { Badge, Card, EmptyState, ErrorState, PageHeader, Select, Spinner } from "../components/ui";
import { useAuth } from "../lib/auth";
import { formatCallDuration, formatDateTime } from "../lib/format";
import { callsQuery, messagesQuery } from "../lib/queries";
import { useUiLocale } from "../lib/useUiLocale";

/** Zod-validated search params — bad URLs degrade to "no filter". */
export const callsSearchSchema = z.object({
  outcome: endReasonSchema.optional().catch(undefined),
  lang: localeSchema.optional().catch(undefined),
  urgency: urgencySchema.optional().catch(undefined),
});
export type CallsSearch = z.infer<typeof callsSearchSchema>;

const OUTCOMES: EndReason[] = ["completed", "caller_hangup", "transferred", "voicemail", "error"];
const LOCALES: Locale[] = ["en-CA", "fr-CA"];
const URGENCIES: Urgency[] = ["low", "normal", "high", "emergency"];

export function CallsPage() {
  const { t } = useTranslation();
  const { client } = useAuth();
  const locale = useUiLocale();
  const navigate = useNavigate({ from: "/calls" });
  const search = useSearch({ from: "/authed/calls" });

  const calls = useQuery(callsQuery(client));
  const messages = useQuery(messagesQuery(client));

  if (calls.isPending) {
    return <Spinner />;
  }
  if (calls.isError) {
    return <ErrorState error={calls.error} onRetry={() => void calls.refetch()} />;
  }

  const urgencyByCall = new Map<string, Urgency>();
  for (const m of messages.data ?? []) {
    if (m.callId !== undefined) {
      urgencyByCall.set(m.callId, m.urgency);
    }
  }

  const filtered = calls.data.filter((c) => {
    if (search.outcome !== undefined && c.endReason !== search.outcome) {
      return false;
    }
    if (search.lang !== undefined && c.locale !== search.lang) {
      return false;
    }
    if (search.urgency !== undefined && urgencyByCall.get(c.callId) !== search.urgency) {
      return false;
    }
    return true;
  });

  const setFilter = (patch: Partial<CallsSearch>) => {
    void navigate({ search: (prev: CallsSearch) => ({ ...prev, ...patch }) });
  };
  const hasFilter =
    search.outcome !== undefined || search.lang !== undefined || search.urgency !== undefined;

  const urgencyTone = { low: "neutral", normal: "neutral", high: "warn", emergency: "danger" } as const;

  return (
    <div>
      <PageHeader
        title={t("calls.title")}
        actions={
          <>
            <Select
              ariaLabel={t("app:filters.outcome")}
              value={search.outcome ?? ""}
              onChange={(v) =>
                setFilter({ outcome: v === "" ? undefined : (v as EndReason) })
              }
            >
              <option value="">{`${t("app:filters.outcome")}: ${t("app:filters.any")}`}</option>
              {OUTCOMES.map((o) => (
                <option key={o} value={o}>
                  {t(`calls.endReason.${o}`)}
                </option>
              ))}
            </Select>
            <Select
              ariaLabel={t("app:filters.language")}
              value={search.lang ?? ""}
              onChange={(v) => setFilter({ lang: v === "" ? undefined : (v as Locale) })}
            >
              <option value="">{`${t("app:filters.language")}: ${t("app:filters.any")}`}</option>
              {LOCALES.map((l) => (
                <option key={l} value={l}>
                  {l}
                </option>
              ))}
            </Select>
            <Select
              ariaLabel={t("app:filters.urgency")}
              value={search.urgency ?? ""}
              onChange={(v) => setFilter({ urgency: v === "" ? undefined : (v as Urgency) })}
            >
              <option value="">{`${t("app:filters.urgency")}: ${t("app:filters.any")}`}</option>
              {URGENCIES.map((u) => (
                <option key={u} value={u}>
                  {t(`messages.urgencyLevel.${u}`)}
                </option>
              ))}
            </Select>
            {hasFilter ? (
              <button
                type="button"
                className="text-sm text-accent hover:underline"
                onClick={() =>
                  setFilter({ outcome: undefined, lang: undefined, urgency: undefined })
                }
              >
                {t("app:filters.clear")}
              </button>
            ) : null}
          </>
        }
      />

      {calls.data.length === 0 ? (
        <EmptyState message={t("calls.empty")} />
      ) : filtered.length === 0 ? (
        <EmptyState message={t("app:filters.noMatch")} />
      ) : (
        <Card className="overflow-x-auto p-0">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-edge text-left text-xs text-muted uppercase">
                <th className="px-4 py-2 font-medium">{t("calls.startedAt")}</th>
                <th className="px-4 py-2 font-medium">{t("app:filters.outcome")}</th>
                <th className="px-4 py-2 font-medium">{t("calls.language")}</th>
                <th className="px-4 py-2 font-medium">{t("app:filters.urgency")}</th>
                <th className="px-4 py-2 text-right font-medium">{t("calls.duration")}</th>
                <th className="px-4 py-2 text-right font-medium">{t("calls.recorded")}</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((call) => {
                const urgency = urgencyByCall.get(call.callId);
                return (
                  <tr key={call.callId} className="border-b border-edge last:border-0 hover:bg-surface-2">
                    <td className="px-4 py-2 whitespace-nowrap">
                      <Link
                        to="/calls/$callId"
                        params={{ callId: call.callId }}
                        className="font-medium text-accent hover:underline"
                      >
                        {call.startedAt !== undefined
                          ? formatDateTime(call.startedAt, locale)
                          : call.callId}
                      </Link>
                    </td>
                    <td className="px-4 py-2">
                      {call.endReason !== undefined
                        ? t(`calls.endReason.${call.endReason}`)
                        : t(`calls.status.${call.status === "" ? "unknown" : call.status}`)}
                    </td>
                    <td className="px-4 py-2">{call.locale ?? "—"}</td>
                    <td className="px-4 py-2">
                      {urgency !== undefined ? (
                        <Badge tone={urgencyTone[urgency]}>
                          {t(`messages.urgencyLevel.${urgency}`)}
                        </Badge>
                      ) : (
                        "—"
                      )}
                    </td>
                    <td className="px-4 py-2 text-right tabular-nums">
                      {formatCallDuration(call.startedAt, call.endedAt)}
                    </td>
                    <td className="px-4 py-2 text-right">
                      {call.recorded ? <Badge tone="accent">{t("calls.recorded")}</Badge> : "—"}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </Card>
      )}
    </div>
  );
}
