/**
 * Self-service onboarding (RF4, target <= 10 min) in four steps:
 *  1. business facts (name, plan, owner mobile, default language, coverage);
 *  2. website import — POST /v1/tenants carries websiteUrl and the server's
 *     LLM extraction comes back as CreateTenantResponse.profile for review;
 *     the one-time API key is shown here and stored via signIn;
 *  3. phone number (POST /v1/dids with the fresh key);
 *  4. test call instructions.
 */
import {
  addDidRequestSchema,
  createTenantRequestSchema,
  localeSchema,
  planSchema,
  type CreateTenantRequest,
  type CreateTenantResponse,
  type DID,
  type Locale,
  type Plan,
} from "@frontdesk/shared";
import { useMutation } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { Badge, Button, Card, ErrorState, Field, inputClass } from "../components/ui";
import { useAuth } from "../lib/auth";
import { formatPhone } from "../lib/format";

const STEPS = ["stepBusiness", "stepImport", "stepNumber", "stepTest"] as const;

export function OnboardingPage() {
  const { t } = useTranslation();
  const { client, signIn } = useAuth();
  const navigate = useNavigate();

  const [step, setStep] = useState(0);

  // Step 1 state.
  const [name, setName] = useState("");
  const [plan, setPlan] = useState<Plan>("pro");
  const [ownerMobile, setOwnerMobile] = useState("");
  const [defaultLocale, setDefaultLocale] = useState<Locale>("en-CA");
  const [prefixes, setPrefixes] = useState("");

  // Step 2 state.
  const [websiteUrl, setWebsiteUrl] = useState("");
  const [created, setCreated] = useState<CreateTenantResponse | null>(null);
  const [formError, setFormError] = useState<string | null>(null);

  // Step 3 state.
  const [number, setNumber] = useState("");
  const [did, setDid] = useState<DID | null>(null);

  const createTenant = useMutation({
    mutationFn: (req: CreateTenantRequest) => client.createTenant(req),
    onSuccess: (res) => {
      setCreated(res);
      signIn(res.apiKey);
    },
  });

  const addDid = useMutation({
    mutationFn: (n: string) => {
      if (created === null) {
        return Promise.reject(new Error("tenant not created yet"));
      }
      return client.withApiKey(created.apiKey).addDid({ number: n });
    },
    onSuccess: (d) => setDid(d),
  });

  const step1Valid = name.trim() !== "" && ownerMobile.trim() !== "";

  const submitCreate = (skipWebsite: boolean) => {
    setFormError(null);
    const candidate: CreateTenantRequest = {
      name: name.trim(),
      plan,
      ownerMobile: ownerMobile.trim(),
      defaultLocale,
      ...(skipWebsite || websiteUrl.trim() === "" ? {} : { websiteUrl: websiteUrl.trim() }),
      ...(prefixes.trim() !== ""
        ? {
            postalPrefixes: prefixes
              .split(",")
              .map((p) => p.trim().toUpperCase())
              .filter((p) => p !== ""),
          }
        : {}),
    };
    const parsed = createTenantRequestSchema.safeParse(candidate);
    if (!parsed.success) {
      setFormError(parsed.error.issues[0]?.message ?? t("errors.generic"));
      return;
    }
    createTenant.mutate(parsed.data);
  };

  return (
    <div className="mx-auto min-h-dvh max-w-2xl p-6">
      <div className="mb-6">
        <h1 className="text-xl font-semibold">{t("onboarding.title")}</h1>
        <p className="text-sm text-muted">{t("onboarding.subtitle")}</p>
      </div>

      {/* Stepper */}
      <ol className="mb-6 flex items-center gap-2">
        {STEPS.map((key, i) => (
          <li key={key} className="flex flex-1 items-center gap-2">
            <span
              className={`flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold ${
                i < step
                  ? "bg-ok text-accent-ink"
                  : i === step
                    ? "bg-accent text-accent-ink"
                    : "bg-surface-2 text-muted"
              }`}
            >
              {i + 1}
            </span>
            <span className={`hidden text-xs sm:block ${i === step ? "font-medium" : "text-muted"}`}>
              {t(`app:onboarding.${key}`)}
            </span>
            {i < STEPS.length - 1 ? <span className="h-px flex-1 bg-edge" /> : null}
          </li>
        ))}
      </ol>
      <p className="mb-4 text-xs text-muted">
        {t("app:onboarding.stepOf", { current: step + 1, total: STEPS.length })}
      </p>

      {step === 0 ? (
        <Card>
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (step1Valid) {
                setStep(1);
              }
            }}
          >
            <Field label={t("onboarding.businessName")}>
              <input className={inputClass} value={name} onChange={(e) => setName(e.target.value)} required />
            </Field>
            <Field label={t("onboarding.plan")}>
              <div className="flex gap-2">
                {planSchema.options.map((p) => (
                  <button
                    key={p}
                    type="button"
                    onClick={() => setPlan(p)}
                    className={`flex-1 rounded-md border px-3 py-2 text-sm font-medium ${
                      plan === p
                        ? "border-accent bg-accent/10 text-accent"
                        : "border-edge bg-surface hover:bg-surface-2"
                    }`}
                  >
                    {t(`usage.planName.${p}`)}
                  </button>
                ))}
              </div>
            </Field>
            <Field label={t("onboarding.ownerMobile")} hint={t("onboarding.ownerMobileHint")}>
              <input
                className={inputClass}
                type="tel"
                placeholder="+15145550100"
                value={ownerMobile}
                onChange={(e) => setOwnerMobile(e.target.value)}
                required
              />
            </Field>
            <Field label={t("onboarding.defaultLocale")}>
              <div className="flex gap-2">
                {localeSchema.options.map((l) => (
                  <button
                    key={l}
                    type="button"
                    onClick={() => setDefaultLocale(l)}
                    className={`flex-1 rounded-md border px-3 py-2 text-sm font-medium ${
                      defaultLocale === l
                        ? "border-accent bg-accent/10 text-accent"
                        : "border-edge bg-surface hover:bg-surface-2"
                    }`}
                  >
                    {l === "en-CA" ? t("app:language.en") : t("app:language.fr")}
                  </button>
                ))}
              </div>
            </Field>
            <Field label={t("onboarding.coverage")} hint={t("onboarding.coverageHint")}>
              <input
                className={inputClass}
                placeholder="H2X, H3B"
                value={prefixes}
                onChange={(e) => setPrefixes(e.target.value)}
              />
            </Field>
            <div className="flex justify-end">
              <Button type="submit" disabled={!step1Valid}>
                {t("common.next")}
              </Button>
            </div>
          </form>
        </Card>
      ) : null}

      {step === 1 ? (
        <Card>
          {created === null ? (
            <div className="space-y-4">
              <Field label={t("onboarding.website")} hint={t("onboarding.websiteHint")}>
                <input
                  className={inputClass}
                  type="url"
                  placeholder="https://…"
                  value={websiteUrl}
                  onChange={(e) => setWebsiteUrl(e.target.value)}
                />
              </Field>
              {formError !== null ? <p className="text-sm text-danger">{formError}</p> : null}
              {createTenant.isError ? <ErrorState error={createTenant.error} /> : null}
              {createTenant.isPending ? (
                <p className="text-sm text-muted">{t("app:onboarding.importing")}</p>
              ) : null}
              <div className="flex justify-between">
                <Button variant="ghost" onClick={() => setStep(0)}>
                  {t("common.back")}
                </Button>
                <div className="flex gap-2">
                  <Button variant="ghost" disabled={createTenant.isPending} onClick={() => submitCreate(true)}>
                    {t("app:onboarding.skipImport")}
                  </Button>
                  <Button
                    disabled={createTenant.isPending || websiteUrl.trim() === ""}
                    onClick={() => submitCreate(false)}
                  >
                    {t("onboarding.create")}
                  </Button>
                </div>
              </div>
            </div>
          ) : (
            <div className="space-y-4">
              <div>
                <h2 className="text-sm font-semibold">{t("onboarding.apiKeyTitle")}</h2>
                <p className="mt-1 text-xs text-warn">{t("auth.keyShownOnce")}</p>
                <code className="mt-2 block rounded-md bg-surface-2 p-3 text-sm break-all select-all">
                  {created.apiKey}
                </code>
                <p className="mt-1 text-xs text-muted">{t("app:onboarding.keyWarning")}</p>
              </div>
              {created.profile !== undefined ? (
                <div>
                  <h2 className="text-sm font-semibold">{t("onboarding.reviewProfile")}</h2>
                  {(created.profile.services ?? []).length > 0 ? (
                    <div className="mt-2">
                      <p className="text-xs font-medium text-muted uppercase">{t("onboarding.services")}</p>
                      <div className="mt-1 flex flex-wrap gap-1.5">
                        {(created.profile.services ?? []).map((s) => (
                          <Badge key={s}>{s}</Badge>
                        ))}
                      </div>
                    </div>
                  ) : null}
                  {(created.profile.faqs ?? []).length > 0 ? (
                    <div className="mt-3">
                      <p className="text-xs font-medium text-muted uppercase">{t("onboarding.faq")}</p>
                      <ul className="mt-1 space-y-2">
                        {(created.profile.faqs ?? []).map((f) => (
                          <li key={f.question} className="rounded-md bg-surface-2 p-2 text-sm">
                            <p className="font-medium">{f.question}</p>
                            <p className="text-muted">{f.answer}</p>
                          </li>
                        ))}
                      </ul>
                    </div>
                  ) : null}
                  {(created.profile.services ?? []).length === 0 &&
                  (created.profile.faqs ?? []).length === 0 ? (
                    <p className="mt-2 text-sm text-muted">{t("app:onboarding.nothingExtracted")}</p>
                  ) : null}
                </div>
              ) : null}
              <div className="flex justify-end">
                <Button onClick={() => setStep(2)}>{t("common.next")}</Button>
              </div>
            </div>
          )}
        </Card>
      ) : null}

      {step === 2 ? (
        <Card>
          <div className="space-y-4">
            <Field label={t("dids.numberLabel")} hint={t("app:onboarding.didHint")}>
              <input
                className={inputClass}
                placeholder="+15145550100"
                value={number}
                onChange={(e) => setNumber(e.target.value)}
                disabled={did !== null}
              />
            </Field>
            {did !== null ? (
              <p className="flex items-center gap-2 text-sm text-ok">
                <Badge tone="ok">{t("dids.active")}</Badge>
                {formatPhone(did.number)}
              </p>
            ) : null}
            {addDid.isError ? <ErrorState error={addDid.error} /> : null}
            <div className="flex justify-between">
              <Button variant="ghost" onClick={() => setStep(1)}>
                {t("common.back")}
              </Button>
              <div className="flex gap-2">
                {did === null ? (
                  <Button
                    disabled={addDid.isPending || !addDidRequestSchema.shape.number.safeParse(number.trim()).success}
                    onClick={() => addDid.mutate(number.trim())}
                  >
                    {t("dids.add")}
                  </Button>
                ) : null}
                <Button variant={did === null ? "ghost" : "primary"} onClick={() => setStep(3)}>
                  {t("common.next")}
                </Button>
              </div>
            </div>
          </div>
        </Card>
      ) : null}

      {step === 3 ? (
        <Card>
          <div className="space-y-4">
            <h2 className="text-sm font-semibold">{t("app:onboarding.testTitle")}</h2>
            {did !== null ? (
              <p className="text-sm leading-relaxed">
                {t("app:onboarding.testBody", {
                  number: formatPhone(did.number),
                  locale: defaultLocale === "en-CA" ? t("app:language.en") : t("app:language.fr"),
                })}
              </p>
            ) : (
              <p className="text-sm text-muted">{t("app:onboarding.testNoNumber")}</p>
            )}
            <div className="flex justify-between">
              <Button variant="ghost" onClick={() => setStep(2)}>
                {t("common.back")}
              </Button>
              <Button onClick={() => void navigate({ to: "/" })}>{t("onboarding.finish")}</Button>
            </div>
          </div>
        </Card>
      ) : null}

      <p className="mt-6 text-center text-sm text-muted">
        <Link to="/login" className="text-accent hover:underline">
          {t("auth.signIn")}
        </Link>
      </p>
    </div>
  );
}
