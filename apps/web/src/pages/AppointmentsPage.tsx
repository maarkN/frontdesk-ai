/** Appointments: upcoming/past list + manual booking form (POST /v1/appointments). */
import { createAppointmentRequestSchema, type CreateAppointmentRequest } from "@frontdesk/shared";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import {
  Badge,
  Button,
  Card,
  EmptyState,
  ErrorState,
  Field,
  PageHeader,
  Spinner,
  inputClass,
} from "../components/ui";
import { useAuth } from "../lib/auth";
import { formatDateTime, formatPhone } from "../lib/format";
import { appointmentsQuery, invalidateAppointments } from "../lib/queries";
import { useUiLocale } from "../lib/useUiLocale";

function toIso(local: string): string {
  return new Date(local).toISOString();
}

function NewAppointmentForm({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation();
  const { client } = useAuth();
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [phone, setPhone] = useState("");
  const [service, setService] = useState("");
  const [postalCode, setPostalCode] = useState("");
  const [startsAt, setStartsAt] = useState("");
  const [endsAt, setEndsAt] = useState("");
  const [notes, setNotes] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: (req: CreateAppointmentRequest) => client.createAppointment(req),
    onSuccess: async () => {
      await invalidateAppointments(qc);
      onDone();
    },
  });

  const submit = () => {
    setFormError(null);
    if (startsAt === "" || endsAt === "") {
      setFormError(t("errors.generic"));
      return;
    }
    const candidate: CreateAppointmentRequest = {
      customerName: name.trim(),
      service: service.trim(),
      startsAt: toIso(startsAt),
      endsAt: toIso(endsAt),
      ...(phone.trim() !== "" ? { customerPhone: phone.trim() } : {}),
      ...(postalCode.trim() !== "" ? { postalCode: postalCode.trim().toUpperCase() } : {}),
      ...(notes.trim() !== "" ? { notes: notes.trim() } : {}),
    };
    const parsed = createAppointmentRequestSchema.safeParse(candidate);
    if (!parsed.success) {
      setFormError(parsed.error.issues[0]?.message ?? t("errors.generic"));
      return;
    }
    create.mutate(parsed.data);
  };

  return (
    <Card className="mb-4">
      <form
        className="grid gap-3 sm:grid-cols-2"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <Field label={t("appointments.customerName")}>
          <input className={inputClass} value={name} onChange={(e) => setName(e.target.value)} required />
        </Field>
        <Field label={`${t("appointments.customerPhone")} (${t("common.optional")})`}>
          <input className={inputClass} value={phone} onChange={(e) => setPhone(e.target.value)} />
        </Field>
        <Field label={t("appointments.service")}>
          <input className={inputClass} value={service} onChange={(e) => setService(e.target.value)} required />
        </Field>
        <Field label={`${t("appointments.postalCode")} (${t("common.optional")})`}>
          <input className={inputClass} value={postalCode} onChange={(e) => setPostalCode(e.target.value)} />
        </Field>
        <Field label={t("appointments.startsAt")}>
          <input
            className={inputClass}
            type="datetime-local"
            value={startsAt}
            onChange={(e) => setStartsAt(e.target.value)}
            required
          />
        </Field>
        <Field label={t("appointments.endsAt")}>
          <input
            className={inputClass}
            type="datetime-local"
            value={endsAt}
            onChange={(e) => setEndsAt(e.target.value)}
            required
          />
        </Field>
        <div className="sm:col-span-2">
          <Field label={`${t("appointments.notes")} (${t("common.optional")})`}>
            <textarea
              className={`${inputClass} min-h-16`}
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
            />
          </Field>
        </div>
        {formError !== null ? (
          <p className="text-sm text-danger sm:col-span-2">{formError}</p>
        ) : null}
        {create.isError ? (
          <div className="sm:col-span-2">
            <ErrorState error={create.error} />
          </div>
        ) : null}
        <div className="flex gap-2 sm:col-span-2">
          <Button type="submit" disabled={create.isPending}>
            {t("common.save")}
          </Button>
          <Button variant="ghost" onClick={onDone}>
            {t("common.cancel")}
          </Button>
        </div>
      </form>
    </Card>
  );
}

export function AppointmentsPage() {
  const { t } = useTranslation();
  const { client } = useAuth();
  const locale = useUiLocale();
  const [creating, setCreating] = useState(false);
  const appointments = useQuery(appointmentsQuery(client));

  if (appointments.isPending) {
    return <Spinner />;
  }
  if (appointments.isError) {
    return <ErrorState error={appointments.error} onRetry={() => void appointments.refetch()} />;
  }

  const now = Date.now();
  const sorted = [...appointments.data].sort(
    (a, b) => Date.parse(a.startsAt) - Date.parse(b.startsAt),
  );

  return (
    <div>
      <PageHeader
        title={t("appointments.title")}
        actions={
          creating ? null : (
            <Button onClick={() => setCreating(true)}>{t("appointments.add")}</Button>
          )
        }
      />
      {creating ? <NewAppointmentForm onDone={() => setCreating(false)} /> : null}

      {sorted.length === 0 ? (
        <EmptyState message={t("appointments.empty")} />
      ) : (
        <Card className="overflow-x-auto p-0">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-edge text-left text-xs text-muted uppercase">
                <th className="px-4 py-2 font-medium">{t("appointments.startsAt")}</th>
                <th className="px-4 py-2 font-medium">{t("appointments.customerName")}</th>
                <th className="px-4 py-2 font-medium">{t("appointments.service")}</th>
                <th className="px-4 py-2 font-medium">{t("appointments.postalCode")}</th>
                <th className="px-4 py-2 font-medium">{t("appointments.customerPhone")}</th>
                <th className="px-4 py-2 font-medium" />
              </tr>
            </thead>
            <tbody>
              {sorted.map((a) => {
                const past = Date.parse(a.endsAt) < now;
                return (
                  <tr
                    key={a.id}
                    className={`border-b border-edge last:border-0 hover:bg-surface-2 ${past ? "opacity-55" : ""}`}
                  >
                    <td className="px-4 py-2 whitespace-nowrap tabular-nums">
                      {formatDateTime(a.startsAt, locale)}
                    </td>
                    <td className="px-4 py-2 font-medium">{a.customerName}</td>
                    <td className="px-4 py-2">{a.service}</td>
                    <td className="px-4 py-2">{a.postalCode ?? "—"}</td>
                    <td className="px-4 py-2">
                      {a.customerPhone !== undefined ? formatPhone(a.customerPhone) : "—"}
                    </td>
                    <td className="px-4 py-2 text-right">
                      {a.callId !== undefined ? (
                        <Link
                          to="/calls/$callId"
                          params={{ callId: a.callId }}
                          className="text-xs text-accent hover:underline"
                        >
                          {t("appointments.fromCall")}
                        </Link>
                      ) : (
                        <Badge>{t("appointments.manual")}</Badge>
                      )}
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
