import { Search, Star } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { useGet } from "@/lib/hooks";
import { formatMB, percent } from "@/lib/format";
import { availableTools, matchTool, toolGroups, type Tool } from "@/lib/catalog";
import type { Account, AccountSample, Usage } from "@/lib/types";
import { useAccountId, useBase } from "@/state/scope";
import { usePanel } from "@/state/panel";
import { TimeChart } from "@/components/ui/Chart";
import { Badge, Card, ErrorNote, Loading } from "@/components/ui/Layout";

function StatRow({ label, used, limit, fmt }: { label: string; used: number; limit: number; fmt?: (n: number) => string }) {
  const f = fmt ?? String;
  const p = percent(used, limit);
  return (
    <div className="min-w-0 rounded-lg border border-line bg-surface px-3.5 py-3 shadow-card">
      <div className="truncate text-[12px] text-muted">{label}</div>
      <div className="mt-0.5 truncate tabular-nums">
        <span className="text-[18px] font-semibold text-ink">{f(used)}</span>
        <span className="text-[12px] text-muted"> / {limit ? f(limit) : "∞"}</span>
      </div>
      <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-track">
        <div className={`h-full rounded-full ${p >= 90 ? "bg-bad" : "bg-accent"}`} style={{ width: `${limit ? Math.max(p, used ? 2 : 0) : 0}%` }} />
      </div>
    </div>
  );
}

const DEFAULT_PINS = ["files", "email", "databases", "domains", "ssl", "wordpress"];
const PINS_KEY = "breefer-pins";

function usePins() {
  const [pins, setPins] = useState<string[]>(() => {
    try {
      const v: unknown = JSON.parse(localStorage.getItem(PINS_KEY) ?? "null");
      if (Array.isArray(v)) return v.filter((x): x is string => typeof x === "string");
    } catch {
      // storage is unavailable or holds junk: use the defaults
    }
    return DEFAULT_PINS;
  });
  const toggle = (to: string) => {
    const next = pins.includes(to) ? pins.filter((p) => p !== to) : [...pins, to];
    setPins(next);
    try {
      localStorage.setItem(PINS_KEY, JSON.stringify(next));
    } catch {
      // not remembered, still works for this visit
    }
  };
  return { pins, toggle };
}

function Tile({ tool, to, pinned, onPin }: { tool: Tool; to: string; pinned: boolean; onPin: () => void }) {
  return (
    <div className="group relative">
      <Link to={to} className="flex h-full items-center gap-2.5 rounded-md py-2.5 pl-2.5 pr-7 text-[13px] leading-tight text-ink transition-colors hover:bg-surface2">
        <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-accent-soft text-accent transition-colors group-hover:bg-accent group-hover:text-accent-fg">
          <tool.icon className="size-4" aria-hidden />
        </span>
        <span className="min-w-0 flex-1 [overflow-wrap:anywhere]">{tool.label}</span>
      </Link>
      <button
        type="button"
        aria-pressed={pinned}
        aria-label={`${pinned ? "Remove" : "Add"} ${tool.label} ${pinned ? "from" : "to"} favourites`}
        onClick={onPin}
        className={`absolute right-0.5 top-1/2 flex size-6 -translate-y-1/2 items-center justify-center rounded transition-opacity ${pinned ? "text-accent" : "text-subtle opacity-0 focus-visible:opacity-100 group-hover:opacity-100 [@media(pointer:coarse)]:opacity-100"}`}
      >
        <Star className="size-3.5" fill={pinned ? "currentColor" : "none"} aria-hidden />
      </button>
    </div>
  );
}

export default function HostingOverview() {
  const { pins, toggle } = usePins();
  const aid = useAccountId();
  const base = useBase();
  const panel = usePanel();
  const [q, setQ] = useState("");
  const usage = useGet<Usage>(`/accounts/${aid}/usage`);
  const acct = useGet<Account>(`/accounts/${aid}`);
  const metrics = useGet<{ samples: AccountSample[]; limits: { cpu_pct: number; mem_mb: number }; demo: boolean }>(`/accounts/${aid}/metrics?range=24h`);

  if (usage.isLoading || acct.isLoading) return <Loading />;
  if (usage.error || !usage.data || !acct.data) return <ErrorNote error={usage.error ?? acct.error} />;
  const u = usage.data;
  const a = acct.data;
  const m = metrics.data;
  const times = m?.samples.map((s) => s.t) ?? [];
  // An administrator managing the account sees every tool; the customer sees
  // what the package's feature list allows.
  const managed = aid !== "me";
  const list = availableTools(managed ? null : a.disabled_features, managed ? "full" : a.shell).filter((t) => matchTool(t, q));
  const href = (to: string) => (to.startsWith("/") ? to : to ? `${base}/${to}` : base);

  return (
    <>
      <div className="mb-5 flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2.5">
            <h1 className="text-[19px] font-semibold tracking-tight">{a.primary_domain}</h1>
            <Badge tone={a.status === "active" ? "ok" : "bad"}>{a.status === "active" ? "Active" : "Suspended"}</Badge>
          </div>
          <p className="mt-0.5 text-[13px] text-muted">
            {a.sys_user} · {u.package} plan
          </p>
        </div>
        <label className="relative w-full max-w-xs">
          <span className="sr-only">Filter tools</span>
          <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-subtle" aria-hidden />
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Filter tools"
            className="h-8 w-full rounded-md border border-strong bg-surface pl-8 pr-2 text-[13px] placeholder:text-subtle focus:border-accent focus:outline-none focus:ring-2 focus:ring-[var(--focus)]"
          />
        </label>
      </div>

      {a.status === "suspended" && (
        <div className="mb-4 rounded-md bg-warn-soft px-3 py-2 text-[13px] text-warn">
          This account is suspended{a.suspend_reason ? `: ${a.suspend_reason}` : ""}. Websites and email are offline.
        </div>
      )}

      {!q && (
        <div className="mb-5 grid grid-cols-2 gap-3 sm:grid-cols-3 2xl:grid-cols-6">
          <StatRow label="Disk usage" used={u.disk.used} limit={u.disk.limit} fmt={formatMB} />
          <StatRow label="Bandwidth" used={u.bandwidth.used} limit={u.bandwidth.limit} fmt={formatMB} />
          <StatRow label="Domains" used={u.domains.used} limit={u.domains.limit} />
          <StatRow label="Email accounts" used={u.mailboxes.used} limit={u.mailboxes.limit} />
          <StatRow label="Databases" used={u.databases.used} limit={u.databases.limit} />
          <StatRow label="Applications" used={u.apps.used} limit={u.apps.limit} />
        </div>
      )}

      <div className="grid gap-5 xl:grid-cols-[1fr_280px]">
        <div className="min-w-0 space-y-4">
          {!q && list.some((t) => pins.includes(t.to)) && (
            <section className="rounded-lg border border-line bg-surface shadow-card">
              <h2 className="border-b border-line px-4 py-2.5 text-[12px] font-semibold uppercase tracking-wide text-muted">Favourites</h2>
              <div className="grid grid-cols-2 gap-px p-1.5 sm:grid-cols-3">
                {list
                  .filter((t) => pins.includes(t.to))
                  .map((t) => (
                    <Tile key={t.label} tool={t} to={href(t.to)} pinned onPin={() => toggle(t.to)} />
                  ))}
              </div>
            </section>
          )}
          {toolGroups.map((g) => {
            const tiles = list.filter((t) => t.group === g);
            if (!tiles.length) return null;
            return (
              <section key={g} className="rounded-lg border border-line bg-surface shadow-card">
                <h2 className="border-b border-line px-4 py-2.5 text-[12px] font-semibold uppercase tracking-wide text-muted">{g}</h2>
                <div className="grid grid-cols-2 gap-px p-1.5 sm:grid-cols-3 2xl:grid-cols-4">
                  {tiles.map((t) => (
                    <Tile key={t.label} tool={t} to={href(t.to)} pinned={pins.includes(t.to)} onPin={() => toggle(t.to)} />
                  ))}
                </div>
              </section>
            );
          })}
          {!list.length && <p className="text-[13px] text-muted">No tools match “{q}”.</p>}

          <div className="grid gap-4 lg:grid-cols-2">
            <Card title="CPU" description={m ? `Last 24 hours · limit ${m.limits.cpu_pct}%` : undefined}>
              <TimeChart
                label="CPU usage of this account over the last 24 hours"
                times={times}
                series={[{ name: "CPU", color: "var(--series-1)", values: m?.samples.map((s) => s.cpu) ?? [] }]}
                format={(v) => `${Math.round(v)}%`}
                max={m?.limits.cpu_pct}
                height={140}
              />
            </Card>
            <Card title="Memory" description={m ? `Last 24 hours · limit ${formatMB(m.limits.mem_mb)}` : undefined}>
              <TimeChart
                label="Memory usage of this account over the last 24 hours"
                times={times}
                series={[{ name: "Memory", color: "var(--series-1)", values: m?.samples.map((s) => s.mem_mb) ?? [] }]}
                format={formatMB}
                max={m?.limits.mem_mb}
                height={140}
              />
            </Card>
          </div>
          {m?.demo && <p className="text-[12px] text-subtle">Dry-run mode: resource charts show demo data.</p>}
        </div>

        <aside className="space-y-4">
          <Card title="General information" flush>
            <dl className="divide-y divide-[var(--line)] text-[12.5px]">
              {[
                ["Current user", a.sys_user],
                ["Primary domain", a.primary_domain],
                ["Home directory", a.home_dir],
                ["Plan", u.package],
                ["Shell access", a.shell === "full" ? "Full shell" : a.shell === "sftp" ? "SFTP only" : "Disabled"],
                ...(panel.hostname ? [["Server", panel.hostname]] : []),
              ].map(([k, v]) => (
                <div key={k} className="px-4 py-2">
                  <dt className="text-muted">{k}</dt>
                  <dd className="truncate font-medium text-ink" title={v}>
                    {v}
                  </dd>
                </div>
              ))}
            </dl>
          </Card>
        </aside>
      </div>
    </>
  );
}
