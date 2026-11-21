import { useState } from "react";
import { Link } from "react-router-dom";
import { useGet } from "@/lib/hooks";
import { formatMB, formatUptime, percent } from "@/lib/format";
import type { Account, HostSample, ServerStatus, Ticket, TopAccount } from "@/lib/types";
import { useAuth } from "@/state/auth";
import { Sparkline, TimeChart } from "@/components/ui/Chart";
import { Badge, Card, ErrorNote, Loading, Meter, PageHeader, Segmented, Table, Td, Th, Tr } from "@/components/ui/Layout";

type Range = "1h" | "6h" | "24h";

function Stat({ label, value, sub, trend }: { label: string; value: string; sub?: string; trend?: number[] }) {
  return (
    <div className="flex items-end justify-between gap-3 rounded-lg border border-line bg-surface px-4 py-3 shadow-card">
      <div className="min-w-0">
        <div className="text-[12px] font-medium text-muted">{label}</div>
        <div className="mt-1 text-[22px] font-semibold leading-none tracking-tight tabular-nums text-ink">{value}</div>
        {sub && <div className="mt-1.5 truncate text-[12px] text-muted">{sub}</div>}
      </div>
      {trend && <Sparkline values={trend} />}
    </div>
  );
}

const kb = (v: number) => (v >= 1024 ? `${(v / 1024).toFixed(1)} MB/s` : `${Math.round(v)} KB/s`);

export default function ServerOverview() {
  const { user } = useAuth();
  const isRoot = user?.role === "root";
  const [range, setRange] = useState<Range>("24h");
  const status = useGet<ServerStatus>("/server/status", isRoot);
  const metrics = useGet<{ samples: HostSample[]; demo: boolean }>(`/server/metrics?range=${range}`, isRoot, () => 30_000);
  const accounts = useGet<Account[]>("/accounts");
  const top = useGet<TopAccount[]>("/server/top-accounts");
  const tickets = useGet<Ticket[]>("/tickets?status=open");

  if (isRoot && status.isLoading) return <Loading />;
  if (isRoot && status.error) return <ErrorNote error={status.error} />;
  const s = status.data;
  const list = accounts.data ?? [];
  const smp = metrics.data?.samples ?? [];
  const times = smp.map((x) => x.t);
  const last = smp[smp.length - 1];
  const waiting = tickets.data?.filter((t) => !t.mine && (t.status === "open" || t.status === "customer_reply")).length ?? 0;
  const suspended = list.filter((a) => a.status === "suspended").length;
  const diskUsed = list.reduce((n, a) => n + a.disk_used_mb, 0);

  return (
    <>
      <PageHeader
        title={isRoot && s ? s.hostname : "Overview"}
        description={isRoot && s ? `BreeferCP ${s.version} on ${s.os} · up ${formatUptime(s.uptime_s)} · ${s.cpus} CPU cores` : "Your customers at a glance."}
        actions={metrics.data?.demo ? <Badge tone="warn">Dry-run: demo metrics</Badge> : undefined}
      />

      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat label="Accounts" value={String(list.length)} sub={`${suspended} suspended`} />
        <Stat label="Open tickets" value={String(waiting)} sub={waiting ? "Waiting for a reply" : "All answered"} />
        {isRoot && s ? (
          <>
            <Stat label="CPU" value={`${last ? Math.round(last.cpu) : 0}%`} sub={`load ${last?.load.toFixed(2) ?? "0.00"}`} trend={smp.slice(-60).map((x) => x.cpu)} />
            <Stat label="Memory" value={`${last ? Math.round(last.mem) : 0}%`} sub={s.memory.total_mb ? `${formatMB(s.memory.used_mb)} of ${formatMB(s.memory.total_mb)}` : "of physical RAM"} trend={smp.slice(-60).map((x) => x.mem)} />
          </>
        ) : (
          <>
            <Stat label="Disk used by customers" value={formatMB(diskUsed)} />
            <Stat label="Active accounts" value={String(list.length - suspended)} />
          </>
        )}
      </div>

      {isRoot && (
        <>
          <div className="mb-3 flex items-center justify-between">
            <h2 className="text-[15px] font-semibold">Server health</h2>
            <Segmented
              label="Time range"
              options={[
                { id: "1h", label: "1 hour" },
                { id: "6h", label: "6 hours" },
                { id: "24h", label: "24 hours" },
              ]}
              value={range}
              onChange={setRange}
            />
          </div>
          <div className="mb-6 grid gap-4 lg:grid-cols-2">
            <Card title="CPU usage">
              <TimeChart label="Server CPU usage" times={times} series={[{ name: "CPU", color: "var(--series-1)", values: smp.map((x) => x.cpu) }]} format={(v) => `${Math.round(v)}%`} max={100} />
            </Card>
            <Card title="Memory usage">
              <TimeChart label="Server memory usage" times={times} series={[{ name: "Memory", color: "var(--series-1)", values: smp.map((x) => x.mem) }]} format={(v) => `${Math.round(v)}%`} max={100} />
            </Card>
            <Card title="Load average (1 min)">
              <TimeChart label="Server load average" times={times} series={[{ name: "Load", color: "var(--series-1)", values: smp.map((x) => x.load) }]} format={(v) => v.toFixed(1)} />
            </Card>
            <Card title="Network traffic">
              <TimeChart
                label="Network traffic in and out"
                times={times}
                series={[
                  { name: "In", color: "var(--series-1)", values: smp.map((x) => x.rx) },
                  { name: "Out", color: "var(--series-2)", values: smp.map((x) => x.tx) },
                ]}
                format={kb}
              />
            </Card>
          </div>
        </>
      )}

      <div className="grid gap-4 lg:grid-cols-[1.6fr_1fr]">
        <Card title="Top accounts by CPU" description="Average over the last hour" flush>
          {!top.data?.length ? (
            <p className="px-4 py-8 text-center text-[13px] text-muted">No usage samples yet.</p>
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Account</Th>
                  <Th className="text-right">CPU</Th>
                  <Th className="text-right">Memory</Th>
                  <Th className="w-40">Disk</Th>
                </tr>
              </thead>
              <tbody>
                {top.data.map((t) => (
                  <Tr key={t.id}>
                    <Td>
                      <Link to={`/accounts/${t.id}`} className="font-medium text-ink hover:text-accent">
                        {t.domain}
                      </Link>
                      <div className="text-[12px] text-muted">{t.user}</div>
                    </Td>
                    <Td className="text-right tabular-nums">{t.cpu}%</Td>
                    <Td className="text-right tabular-nums">{formatMB(t.mem_mb)}</Td>
                    <Td>
                      <div className="flex items-center gap-2">
                        <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-surface2">
                          <div className="h-full rounded-full bg-accent" style={{ width: `${percent(t.disk_used_mb, t.disk_limit_mb)}%` }} />
                        </div>
                        <span className="w-10 text-right text-[12px] tabular-nums text-muted">{t.disk_limit_mb ? `${percent(t.disk_used_mb, t.disk_limit_mb)}%` : "∞"}</span>
                      </div>
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>
        {isRoot && s ? (
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
              <div className="grid grid-cols-2 gap-3 border-t border-line pt-4 text-[12.5px]">
                <div>
                  <div className="text-muted">Database</div>
                  <div className="font-medium">{s.database === "sqlite" ? "SQLite (single node)" : "PostgreSQL (cluster)"}</div>
                </div>
                <div>
                  <div className="text-muted">Suspended</div>
                  <div className="font-medium">{s.accounts.suspended} accounts</div>
                </div>
              </div>
            </div>
          </Card>
        ) : (
          <Card title="Recent tickets" flush>
            <ul className="divide-y divide-line">
              {(tickets.data ?? []).slice(0, 6).map((t) => (
                <li key={t.id} className="px-4 py-2.5 text-[13px]">
                  <Link to={`/tickets?id=${t.id}`} className="font-medium text-ink hover:text-accent">
                    {t.subject}
                  </Link>
                  <div className="text-[12px] text-muted">{t.customer}</div>
                </li>
              ))}
              {!tickets.data?.length && <li className="px-4 py-8 text-center text-[13px] text-muted">No open tickets.</li>}
            </ul>
          </Card>
        )}
      </div>
    </>
  );
}
