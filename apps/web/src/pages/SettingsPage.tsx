/**
 * Tenant settings. The /v1 surface only mutates the answering mode
 * (PUT /v1/tenants/me/mode) and numbers (POST /v1/dids); business hours,
 * coverage, owner mobile and default language are set at onboarding and
 * shown read-only here (see settingsPage.readOnlyNote).
 */
import {
  ApiError,
  addDidRequestSchema,
  dayKeySchema,
  type Mode,
  type Tenant,
  type UpdateModeRequest,
} from "@frontdesk/shared";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import {
  Badge,
  Button,
  Card,
  ErrorState,
  Field,
  PageHeader,
  Spinner,
  inputClass,
} from "../components/ui";
import { useAuth } from "../lib/auth";
import { formatPhone } from "../lib/format";
import { didsQuery, invalidateDids, invalidateTenant, tenantQuery } from "../lib/queries";
import { useUiLocale } from "../lib/useUiLocale";

function ModeCard({ tenant }: { tenant: Tenant }) {
  const { t } = useTranslation();
  const { client } = useAuth();
  const qc = useQueryClient();
  const [mode, setMode] = useState<Mode>(tenant.mode);
  const [overflowSeconds, setOverflowSeconds] = useState<number>(tenant.overflowSeconds ?? 20);
  const [savedFlash, setSavedFlash] = useState(false);

  const update = useMutation({
    mutationFn: (req: UpdateModeRequest) => client.updateMode(req),
    onSuccess: async () => {
      await invalidateTenant(qc);
      setSavedFlash(true);
      setTimeout(() => setSavedFlash(false), 2500);
    },
  });

  const dirty = mode !== tenant.mode || (mode === "overflow" && overflowSeconds !== tenant.overflowSeconds);

  return (
    <Card>
      <h2 className="mb-3 text-sm font-semibold">{t("mode.title")}</h2>
      <div className="space-y-3">
        <label className="flex cursor-pointer items-start gap-3">
          <input
            type="radio"
            name="mode"
            className="mt-1 accent-[var(--fd-accent)]"
            checked={mode === "overflow"}
            onChange={() => setMode("overflow")}
          />
          <span>
            <span className="block text-sm font-medium">{t("mode.overflow")}</span>
            <span className="block text-xs text-muted">
              {t("mode.overflowHint", { seconds: overflowSeconds })}
            </span>
          </span>
        </label>
        <label className="flex cursor-pointer items-start gap-3">
          <input
            type="radio"
            name="mode"
            className="mt-1 accent-[var(--fd-accent)]"
            checked={mode === "always_ai"}
            onChange={() => setMode("always_ai")}
          />
          <span>
            <span className="block text-sm font-medium">{t("mode.alwaysAi")}</span>
            <span className="block text-xs text-muted">{t("mode.alwaysAiHint")}</span>
          </span>
        </label>

        {mode === "always_ai" ? (
          <p className="rounded-md border border-warn/40 bg-warn/10 p-3 text-xs">
            {t("app:settingsPage.alwaysAiWarning")}
          </p>
        ) : (
          <Field label={t("mode.overflowSecondsLabel")}>
            <input
              className={`${inputClass} max-w-32`}
              type="number"
              min={5}
              max={120}
              value={overflowSeconds}
              onChange={(e) => setOverflowSeconds(Math.max(1, Number(e.target.value) || 1))}
            />
          </Field>
        )}

        {update.isError ? <ErrorState error={update.error} /> : null}
        <div className="flex items-center gap-3">
          <Button
            disabled={!dirty || update.isPending}
            onClick={() =>
              update.mutate(
                mode === "overflow" ? { mode, overflowSeconds } : { mode },
              )
            }
          >
            {t("common.save")}
          </Button>
          {savedFlash ? <span className="text-sm text-ok">{t("mode.updated")}</span> : null}
        </div>
      </div>
    </Card>
  );
}

function HoursCard({ tenant }: { tenant: Tenant }) {
  const { t } = useTranslation();
  const locale = useUiLocale();
  // dayKeySchema.options starts at "sun"; 2026-08-02 is a Sunday.
  const weekday = new Intl.DateTimeFormat(locale, { weekday: "short" });
  const dayLabel = (index: number) => weekday.format(new Date(Date.UTC(2026, 7, 2 + index, 12)));
  return (
    <Card>
      <h2 className="mb-1 text-sm font-semibold">{t("onboarding.hours")}</h2>
      <p className="mb-3 text-xs text-muted">{t("app:settingsPage.hoursHint")}</p>
      <table className="w-full text-sm">
        <tbody>
          {dayKeySchema.options.map((day, index) => {
            const ranges = tenant.hours?.[day] ?? [];
            return (
              <tr key={day} className="border-b border-edge last:border-0">
                <td className="py-1.5 pr-3 font-medium">{dayLabel(index)}</td>
                <td className="py-1.5 text-right tabular-nums">
                  {ranges.length === 0 ? (
                    <span className="text-muted">{t("app:settingsPage.closed")}</span>
                  ) : (
                    ranges.map((r) => `${r.open}–${r.close}`).join(", ")
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </Card>
  );
}

function DidsCard() {
  const { t } = useTranslation();
  const { client } = useAuth();
  const qc = useQueryClient();
  const dids = useQuery(didsQuery(client));
  const [number, setNumber] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const add = useMutation({
    mutationFn: (n: string) => client.addDid({ number: n }),
    onSuccess: async () => {
      setNumber("");
      await invalidateDids(qc);
    },
  });

  return (
    <Card>
      <h2 className="mb-3 text-sm font-semibold">{t("dids.title")}</h2>
      {dids.isPending ? (
        <Spinner />
      ) : dids.isError ? (
        <ErrorState error={dids.error} onRetry={() => void dids.refetch()} />
      ) : dids.data.length === 0 ? (
        <p className="mb-3 text-sm text-muted">{t("dids.empty")}</p>
      ) : (
        <ul className="mb-3 space-y-1.5">
          {dids.data.map((d) => (
            <li key={d.number} className="flex items-center justify-between text-sm">
              <span className="font-medium tabular-nums">{formatPhone(d.number)}</span>
              <span className="flex items-center gap-2 text-xs text-muted">
                {d.provider}
                <Badge tone={d.active ? "ok" : "neutral"}>
                  {t(d.active ? "dids.active" : "dids.inactive")}
                </Badge>
              </span>
            </li>
          ))}
        </ul>
      )}
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          setFormError(null);
          const parsed = addDidRequestSchema.shape.number.safeParse(number.trim());
          if (!parsed.success) {
            setFormError(parsed.error.issues[0]?.message ?? t("errors.generic"));
            return;
          }
          add.mutate(parsed.data);
        }}
      >
        <input
          className={inputClass}
          placeholder="+15145550100"
          value={number}
          onChange={(e) => setNumber(e.target.value)}
          aria-label={t("dids.numberLabel")}
        />
        <Button type="submit" disabled={add.isPending || number.trim() === ""}>
          {t("dids.add")}
        </Button>
      </form>
      {formError !== null ? <p className="mt-2 text-sm text-danger">{formError}</p> : null}
      {add.isError ? (
        <p className="mt-2 text-sm text-danger">
          {add.error instanceof ApiError && add.error.isConflict
            ? t("dids.conflict")
            : t("errors.generic")}
        </p>
      ) : null}
    </Card>
  );
}

export function SettingsPage() {
  const { t } = useTranslation();
  const { client } = useAuth();
  const tenant = useQuery(tenantQuery(client));

  if (tenant.isPending) {
    return <Spinner />;
  }
  if (tenant.isError) {
    return <ErrorState error={tenant.error} onRetry={() => void tenant.refetch()} />;
  }

  const data = tenant.data;

  return (
    <div>
      <PageHeader
        title={t("app:settingsPage.title")}
        actions={<Badge tone="accent">{t(`usage.planName.${data.plan}`)}</Badge>}
      />
      <div className="grid items-start gap-4 lg:grid-cols-2">
        <div className="space-y-4">
          <ModeCard tenant={data} />
          <DidsCard />
        </div>
        <div className="space-y-4">
          <HoursCard tenant={data} />
          <Card>
            <h2 className="mb-3 text-sm font-semibold">{t("app:settingsPage.coverageTitle")}</h2>
            <div className="flex flex-wrap gap-1.5">
              {(data.coverage?.postalPrefixes ?? []).length === 0 ? (
                <p className="text-sm text-muted">{t("onboarding.coverageHint")}</p>
              ) : (
                (data.coverage?.postalPrefixes ?? []).map((p) => <Badge key={p}>{p}</Badge>)
              )}
            </div>
          </Card>
          <Card>
            <h2 className="mb-2 text-sm font-semibold">{t("app:settingsPage.ownerTitle")}</h2>
            <p className="text-sm font-medium tabular-nums">{formatPhone(data.ownerMobile)}</p>
            <p className="mt-1 text-xs text-muted">{t("app:settingsPage.ownerHint")}</p>
          </Card>
          <Card>
            <h2 className="mb-2 text-sm font-semibold">{t("app:settingsPage.languageTitle")}</h2>
            <Badge>{data.defaultLocale}</Badge>
          </Card>
          <p className="text-xs text-muted">{t("app:settingsPage.readOnlyNote")}</p>
        </div>
      </div>
    </div>
  );
}
