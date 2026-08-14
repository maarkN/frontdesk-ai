/** Authenticated shell: left sidebar nav, language switcher, sign out. */
import { localeSchema, type Locale } from "@frontdesk/shared";
import { Link, Outlet, useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { setUiLocale } from "../i18n";
import { useAuth } from "../lib/auth";
import { Select } from "./ui";

const navItems = [
  { to: "/", key: "nav.dashboard" },
  { to: "/calls", key: "nav.calls" },
  { to: "/messages", key: "nav.messages" },
  { to: "/appointments", key: "nav.appointments" },
  { to: "/settings", key: "nav.settings" },
] as const;

export function Layout() {
  const { t, i18n } = useTranslation();
  const { signOut } = useAuth();
  const navigate = useNavigate();

  const uiLocale: Locale = localeSchema.safeParse(i18n.language).success
    ? (i18n.language as Locale)
    : "en-CA";

  return (
    <div className="flex min-h-dvh">
      <aside className="flex w-52 shrink-0 flex-col border-r border-edge bg-surface p-4">
        <div className="mb-6 flex items-center gap-2">
          <span className="flex size-7 items-center justify-center rounded-md bg-accent text-sm font-bold text-accent-ink">
            F
          </span>
          <span className="text-sm font-semibold">{t("common.appName")}</span>
        </div>
        <nav className="flex flex-1 flex-col gap-1">
          {navItems.map((item) => (
            <Link
              key={item.to}
              to={item.to}
              className="rounded-md px-3 py-1.5 text-sm hover:bg-surface-2"
              activeProps={{ className: "bg-surface-2 font-medium text-ink" }}
              inactiveProps={{ className: "text-muted hover:text-ink" }}
              activeOptions={{ exact: item.to === "/" }}
            >
              {t(item.key)}
            </Link>
          ))}
        </nav>
        <div className="flex flex-col gap-2 border-t border-edge pt-3">
          <Select
            ariaLabel={t("app:language.label")}
            value={uiLocale}
            onChange={(v) => {
              const parsed = localeSchema.safeParse(v);
              if (parsed.success) {
                setUiLocale(parsed.data);
              }
            }}
          >
            <option value="en-CA">{t("app:language.en")}</option>
            <option value="fr-CA">{t("app:language.fr")}</option>
          </Select>
          <button
            type="button"
            className="rounded-md px-3 py-1.5 text-left text-sm text-muted hover:bg-surface-2 hover:text-ink"
            onClick={() => {
              signOut();
              void navigate({ to: "/login" });
            }}
          >
            {t("nav.signOut")}
          </button>
        </div>
      </aside>
      <main className="min-w-0 flex-1 p-6">
        <Outlet />
      </main>
    </div>
  );
}
