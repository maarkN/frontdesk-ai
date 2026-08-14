/** Small Tailwind-only UI primitives shared by every page. */
import { ApiError } from "@frontdesk/shared";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

export function PageHeader({ title, actions }: { title: string; actions?: ReactNode }) {
  return (
    <div className="mb-5 flex items-center justify-between gap-3">
      <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
      {actions !== undefined ? <div className="flex items-center gap-2">{actions}</div> : null}
    </div>
  );
}

export function Card({ children, className = "" }: { children: ReactNode; className?: string }) {
  return (
    <div className={`rounded-lg border border-edge bg-surface p-4 ${className}`}>{children}</div>
  );
}

const badgeTones = {
  neutral: "bg-surface-2 text-muted",
  ok: "bg-ok/15 text-ok",
  warn: "bg-warn/15 text-warn",
  danger: "bg-danger/15 text-danger",
  accent: "bg-accent/15 text-accent",
} as const;

export type BadgeTone = keyof typeof badgeTones;

export function Badge({ tone = "neutral", children }: { tone?: BadgeTone; children: ReactNode }) {
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium whitespace-nowrap ${badgeTones[tone]}`}
    >
      {children}
    </span>
  );
}

export function Spinner() {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-center gap-2 py-10 text-muted" role="status">
      <span className="size-4 animate-spin rounded-full border-2 border-muted border-t-transparent" />
      <span className="text-sm">{t("common.loading")}</span>
    </div>
  );
}

export function EmptyState({ message }: { message: string }) {
  return (
    <div className="rounded-lg border border-dashed border-edge py-10 text-center text-sm text-muted">
      {message}
    </div>
  );
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const { t } = useTranslation();
  let message = t("errors.generic");
  if (error instanceof ApiError) {
    if (error.isUnauthorized) {
      message = t("errors.unauthorized");
    } else if (error.isNotFound) {
      message = t("errors.notFound");
    } else {
      message = error.message;
    }
  } else if (error instanceof TypeError) {
    message = t("errors.network");
  }
  return (
    <div className="rounded-lg border border-danger/40 bg-danger/5 p-4 text-sm">
      <p className="text-danger">{message}</p>
      {onRetry !== undefined ? (
        <button
          type="button"
          onClick={onRetry}
          className="mt-2 rounded-md border border-edge bg-surface px-3 py-1 text-xs font-medium hover:bg-surface-2"
        >
          {t("common.retry")}
        </button>
      ) : null}
    </div>
  );
}

export function Button({
  children,
  onClick,
  type = "button",
  variant = "primary",
  disabled = false,
}: {
  children: ReactNode;
  onClick?: () => void;
  type?: "button" | "submit";
  variant?: "primary" | "ghost" | "danger";
  disabled?: boolean;
}) {
  const styles = {
    primary: "bg-accent text-accent-ink hover:opacity-90",
    ghost: "border border-edge bg-surface hover:bg-surface-2",
    danger: "border border-danger/40 text-danger hover:bg-danger/10",
  } as const;
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      className={`rounded-md px-3 py-1.5 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-50 ${styles[variant]}`}
    >
      {children}
    </button>
  );
}

export function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <label className="block">
      <span className="mb-1 block text-sm font-medium">{label}</span>
      {children}
      {hint !== undefined ? <span className="mt-1 block text-xs text-muted">{hint}</span> : null}
    </label>
  );
}

export const inputClass =
  "w-full rounded-md border border-edge bg-surface px-3 py-2 text-sm outline-none focus:border-accent focus:ring-1 focus:ring-accent";

export function Select({
  value,
  onChange,
  children,
  ariaLabel,
}: {
  value: string;
  onChange: (value: string) => void;
  children: ReactNode;
  ariaLabel?: string;
}) {
  return (
    <select
      value={value}
      aria-label={ariaLabel}
      onChange={(e) => onChange(e.target.value)}
      className="rounded-md border border-edge bg-surface px-2 py-1.5 text-sm outline-none focus:border-accent"
    >
      {children}
    </select>
  );
}
