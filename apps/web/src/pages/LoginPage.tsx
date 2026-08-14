/** API-key sign-in: validates the key against GET /v1/tenants/me. */
import { ApiError } from "@frontdesk/shared";
import { Link, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { useAuth } from "../lib/auth";
import { Button, Field, inputClass } from "../components/ui";

export function LoginPage() {
  const { t } = useTranslation();
  const { client, signIn } = useAuth();
  const navigate = useNavigate();
  const [key, setKey] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    const trimmed = key.trim();
    if (trimmed === "" || busy) {
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await client.withApiKey(trimmed).getTenant();
      signIn(trimmed);
      await navigate({ to: "/" });
    } catch (err) {
      if (err instanceof ApiError && err.isUnauthorized) {
        setError(t("auth.invalidKey"));
      } else if (err instanceof TypeError) {
        setError(t("errors.network"));
      } else {
        setError(t("errors.generic"));
      }
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex min-h-dvh items-center justify-center p-6">
      <div className="w-full max-w-sm rounded-lg border border-edge bg-surface p-6">
        <div className="mb-5 flex items-center gap-2">
          <span className="flex size-8 items-center justify-center rounded-md bg-accent font-bold text-accent-ink">
            F
          </span>
          <h1 className="text-lg font-semibold">{t("app:login.title")}</h1>
        </div>
        <p className="mb-4 text-sm text-muted">{t("app:login.subtitle")}</p>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
          className="space-y-4"
        >
          <Field label={t("auth.apiKeyLabel")}>
            <input
              className={inputClass}
              type="password"
              autoComplete="off"
              placeholder={t("auth.apiKeyPlaceholder")}
              value={key}
              onChange={(e) => setKey(e.target.value)}
            />
          </Field>
          {error !== null ? <p className="text-sm text-danger">{error}</p> : null}
          <Button type="submit" disabled={busy || key.trim() === ""}>
            {t("auth.signIn")}
          </Button>
        </form>
        <p className="mt-5 border-t border-edge pt-4 text-sm text-muted">
          {t("app:login.noAccount")}{" "}
          <Link to="/onboarding" className="font-medium text-accent hover:underline">
            {t("app:login.startOnboarding")}
          </Link>
        </p>
      </div>
    </div>
  );
}
