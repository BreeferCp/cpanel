import clsx from "clsx";
import { Loader2, type LucideIcon } from "lucide-react";
import type { HTMLAttributes, ReactNode, TdHTMLAttributes, ThHTMLAttributes } from "react";

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
}) {
  return (
    <div className="mb-5 flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        <h1 className="text-[19px] font-semibold tracking-tight text-ink">{title}</h1>
        {description && <p className="mt-0.5 max-w-2xl text-[13px] text-muted">{description}</p>}
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </div>
  );
}

export function Card({
  title,
  description,
  actions,
  children,
  className,
  flush,
}: {
  title?: string;
  description?: string;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  flush?: boolean;
}) {
  return (
    <section className={clsx("rounded-lg border border-line bg-surface shadow-card", className)}>
      {(title || actions) && (
        <header className="flex items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div className="min-w-0">
            {title && <h2 className="text-[13.5px] font-semibold text-ink">{title}</h2>}
            {description && <p className="mt-0.5 text-[12.5px] text-muted">{description}</p>}
          </div>
          {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
        </header>
      )}
      <div className={flush ? "" : "p-4"}>{children}</div>
    </section>
  );
}

type Tone = "neutral" | "ok" | "warn" | "bad" | "accent";
const tones: Record<Tone, string> = {
  neutral: "bg-surface2 text-muted border-line",
  ok: "bg-ok-soft text-ok border-transparent",
  warn: "bg-warn-soft text-warn border-transparent",
  bad: "bg-bad-soft text-bad border-transparent",
  accent: "bg-accent-soft text-accent border-transparent",
};

export function Badge({ tone = "neutral", children }: { tone?: Tone; children: ReactNode }) {
  return (
    <span className={clsx("inline-flex items-center gap-1 rounded border px-1.5 py-px text-[11.5px] font-medium", tones[tone])}>
      {children}
    </span>
  );
}

export function StatusDot({ ok }: { ok: boolean }) {
  return <span className={clsx("inline-block size-2 rounded-full", ok ? "bg-ok" : "bg-bad")} aria-hidden />;
}

export function Spinner({ className }: { className?: string }) {
  return <Loader2 className={clsx("size-4 animate-spin text-subtle", className)} aria-label="Loading" />;
}

export function Loading() {
  return (
    <div className="flex items-center justify-center py-16">
      <Spinner className="size-5" />
    </div>
  );
}

export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
}: {
  icon: LucideIcon;
  title: string;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex flex-col items-center px-6 py-12 text-center">
      <div className="mb-3 flex size-10 items-center justify-center rounded-lg border border-line bg-surface2 text-subtle">
        <Icon className="size-5" aria-hidden />
      </div>
      <h3 className="text-[13.5px] font-semibold text-ink">{title}</h3>
      {description && <p className="mt-1 max-w-sm text-[13px] text-muted">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

export function ErrorNote({ error }: { error: unknown }) {
  if (!error) return null;
  const msg = error instanceof Error ? error.message : String(error);
  return (
    <div role="alert" className="rounded-md border border-transparent bg-bad-soft px-3 py-2 text-[13px] text-bad">
      {msg}
    </div>
  );
}

export function Meter({ used, limit, format }: { used: number; limit: number; format?: (n: number) => string }) {
  const f = format ?? String;
  const pct = limit ? Math.min(100, (used / limit) * 100) : 0;
  const hot = limit > 0 && pct >= 90;
  return (
    <div>
      <div className="flex items-baseline justify-between text-[12.5px]">
        <span className="font-medium text-ink">{f(used)}</span>
        <span className="text-muted">{limit ? `of ${f(limit)}` : "unlimited"}</span>
      </div>
      <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-surface2 ring-1 ring-inset ring-[var(--line)]">
        <div
          className={clsx("h-full rounded-full transition-all", hot ? "bg-bad" : "bg-accent")}
          style={{ width: `${limit ? Math.max(pct, used > 0 ? 2 : 0) : 0}%` }}
          role="progressbar"
          aria-valuenow={used}
          aria-valuemax={limit || undefined}
        />
      </div>
    </div>
  );
}

// ---- table ------------------------------------------------------------------

export function Table({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div className="overflow-x-auto">
      <table className={clsx("w-full border-collapse text-left text-[13px]", className)}>{children}</table>
    </div>
  );
}
export function Th({ className, ...rest }: ThHTMLAttributes<HTMLTableCellElement>) {
  return (
    <th
      className={clsx(
        "whitespace-nowrap border-b border-line bg-surface2 px-4 py-2 text-[11.5px] font-semibold uppercase tracking-wide text-muted first:rounded-tl-lg last:rounded-tr-lg",
        className,
      )}
      {...rest}
    />
  );
}
export function Td({ className, ...rest }: TdHTMLAttributes<HTMLTableCellElement>) {
  return <td className={clsx("border-b border-line px-4 py-2.5 align-middle", className)} {...rest} />;
}
export function Tr({ className, ...rest }: HTMLAttributes<HTMLTableRowElement>) {
  return <tr className={clsx("transition-colors hover:bg-surface2/60 [&:last-child>td]:border-b-0", className)} {...rest} />;
}

export function Mono({ children, className }: { children: ReactNode; className?: string }) {
  return <span className={clsx("font-mono text-[12.5px]", className)}>{children}</span>;
}

// ---- tabs -------------------------------------------------------------------

export function Tabs<T extends string>({
  tabs,
  value,
  onChange,
}: {
  tabs: { id: T; label: string }[];
  value: T;
  onChange: (v: T) => void;
}) {
  return (
    <div role="tablist" className="mb-4 flex gap-1 border-b border-line">
      {tabs.map((t) => (
        <button
          key={t.id}
          role="tab"
          aria-selected={value === t.id}
          onClick={() => onChange(t.id)}
          className={clsx(
            "-mb-px border-b-2 px-3 py-2 text-[13px] font-medium transition-colors",
            value === t.id ? "border-accent text-ink" : "border-transparent text-muted hover:text-ink",
          )}
        >
          {t.label}
        </button>
      ))}
    </div>
  );
}
