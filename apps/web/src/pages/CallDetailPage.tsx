/**
 * Call detail: recording player, redacted transcript, per-turn latency
 * metrics and an event timeline. The API returns the folded CallSnapshot
 * (state = fold(events)), not the raw stream, so the timeline is derived
 * from snapshot fields: lifecycle timestamps, consent, locale history,
 * flow path and degradation stage.
 */
import type { Call, TurnLatency } from "@frontdesk/shared";
import type { TFunction } from "i18next";
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { AudioPlayer } from "../components/AudioPlayer";
import { TranscriptView } from "../components/TranscriptView";
import { Badge, Card, ErrorState, PageHeader, Spinner } from "../components/ui";
import { useAuth } from "../lib/auth";
import { formatCallDuration, formatDateTime, formatMs } from "../lib/format";
import { callQuery } from "../lib/queries";
import { useUiLocale } from "../lib/useUiLocale";

interface TimelineItem {
  key: string;
  label: string;
  detail?: string | undefined;
  tone: "neutral" | "ok" | "warn" | "danger";
}

function buildTimeline(call: Call, t: TFunction): TimelineItem[] {
  const snap = call.snapshot;
  const items: TimelineItem[] = [];

  if (snap.startedAt !== undefined) {
    items.push({ key: "started", label: t("app:callDetail.callStarted"), tone: "neutral" });
  }
  if (snap.consentCaptured) {
    items.push({
      key: "consent",
      label: t(snap.consentGranted ? "app:callDetail.consentGranted" : "app:callDetail.consentDenied"),
      tone: snap.consentGranted ? "ok" : "warn",
    });
  }
  if (snap.status !== "started" && snap.status !== "") {
    items.push({ key: "answered", label: t("app:callDetail.callAnswered"), tone: "ok" });
  }
  const history = snap.localeHistory ?? [];
  history.forEach((loc, i) => {
    items.push({
      key: `locale-${i}`,
      label: t(i === 0 ? "app:callDetail.langDetected" : "app:callDetail.langSwitched", {
        locale: loc,
      }),
      tone: "neutral",
    });
  });
  for (const node of snap.path ?? []) {
    items.push({
      key: `node-${items.length}`,
      label: t("app:callDetail.nodeEntered", { node }),
      tone: "neutral",
    });
  }
  if (snap.budget.stage !== "" && snap.budget.stage !== "normal") {
    items.push({
      key: "degradation",
      label: t("app:callDetail.degradation", { stage: snap.budget.stage }),
      tone: "warn",
    });
  }
  if (snap.budget.errorCount > 0) {
    items.push({
      key: "errors",
      label: t("app:callDetail.errors", { count: snap.budget.errorCount }),
      tone: "danger",
    });
  }
  if (snap.endedAt !== undefined) {
    items.push({
      key: "ended",
      label: t("app:callDetail.callEnded"),
      detail: snap.endReason !== undefined ? t(`calls.endReason.${snap.endReason}`) : undefined,
      tone: "neutral",
    });
  }
  return items;
}

function LatencyRow({ turn, index }: { turn: TurnLatency; index: number }) {
  const { t } = useTranslation();
  const cell = (v: number | undefined) => (v !== undefined ? formatMs(v) : "—");
  const perceived = turn.perceivedMs;
  const over = perceived !== undefined && perceived > 1200;
  return (
    <tr className="border-b border-edge last:border-0">
      <td className="px-3 py-1.5 text-muted">{t("app:callDetail.turn", { n: index + 1 })}</td>
      <td className="px-3 py-1.5 text-right tabular-nums">{cell(turn.vadEndToSttFinalMs)}</td>
      <td className="px-3 py-1.5 text-right tabular-nums">{cell(turn.sttFinalToLlmFirstTokenMs)}</td>
      <td className="px-3 py-1.5 text-right tabular-nums">{cell(turn.llmFirstTokenToTtsFirstChunkMs)}</td>
      <td className={`px-3 py-1.5 text-right font-medium tabular-nums ${over ? "text-warn" : "text-ok"}`}>
        {cell(perceived)}
      </td>
    </tr>
  );
}

export function CallDetailPage() {
  const { t } = useTranslation();
  const { client } = useAuth();
  const locale = useUiLocale();
  const { callId } = useParams({ from: "/authed/calls/$callId" });

  const call = useQuery(callQuery(client, callId));

  if (call.isPending) {
    return <Spinner />;
  }
  if (call.isError) {
    return <ErrorState error={call.error} onRetry={() => void call.refetch()} />;
  }

  const { snapshot, recordingUrl } = call.data;
  const timeline = buildTimeline(call.data, t);
  const dotTone = {
    neutral: "bg-muted",
    ok: "bg-ok",
    warn: "bg-warn",
    danger: "bg-danger",
  } as const;

  return (
    <div>
      <Link to="/calls" className="text-sm text-accent hover:underline">
        ← {t("calls.title")}
      </Link>
      <PageHeader
        title={
          snapshot.startedAt !== undefined
            ? `${t("calls.detail")} — ${formatDateTime(snapshot.startedAt, locale)}`
            : t("calls.detail")
        }
        actions={
          <>
            {snapshot.locale !== undefined ? <Badge>{snapshot.locale}</Badge> : null}
            <Badge tone={snapshot.status === "ended" ? "neutral" : "accent"}>
              {t(`calls.status.${snapshot.status === "" ? "unknown" : snapshot.status}`)}
            </Badge>
            <Badge>{`${t("calls.duration")}: ${formatCallDuration(snapshot.startedAt, snapshot.endedAt)}`}</Badge>
          </>
        }
      />

      <div className="grid gap-4 lg:grid-cols-[2fr_1fr]">
        <div className="space-y-4">
          <Card>
            <h2 className="mb-3 text-sm font-semibold">{t("app:callDetail.recording")}</h2>
            {recordingUrl !== undefined ? (
              <AudioPlayer src={recordingUrl} />
            ) : (
              <p className="text-sm text-muted">{t("app:callDetail.noRecording")}</p>
            )}
          </Card>

          <Card>
            <h2 className="mb-1 text-sm font-semibold">{t("transcript.title")}</h2>
            <p className="mb-3 text-xs text-muted">{t("app:callDetail.piiNote")}</p>
            <TranscriptView entries={snapshot.transcript ?? []} locale={locale} />
          </Card>

          <Card>
            <h2 className="mb-3 text-sm font-semibold">{t("app:callDetail.metrics")}</h2>
            {(snapshot.turnLatencies ?? []).length === 0 ? (
              <p className="text-sm text-muted">{t("common.empty")}</p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-edge text-right text-xs text-muted uppercase">
                      <th className="px-3 py-1.5 text-left font-medium">{t("latency.title")}</th>
                      <th className="px-3 py-1.5 font-medium">{t("latency.stt")}</th>
                      <th className="px-3 py-1.5 font-medium">{t("latency.llmFirstToken")}</th>
                      <th className="px-3 py-1.5 font-medium">{t("latency.ttsFirstChunk")}</th>
                      <th className="px-3 py-1.5 font-medium">{t("latency.perceived")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(snapshot.turnLatencies ?? []).map((turn, i) => (
                      <LatencyRow key={i} turn={turn} index={i} />
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Card>
        </div>

        <Card className="self-start">
          <h2 className="mb-3 text-sm font-semibold">{t("app:callDetail.timeline")}</h2>
          {timeline.length === 0 ? (
            <p className="text-sm text-muted">{t("common.empty")}</p>
          ) : (
            <ol className="space-y-0">
              {timeline.map((item, i) => (
                <li key={item.key} className="relative flex gap-3 pb-4 last:pb-0">
                  {i < timeline.length - 1 ? (
                    <span className="absolute top-3 left-[5px] h-full w-px bg-edge" />
                  ) : null}
                  <span className={`relative mt-1.5 size-[11px] shrink-0 rounded-full ${dotTone[item.tone]}`} />
                  <div className="min-w-0 text-sm">
                    <p>{item.label}</p>
                    {item.detail !== undefined ? (
                      <p className="text-xs text-muted">{item.detail}</p>
                    ) : null}
                  </div>
                </li>
              ))}
            </ol>
          )}
        </Card>
      </div>
    </div>
  );
}
