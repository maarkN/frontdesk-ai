/**
 * KPI dashboard. Sources:
 *  - calls today / resolution rate: GET /v1/calls (client-side fold — the
 *    list endpoint has no query params yet);
 *  - p50 turn latency: perceivedMs over the turn latencies of up to
 *    LATENCY_SAMPLE recent calls (GET /v1/calls/{id}, cached individually);
 *  - upcoming jobs + estimated value: GET /v1/appointments; the CAD value is
 *    count x AVG_JOB_VALUE_CAD (documented heuristic — no price in domain).
 */
import { useQueries, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { Badge, Card, EmptyState, ErrorState, PageHeader, Spinner } from "../components/ui";
import { useAuth } from "../lib/auth";
import {
  formatCad,
  formatCallDuration,
  formatMs,
  formatPercent,
  formatTime,
  p50,
} from "../lib/format";
import { appointmentsQuery, callQuery, callsQuery } from "../lib/queries";
import { useUiLocale } from "../lib/useUiLocale";

const LATENCY_SAMPLE = 10;
const AVG_JOB_VALUE_CAD_CENTS = 25_000;

function KpiCard({
  label,
  value,
  hint,
  accent,
}: {
  label: string;
  value: string;
  hint?: string | undefined;
  accent?: string | undefined;
}) {
  return (
    <Card>
      <p className="text-xs font-medium tracking-wide text-muted uppercase">{label}</p>
      <p className="mt-1 text-2xl font-semibold tabular-nums">{value}</p>
      {accent !== undefined ? <p className="mt-0.5 text-sm text-accent">{accent}</p> : null}
      {hint !== undefined ? <p className="mt-1 text-xs text-muted">{hint}</p> : null}
    </Card>
  );
}

export function DashboardPage() {
  const { t } = useTranslation();
  const { client } = useAuth();
  const locale = useUiLocale();

  const calls = useQuery(callsQuery(client));
  const appointments = useQuery(appointmentsQuery(client));

  const recentIds = (calls.data ?? []).slice(0, LATENCY_SAMPLE).map((c) => c.callId);
  const details = useQueries({
    queries: recentIds.map((id) => callQuery(client, id)),
  });

  if (calls.isPending) {
    return <Spinner />;
  }
  if (calls.isError) {
    return <ErrorState error={calls.error} onRetry={() => void calls.refetch()} />;
  }

  const all = calls.data;
  const todayStart = new Date();
  todayStart.setHours(0, 0, 0, 0);
  const today = all.filter(
    (c) => c.startedAt !== undefined && Date.parse(c.startedAt) >= todayStart.getTime(),
  );

  const monthAgo = Date.now() - 30 * 24 * 3600 * 1000;
  const ended = all.filter(
    (c) =>
      c.status === "ended" &&
      c.endReason !== undefined &&
      (c.startedAt === undefined || Date.parse(c.startedAt) >= monthAgo),
  );
  const resolved = ended.filter(
    (c) => c.endReason === "completed" || c.endReason === "voicemail",
  );
  const resolutionRate = ended.length > 0 ? resolved.length / ended.length : null;

  const perceived = details
    .flatMap((d) => d.data?.snapshot.turnLatencies ?? [])
    .map((l) => l.perceivedMs)
    .filter((v): v is number => v !== undefined);
  const p50Ms = p50(perceived);

  const now = Date.now();
  const upcoming = (appointments.data ?? []).filter((a) => Date.parse(a.startsAt) >= now);
  const estValue = formatCad(upcoming.length * AVG_JOB_VALUE_CAD_CENTS, locale);

  return (
    <div>
      <PageHeader title={t("app:dashboard.title")} />
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <KpiCard label={t("app:dashboard.callsToday")} value={today.length.toString()} />
        <KpiCard
          label={t("app:dashboard.resolutionRate")}
          value={resolutionRate !== null ? formatPercent(resolutionRate, locale) : t("common.notAvailable")}
          hint={t("app:dashboard.resolutionHint")}
        />
        <KpiCard
          label={t("app:dashboard.p50Latency")}
          value={p50Ms !== null ? formatMs(p50Ms) : t("common.notAvailable")}
          hint={
            p50Ms !== null
              ? t("app:dashboard.latencyHint", { count: recentIds.length })
              : undefined
          }
          accent={t("app:dashboard.latencyTarget")}
        />
        <KpiCard
          label={t("app:dashboard.scheduledJobs")}
          value={upcoming.length.toString()}
          accent={upcoming.length > 0 ? t("app:dashboard.estValue", { amount: estValue }) : undefined}
          hint={t("app:dashboard.estValueHint", {
            amount: formatCad(AVG_JOB_VALUE_CAD_CENTS, locale),
          })}
        />
      </div>

      <div className="mt-6">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold">{t("app:dashboard.recentCalls")}</h2>
          <Link to="/calls" className="text-sm text-accent hover:underline">
            {t("app:dashboard.viewAll")}
          </Link>
        </div>
        {today.length === 0 ? (
          <EmptyState message={t("app:dashboard.noCallsToday")} />
        ) : (
          <Card className="divide-y divide-edge p-0">
            {today.slice(0, 8).map((call) => (
              <Link
                key={call.callId}
                to="/calls/$callId"
                params={{ callId: call.callId }}
                className="flex items-center justify-between gap-3 px-4 py-2.5 text-sm hover:bg-surface-2"
              >
                <span className="tabular-nums text-muted">
                  {call.startedAt !== undefined ? formatTime(call.startedAt, locale) : "—"}
                </span>
                <span className="flex-1 truncate">
                  {call.endReason !== undefined
                    ? t(`calls.endReason.${call.endReason}`)
                    : t(`calls.status.${call.status === "" ? "unknown" : call.status}`)}
                </span>
                {call.locale !== undefined ? <Badge>{call.locale}</Badge> : null}
                <span className="w-12 text-right tabular-nums text-muted">
                  {formatCallDuration(call.startedAt, call.endedAt)}
                </span>
              </Link>
            ))}
          </Card>
        )}
      </div>
    </div>
  );
}
