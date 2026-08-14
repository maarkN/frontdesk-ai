/**
 * Structured messages (who / what / urgency / callback). "Handled" is a
 * device-local triage flag (see lib/handled.ts) until the API grows the
 * field.
 */
import type { Urgency } from "@frontdesk/shared";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { Badge, Button, Card, EmptyState, ErrorState, PageHeader, Spinner } from "../components/ui";
import { useAuth } from "../lib/auth";
import { formatDateTime, formatPhone } from "../lib/format";
import { useHandledMessages } from "../lib/handled";
import { messagesQuery } from "../lib/queries";
import { useUiLocale } from "../lib/useUiLocale";

const urgencyTone: Record<Urgency, "neutral" | "warn" | "danger"> = {
  low: "neutral",
  normal: "neutral",
  high: "warn",
  emergency: "danger",
};

export function MessagesPage() {
  const { t } = useTranslation();
  const { client } = useAuth();
  const locale = useUiLocale();
  const messages = useQuery(messagesQuery(client));
  const { handled, toggle } = useHandledMessages();
  const [showHandled, setShowHandled] = useState(false);

  if (messages.isPending) {
    return <Spinner />;
  }
  if (messages.isError) {
    return <ErrorState error={messages.error} onRetry={() => void messages.refetch()} />;
  }

  const visible = messages.data.filter((m) => showHandled || !handled.has(m.id));

  return (
    <div>
      <PageHeader
        title={t("messages.title")}
        actions={
          <label className="flex items-center gap-2 text-sm text-muted">
            <input
              type="checkbox"
              checked={showHandled}
              onChange={(e) => setShowHandled(e.target.checked)}
              className="accent-[var(--fd-accent)]"
            />
            {t("app:messagesPage.showHandled")}
          </label>
        }
      />
      <p className="mb-4 text-xs text-muted">{t("app:messagesPage.localNote")}</p>

      {visible.length === 0 ? (
        <EmptyState message={t("messages.empty")} />
      ) : (
        <div className="space-y-3">
          {visible.map((m) => {
            const isHandled = handled.has(m.id);
            return (
              <Card key={m.id} className={isHandled ? "opacity-60" : ""}>
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-semibold">{m.who}</span>
                      <Badge tone={urgencyTone[m.urgency]}>
                        {t(`messages.urgencyLevel.${m.urgency}`)}
                      </Badge>
                      <Badge tone={isHandled ? "ok" : "accent"}>
                        {t(isHandled ? "app:messagesPage.handled" : "app:messagesPage.new")}
                      </Badge>
                    </div>
                    <p className="mt-1 text-sm">{m.what}</p>
                    <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted">
                      <span>
                        {t("messages.callback")}:{" "}
                        <a className="text-accent hover:underline" href={`tel:${m.callbackPhone}`}>
                          {formatPhone(m.callbackPhone)}
                        </a>
                      </span>
                      <time>{formatDateTime(m.createdAt, locale)}</time>
                      {m.callId !== undefined ? (
                        <Link
                          to="/calls/$callId"
                          params={{ callId: m.callId }}
                          className="text-accent hover:underline"
                        >
                          {t("app:messagesPage.call")} →
                        </Link>
                      ) : null}
                    </div>
                  </div>
                  <Button variant="ghost" onClick={() => toggle(m.id)}>
                    {t(isHandled ? "app:messagesPage.markUnhandled" : "app:messagesPage.markHandled")}
                  </Button>
                </div>
              </Card>
            );
          })}
        </div>
      )}
    </div>
  );
}
