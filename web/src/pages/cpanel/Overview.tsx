import { Search } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { useGet } from "@/lib/hooks";
import { formatMB, percent } from "@/lib/format";
import { availableTools, matchTool, toolGroups } from "@/lib/catalog";
import type { Account, AccountSample, Usage } from "@/lib/types";
import { useAccountId, useBase } from "@/state/scope";
import { usePanel } from "@/state/panel";
import { TimeChart } from "@/components/ui/Chart";
import { Badge, Card, ErrorNote, Loading } from "@/components/ui/Layout";

function StatRow({ label, used, limit, fmt }: { label: string; used: number; limit: number; fmt?: (n: number) => string }) {
  const f = fmt ?? String;
  const p = percent(used, limit);
  return (
    <div className="px-4 py-2.5">
      <div className="flex items-baseline justify-between gap-2 text-[12.5px]">
        <span className="text-muted">{label}</span>
        <span className="tabular-nums">
          <span className="font-medium text-ink">{f(used)}</span>
          <span className="text-subtle"> / {limit ? f(limit) : "∞"}</span>
        </span>
      </div>
      <div className="mt-1.5 h-1 overflow-hidden rounded-full bg-surface2">
        <div className={`h-full rounded-full ${p >= 90 ? "bg-bad" : "bg-accent"}`} style={{ width: `${limit ? Math.max(p, used ? 2 : 0) : 0}%` }} />
      </div>
    </div>
  );
}

export default function HostingOverview() {
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

      <div className="grid gap-5 xl:grid-cols-[1fr_300px]">
        <div className="min-w-0 space-y-4">
          {toolGroups.map((g) => {
            const tiles = list.filter((t) => t.group === g);
            if (!tiles.length) return null;
            return (
              <section key={g} className="rounded-lg border border-line bg-surface shadow-card">
                <h2 className="border-b border-line px-4 py-2.5 text-[12px] font-semibold uppercase tracking-wide text-muted">{g}</h2>
                <div className="grid grid-cols-2 gap-px p-1.5 sm:grid-cols-3 lg:grid-cols-4">
                  {tiles.map((t) => (
                    <Link
                      key={t.label}
                      to={href(t.to)}
                      className="flex items-center gap-2.5 rounded-md px-2.5 py-2.5 text-[13px] leading-tight text-ink transition-colors hover:bg-surface2"
                    >
                      <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-accent-soft text-accent">
                        <t.icon className="size-4" aria-hidden />
                      </span>
                      <span className="min-w-0 flex-1">{t.label}</span>
                    </Link>
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
          <Card title="Statistics" flush>
            <div className="divide-y divide-[var(--line)]">
              <StatRow label="Disk usage" used={u.disk.used} limit={u.disk.limit} fmt={formatMB} />
              <StatRow label="Bandwidth" used={u.bandwidth.used} limit={u.bandwidth.limit} fmt={formatMB} />
              <StatRow label="Domains" used={u.domains.used} limit={u.domains.limit} />
              <StatRow label="Email accounts" used={u.mailboxes.used} limit={u.mailboxes.limit} />
              <StatRow label="Databases" used={u.databases.used} limit={u.databases.limit} />
              <StatRow label="Applications" used={u.apps.used} limit={u.apps.limit} />
            </div>
          </Card>
        </aside>
      </div>
    </>
  );
}
