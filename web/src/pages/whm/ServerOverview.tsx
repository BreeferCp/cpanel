import { Link } from "react-router-dom";
import { useGet } from "@/lib/hooks";
import { formatMB, formatUptime } from "@/lib/format";
import type { ServerStatus } from "@/lib/types";
import { useAuth } from "@/state/auth";
import { Badge, Card, ErrorNote, Loading, Meter, PageHeader } from "@/components/ui/Layout";

function Stat({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded-lg border border-line bg-surface px-4 py-3 shadow-card">
      <div className="text-[12px] font-medium text-muted">{label}</div>
      <div className="mt-1 text-[22px] font-semibold leading-none tracking-tight text-ink tabular-nums">{value}</div>
      {sub && <div className="mt-1.5 text-[12px] text-muted">{sub}</div>}
    </div>
  );
}

export default function ServerOverview() {
  const { user } = useAuth();
  const isRoot = user?.role === "root";
  // Resellers do not see host metrics; they see their own customer totals only.
  const q = useGet<ServerStatus>("/server/status", isRoot);
  const accounts = useGet<{ id: string; status: string }[]>("/accounts");

  if (isRoot && q.isLoading) return <Loading />;
  if (isRoot && q.error) return <ErrorNote error={q.error} />;
  const s = q.data;
  const list = accounts.data ?? [];

  return (
    <>
      <PageHeader
        title={isRoot && s ? s.hostname : "Overview"}
        description={isRoot && s ? `BreeferCP ${s.version} on ${s.os}, up ${formatUptime(s.uptime_s)}` : "Your customers at a glance."}
        actions={s?.dry_run ? <Badge tone="warn">Dry-run mode: no host changes are applied</Badge> : undefined}
      />

      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat label="Accounts" value={String(list.length)} sub={`${list.filter((a) => a.status === "suspended").length} suspended`} />
        {isRoot && s && (
          <>
            <Stat label="Load average" value={s.load[0]?.toFixed(2) ?? "0.00"} sub={`${s.cpus} CPU cores`} />
            <Stat label="Memory" value={formatMB(s.memory.used_mb)} sub={`of ${formatMB(s.memory.total_mb)}`} />
            <Stat label="Database" value={s.database === "sqlite" ? "SQLite" : "PostgreSQL"} sub={s.database === "sqlite" ? "Single node" : "Clustered"} />
          </>
        )}
      </div>

      {isRoot && s && (
        <div className="grid gap-4 lg:grid-cols-2">
          <Card title="Capacity">
            <div className="space-y-5">
              <div>
                <div className="mb-1.5 text-[12.5px] font-medium text-muted">Memory</div>
                <Meter used={s.memory.used_mb} limit={s.memory.total_mb} format={formatMB} />
              </div>
              <div>
                <div className="mb-1.5 text-[12.5px] font-medium text-muted">Disk (account storage)</div>
                <Meter used={s.disk.used_mb} limit={s.disk.total_mb} format={formatMB} />
              </div>
            </div>
          </Card>
          <Card title="Shortcuts">
            <ul className="-my-1 divide-y divide-line text-[13px]">
              {[
                ["Create an account", "/accounts"],
                ["Review services", "/services"],
                ["Edit firewall rules", "/firewall"],
                ["Read the audit log", "/audit"],
              ].map(([label, to]) => (
                <li key={to}>
                  <Link to={to!} className="block py-2.5 text-ink hover:text-accent">
                    {label}
                  </Link>
                </li>
              ))}
            </ul>
          </Card>
        </div>
      )}
    </>
  );
}
