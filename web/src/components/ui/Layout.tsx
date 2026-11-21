import clsx from "clsx";
import { ChevronRight, Loader2, type LucideIcon } from "lucide-react";
import { Link, useLocation } from "react-router-dom";
import { tools, whmNav } from "@/lib/catalog";
import type { HTMLAttributes, ReactNode, TdHTMLAttributes, ThHTMLAttributes } from "react";

/** Resolves the catalog entry for the current route (cPanel tool or WHM page). */
function useCurrentTool(): { label: string; group: string; icon: LucideIcon; home: string; blurb?: string } | null {
  const { pathname } = useLocation();
  const m = pathname.match(/^\/(?:hosting|accounts\/[^/]+)\/([^/?]+)/);
  if (m) {
    const t = tools.find((x) => x.to.split("?")[0] === m[1]);
    const home = pathname.startsWith("/hosting") ? "/hosting" : pathname.slice(0, pathname.indexOf("/", 10));
    if (t) return { label: t.label, group: t.group, icon: t.icon, home, blurb: t.blurb };
  }
  for (const c of whmNav) {
    const it = c.items.find((x) => x.to !== "/" && x.to === pathname);
    if (it) return { label: it.label, group: c.title, icon: it.icon, home: "/", blurb: it.blurb };
  }
  const t = tools.find((x) => x.to.startsWith("/") && x.to === pathname);
  if (t) return { label: t.label, group: t.group, icon: t.icon, home: "/hosting", blurb: t.blurb };
  return null;
}

export function PageHeader({
  title,
  description,
  actions,
  icon,
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
  icon?: LucideIcon;
}) {
  const tool = useCurrentTool();
  const Icon = icon ?? tool?.icon;
  return (
    <div className="mb-5">
      {tool && (
        <nav aria-label="Breadcrumb" className="mb-2 flex items-center gap-1 text-[12px] text-muted">
          <Link to={tool.home} className="hover:text-ink hover:underline">
            Home
          </Link>
          <ChevronRight className="size-3 text-subtle" aria-hidden />
          <span>{tool.group}</span>
          <ChevronRight className="size-3 text-subtle" aria-hidden />
          <span className="text-ink" aria-current="page">
            {title}
          </span>
        </nav>
      )}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          {Icon && (
            <div className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg border border-line bg-accent-soft text-accent">
              <Icon className="size-[18px]" aria-hidden />
            </div>
          )}
          <div className="min-w-0">
            <h1 className="text-[19px] font-semibold tracking-tight text-ink">{title}</h1>
            {(description ?? tool?.blurb) && <p className="mt-0.5 max-w-2xl text-[13px] text-muted">{description ?? tool?.blurb}</p>}
          </div>
        </div>
        {actions && <div className="flex shrink-0 flex-wrap items-center gap-2 sm:flex-nowrap">{actions}</div>}
      </div>
    </div>
  );
}

/** A short "how it works" note under a page header, collapsible. */
export function HelpNote({ children, title = "How this works" }: { children: ReactNode; title?: string }) {
  return (
    <details className="group mb-4 rounded-lg border border-line bg-surface2/60 px-4 py-2.5 text-[13px] text-muted open:pb-3">
      <summary className="cursor-pointer select-none font-medium text-ink marker:text-subtle">{title}</summary>
      <div className="mt-2 space-y-1.5 leading-relaxed">{children}</div>
    </details>
  );
}

/** Small stat block used at the top of tool pages. */
export function Stat({ label, value, hint, tone }: { label: string; value: ReactNode; hint?: string; tone?: "ok" | "warn" | "bad" }) {
  return (
    <div className="min-w-0 rounded-lg border border-line bg-surface px-4 py-3 shadow-card">
      <div className="text-[11.5px] font-medium uppercase tracking-wide text-muted">{label}</div>
      <div className={clsx("mt-1 truncate text-[20px] font-semibold tabular-nums", tone === "bad" ? "text-bad" : tone === "warn" ? "text-warn" : tone === "ok" ? "text-ok" : "text-ink")}>{value}</div>
      {hint && <div className="mt-0.5 truncate text-[12px] text-muted">{hint}</div>}
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
    <section className={clsx("min-w-0 rounded-lg border border-line bg-surface shadow-card", className)}>
      {(title || actions) && (
        <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3">
          <div className="min-w-0">
            {title && <h2 className="text-[13.5px] font-semibold text-ink">{title}</h2>}
            {description && <p className="mt-0.5 text-[12.5px] text-muted [overflow-wrap:anywhere]">{description}</p>}
          </div>
          {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
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
    <div className="relative overflow-x-auto">
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
    <div role="tablist" className="mb-4 flex flex-wrap gap-x-1 border-b border-line">
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

/** Compact single-choice control for filters such as a time range. */
export function Segmented<T extends string>({ options, value, onChange, label }: { options: { id: T; label: string }[]; value: T; onChange: (v: T) => void; label: string }) {
  return (
    <div role="radiogroup" aria-label={label} className="inline-flex rounded-md border border-strong bg-surface p-0.5">
      {options.map((o) => (
        <button
          key={o.id}
          role="radio"
          aria-checked={value === o.id}
          onClick={() => onChange(o.id)}
          className={clsx(
            "whitespace-nowrap rounded px-2.5 py-1 text-[12.5px] font-medium transition-colors",
            value === o.id ? "bg-accent-soft text-accent" : "text-muted hover:text-ink",
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
