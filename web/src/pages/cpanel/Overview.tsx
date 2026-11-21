import {
  AppWindow,
  Archive,
  ArrowUpRight,
  Clock,
  Code2,
  Database,
  FileWarning,
  FolderOpen,
  Globe,
  HardDrive,
  KeyRound,
  LifeBuoy,
  Mail,
  Network,
  Package,
  Repeat,
  ScrollText,
  ShieldCheck,
  SquareTerminal,
  type LucideIcon,
} from "lucide-react";
import { Link } from "react-router-dom";
import { useGet } from "@/lib/hooks";
import { formatMB, percent } from "@/lib/format";
import type { Account, AccountSample, Usage } from "@/lib/types";
import { useAccountId, useBase } from "@/state/scope";
import { TimeChart } from "@/components/ui/Chart";
import { Badge, Card, ErrorNote, Loading } from "@/components/ui/Layout";

interface Tile {
  to: string;
  label: string;
  icon: LucideIcon;
}

const groups = (shell: boolean): { title: string; tiles: Tile[] }[] => [
  {
    title: "Files",
    tiles: [
      { to: "files", label: "File Manager", icon: FolderOpen },
      { to: "disk", label: "Disk Usage", icon: HardDrive },
      { to: "backups", label: "Backups", icon: Archive },
    ],
  },
  {
    title: "Domains",
    tiles: [
      { to: "domains", label: "Domains", icon: Globe },
      { to: "redirects", label: "Redirects", icon: Repeat },
      { to: "dns", label: "Zone Editor", icon: Network },
      { to: "error-pages", label: "Error Pages", icon: FileWarning },
    ],
  },
  {
    title: "Email",
    tiles: [
      { to: "email", label: "Email Accounts", icon: Mail },
      { to: "email?tab=webmail", label: "Webmail", icon: Mail },
    ],
  },
  {
    title: "Databases",
    tiles: [
      { to: "databases", label: "MySQL", icon: Database },
      { to: "databases?pma=1", label: "phpMyAdmin", icon: Database },
    ],
  },
  {
    title: "Software",
    tiles: [
      { to: "installer", label: "App Installer", icon: Package },
      { to: "wordpress", label: "WordPress", icon: AppWindow },
      { to: "php", label: "PHP Manager", icon: Code2 },
      { to: "apps", label: "Node / Python / Ruby", icon: AppWindow },
    ],
  },
  {
    title: "Security & Advanced",
    tiles: [
      { to: "ssl", label: "SSL/TLS", icon: ShieldCheck },
      { to: "ssh", label: "SSH Access", icon: KeyRound },
      ...(shell ? [{ to: "terminal", label: "Terminal", icon: SquareTerminal }] : []),
      { to: "cron", label: "Cron Jobs", icon: Clock },
      { to: "logs", label: "Logs", icon: ScrollText },
      { to: "/tickets", label: "Support", icon: LifeBuoy },
    ],
  },
];

function StatTile({ label, used, limit, fmt }: { label: string; used: number; limit: number; fmt?: (n: number) => string }) {
  const f = fmt ?? String;
  const p = percent(used, limit);
  return (
    <div className="rounded-lg border border-line bg-surface px-4 py-3 shadow-card">
      <div className="text-[12px] font-medium text-muted">{label}</div>
      <div className="mt-1 flex items-baseline gap-1.5">
        <span className="text-[20px] font-semibold leading-none tracking-tight tabular-nums">{f(used)}</span>
        <span className="text-[12px] text-muted">/ {limit ? f(limit) : "∞"}</span>
      </div>
      <div className="mt-2.5 h-1 overflow-hidden rounded-full bg-surface2">
        <div className={`h-full rounded-full ${p >= 90 ? "bg-bad" : "bg-accent"}`} style={{ width: `${limit ? Math.max(p, used ? 2 : 0) : 0}%` }} />
      </div>
    </div>
  );
}

export default function HostingOverview() {
  const aid = useAccountId();
  const base = useBase();
  const usage = useGet<Usage>(`/accounts/${aid}/usage`);
  const acct = useGet<Account>(`/accounts/${aid}`);
  const metrics = useGet<{ samples: AccountSample[]; limits: { cpu_pct: number; mem_mb: number }; demo: boolean }>(
    `/accounts/${aid}/metrics?range=24h`,
  );

  if (usage.isLoading || acct.isLoading) return <Loading />;
  if (usage.error || !usage.data || !acct.data) return <ErrorNote error={usage.error ?? acct.error} />;
  const u = usage.data;
  const a = acct.data;
  const m = metrics.data;
  const times = m?.samples.map((s) => s.t) ?? [];

  return (
    <>
      <div className="mb-5 flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2.5">
            <h1 className="text-[19px] font-semibold tracking-tight">{a.primary_domain}</h1>
            <Badge tone={a.status === "active" ? "ok" : "bad"}>{a.status === "active" ? "Active" : "Suspended"}</Badge>
          </div>
          <p className="mt-0.5 text-[13px] text-muted">
            {a.sys_user} · {u.package} plan · home <span className="font-mono text-[12px]">{a.home_dir}</span>
          </p>
        </div>
      </div>

      {a.status === "suspended" && (
        <div className="mb-4 rounded-md bg-warn-soft px-3 py-2 text-[13px] text-warn">
          This account is suspended{a.suspend_reason ? `: ${a.suspend_reason}` : ""}. Websites and email are offline.
        </div>
      )}

      <div className="mb-5 grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
        <StatTile label="Disk" used={u.disk.used} limit={u.disk.limit} fmt={formatMB} />
        <StatTile label="Bandwidth" used={u.bandwidth.used} limit={u.bandwidth.limit} fmt={formatMB} />
        <StatTile label="Domains" used={u.domains.used} limit={u.domains.limit} />
        <StatTile label="Email accounts" used={u.mailboxes.used} limit={u.mailboxes.limit} />
        <StatTile label="Databases" used={u.databases.used} limit={u.databases.limit} />
        <StatTile label="Applications" used={u.apps.used} limit={u.apps.limit} />
      </div>

      <div className="mb-6 grid gap-4 lg:grid-cols-2">
        <Card title="CPU" description={m ? `Last 24 hours · plan limit ${m.limits.cpu_pct}% (${m.limits.cpu_pct / 100} ${m.limits.cpu_pct === 100 ? "core" : "cores"})` : undefined}>
          <TimeChart
            label="CPU usage of this account over the last 24 hours"
            times={times}
            series={[{ name: "CPU", color: "var(--series-1)", values: m?.samples.map((s) => s.cpu) ?? [] }]}
            format={(v) => `${Math.round(v)}%`}
            max={m?.limits.cpu_pct}
            height={150}
          />
        </Card>
        <Card title="Memory" description={m ? `Last 24 hours · plan limit ${formatMB(m.limits.mem_mb)}` : undefined}>
          <TimeChart
            label="Memory usage of this account over the last 24 hours"
            times={times}
            series={[{ name: "Memory", color: "var(--series-1)", values: m?.samples.map((s) => s.mem_mb) ?? [] }]}
            format={formatMB}
            max={m?.limits.mem_mb}
            height={150}
          />
        </Card>
      </div>
      {m?.demo && <p className="-mt-4 mb-6 text-[12px] text-subtle">Dry-run mode: resource charts show demo data.</p>}

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        {groups(a.shell === "full").map((g) => (
          <section key={g.title} className="rounded-lg border border-line bg-surface shadow-card">
            <h2 className="border-b border-line px-4 py-2.5 text-[12px] font-semibold uppercase tracking-wide text-muted">{g.title}</h2>
            <div className="grid grid-cols-2 p-1.5">
              {g.tiles.map((t) => (
                <Link
                  key={t.to}
                  to={t.to.startsWith("/") ? t.to : `${base}/${t.to}`}
                  className="group flex items-center gap-2.5 rounded-md px-2.5 py-2.5 text-[13px] leading-tight text-ink transition-colors hover:bg-surface2"
                >
                  <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-accent-soft text-accent">
                    <t.icon className="size-4" aria-hidden />
                  </span>
                  <span className="min-w-0 flex-1">{t.label}</span>
                  <ArrowUpRight className="size-3.5 text-subtle opacity-0 transition-opacity group-hover:opacity-100" aria-hidden />
                </Link>
              ))}
            </div>
          </section>
        ))}
      </div>
    </>
  );
}
