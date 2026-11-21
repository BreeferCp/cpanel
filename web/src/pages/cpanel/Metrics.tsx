import { Activity, Bug, Download, FileText, RefreshCw, Users } from "lucide-react";
import { useState } from "react";
import { formatBytes, formatDate, formatMB, timeAgo } from "@/lib/format";
import { useGet } from "@/lib/hooks";
import type { AccountSample, Domain, Usage } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button } from "@/components/ui/Button";
import { TimeChart } from "@/components/ui/Chart";
import { DataTable } from "@/components/ui/DataTable";
import { Select } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, Meter, Mono, PageHeader, Segmented, Stat, Tabs } from "@/components/ui/Layout";

interface Hit {
  ip: string;
  time: number;
  method: string;
  path: string;
  status: number;
  bytes: number;
  referer: string;
  agent: string;
}
interface CountRow {
  key: string;
  count: number;
}
interface VisitorData {
  hits: Hit[];
  total: number;
  unique_ips: number;
  top_pages: CountRow[];
  top_ips: CountRow[];
  top_referrers: CountRow[];
  statuses: Record<string, number>;
}

/** Domain picker shared by the log-based pages. */
function useDomainPick() {
  const aid = useAccountId();
  const domains = useGet<Domain[]>(`/accounts/${aid}/domains`);
  const sites = domains.data?.filter((d) => d.kind !== "alias") ?? [];
  const [pick, setPick] = useState("");
  const name = pick || sites[0]?.name || "";
  const picker = (
    <Select aria-label="Domain" className="w-56" value={name} onChange={(e) => setPick(e.target.value)}>
      {sites.map((d) => (
        <option key={d.id}>{d.name}</option>
      ))}
    </Select>
  );
  return { aid, name, picker, loading: domains.isLoading, sites };
}

const statusTone = (s: number) => (s >= 500 ? "bad" : s >= 400 ? "warn" : s >= 300 ? "accent" : "ok");

function TopList({ title, rows, mono }: { title: string; rows: CountRow[]; mono?: boolean }) {
  const max = Math.max(1, ...rows.map((r) => r.count));
  return (
    <Card title={title}>
      {!rows.length ? (
        <p className="text-[13px] text-muted">No data yet.</p>
      ) : (
        <ul className="space-y-2">
          {rows.map((r) => (
            <li key={r.key} className="text-[12.5px]">
              <div className="flex justify-between gap-3">
                <span className={mono ? "truncate font-mono" : "truncate"} title={r.key}>
                  {r.key}
                </span>
                <span className="tabular-nums text-muted">{r.count}</span>
              </div>
              <div className="mt-1 h-1 rounded-full bg-surface2">
                <div className="h-1 rounded-full bg-[var(--series-1)]" style={{ width: `${(r.count / max) * 100}%` }} />
              </div>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

export function Visitors() {
  const { aid, name, picker } = useDomainPick();
  const q = useGet<VisitorData>(`/accounts/${aid}/visitors?domain=${encodeURIComponent(name)}&limit=1000`, !!name);
  const d = q.data;
  const errors = d ? (d.statuses["4xx"] ?? 0) + (d.statuses["5xx"] ?? 0) : 0;
  return (
    <>
      <PageHeader title="Visitors" actions={<>{picker}<Button icon={RefreshCw} onClick={() => void q.refetch()} loading={q.isFetching} aria-label="Refresh" /></>} />
      {d && (
        <div className="mb-4 grid grid-cols-2 gap-3 lg:grid-cols-4">
          <Stat label="Requests" value={d.total.toLocaleString()} hint="in the current log" />
          <Stat label="Unique visitors" value={d.unique_ips.toLocaleString()} hint="distinct IP addresses" />
          <Stat label="Successful" value={(d.statuses["2xx"] ?? 0).toLocaleString()} tone="ok" />
          <Stat label="Errors" value={errors.toLocaleString()} tone={errors ? "warn" : undefined} hint="4xx and 5xx responses" />
        </div>
      )}
      {d && (
        <div className="mb-4 grid gap-4 lg:grid-cols-3">
          <TopList title="Top pages" rows={d.top_pages} mono />
          <TopList title="Top visitors" rows={d.top_ips} mono />
          <TopList title="Top referrers" rows={d.top_referrers} />
        </div>
      )}
      <DataTable
        rows={d?.hits}
        loading={q.isLoading}
        error={q.error}
        rowKey={(h) => `${h.time}-${h.ip}-${h.path}-${h.bytes}`}
        search={(h) => `${h.ip} ${h.path} ${h.status} ${h.agent} ${h.referer}`}
        searchPlaceholder="Filter by IP, page, status or browser"
        empty={{ icon: Users, title: "No visits recorded", description: "Requests appear here as soon as your site gets traffic." }}
        initialSort={{ key: "time", dir: "desc" }}
        columns={[
          { key: "time", header: "Time", sort: (h) => h.time, render: (h) => <span title={formatDate(h.time)}>{timeAgo(h.time)}</span>, className: "whitespace-nowrap" },
          { key: "ip", header: "IP address", sort: (h) => h.ip, render: (h) => <Mono>{h.ip}</Mono> },
          { key: "path", header: "Request", sort: (h) => h.path, render: (h) => <span className="break-all font-mono text-[12px]"><span className="text-muted">{h.method}</span> {h.path}</span> },
          { key: "status", header: "Status", sort: (h) => h.status, render: (h) => <Badge tone={statusTone(h.status)}>{h.status}</Badge> },
          { key: "bytes", header: "Size", sort: (h) => h.bytes, render: (h) => <span className="tabular-nums">{formatBytes(h.bytes)}</span>, hideSm: true },
          { key: "agent", header: "Browser", render: (h) => <span className="line-clamp-1 max-w-64 text-[12px] text-muted" title={h.agent}>{h.agent}</span>, hideSm: true },
        ]}
      />
    </>
  );
}

export function Errors() {
  const { aid, name, picker } = useDomainPick();
  const [kind, setKind] = useState<"error" | "php">("error");
  const path = `/accounts/${aid}/logs?kind=${kind}&lines=500${kind === "php" ? "" : `&domain=${encodeURIComponent(name)}`}`;
  const q = useGet<{ lines: string[] }>(path, kind === "php" || !!name);
  const lines = [...(q.data?.lines ?? [])].reverse();
  return (
    <>
      <PageHeader
        title="Errors"
        actions={
          <>
            {kind === "error" && picker}
            <Button icon={RefreshCw} onClick={() => void q.refetch()} loading={q.isFetching}>
              Refresh
            </Button>
          </>
        }
      />
      <Tabs
        tabs={[
          { id: "error", label: "Web server errors" },
          { id: "php", label: "PHP errors" },
        ]}
        value={kind}
        onChange={setKind}
      />
      <Card flush>
        {q.isLoading ? (
          <Loading />
        ) : q.error ? (
          <div className="p-4">
            <ErrorNote error={q.error} />
          </div>
        ) : !lines.length ? (
          <EmptyState icon={Bug} title="No errors" description="Nothing has been logged. Newest entries appear at the top." />
        ) : (
          <ol className="max-h-[65vh] divide-y divide-line overflow-auto font-mono text-[12px]">
            {lines.map((l, i) => (
              <li key={i} className={`whitespace-pre-wrap break-all px-4 py-1.5 ${/\b(error|fatal|crit|emerg)\b/i.test(l) ? "text-bad" : /\bwarn/i.test(l) ? "text-warn" : "text-ink"}`}>
                {l}
              </li>
            ))}
          </ol>
        )}
      </Card>
      <p className="mt-2 text-[12px] text-muted">Showing the latest 500 lines, newest first.</p>
    </>
  );
}

interface BwDomain {
  domain: string;
  bytes: number;
  hits: number;
  days: { day: string; bytes: number; hits: number }[];
}

export function Bandwidth() {
  const aid = useAccountId();
  const q = useGet<{ domains: BwDomain[]; month_used_mb: number; limit_mb: number }>(`/accounts/${aid}/bandwidth`);
  if (q.isLoading) return <Loading />;
  if (q.error || !q.data) return <ErrorNote error={q.error} />;
  const doms = q.data.domains;
  const days = [...new Set(doms.flatMap((d) => d.days.map((x) => x.day)))].sort();
  const times = days.map((d) => Date.parse(d + "T12:00:00Z") / 1000);
  const top = [...doms].sort((a, b) => b.bytes - a.bytes).slice(0, 4);
  const total = doms.reduce((s, d) => s + d.bytes, 0);
  return (
    <>
      <PageHeader title="Bandwidth" />
      <div className="mb-4 grid gap-3 md:grid-cols-[1.2fr_1fr_1fr]">
        <Card title="This month">
          <Meter used={q.data.month_used_mb} limit={q.data.limit_mb} format={formatMB} />
        </Card>
        <Stat label="In the current logs" value={formatBytes(total)} hint={`${days.length} day(s)`} />
        <Stat label="Requests" value={doms.reduce((s, d) => s + d.hits, 0).toLocaleString()} />
      </div>
      {days.length > 1 && (
        <Card title="Data served per day" description={top.length < doms.length ? "The four busiest domains." : undefined} className="mb-4">
          <TimeChart
            label="Bytes served per day by domain"
            times={times}
            series={top.map((d, i) => ({ name: d.domain, color: `var(--series-${i + 1})`, values: days.map((day) => d.days.find((x) => x.day === day)?.bytes ?? 0) }))}
            format={formatBytes}
            height={200}
          />
        </Card>
      )}
      <DataTable
        rows={doms}
        rowKey={(d) => d.domain}
        empty={{ icon: Activity, title: "No traffic yet" }}
        initialSort={{ key: "bytes", dir: "desc" }}
        columns={[
          { key: "domain", header: "Domain", sort: (d) => d.domain, render: (d) => <span className="font-medium text-ink">{d.domain}</span> },
          { key: "bytes", header: "Data served", sort: (d) => d.bytes, render: (d) => <span className="tabular-nums">{formatBytes(d.bytes)}</span> },
          { key: "hits", header: "Requests", sort: (d) => d.hits, render: (d) => <span className="tabular-nums">{d.hits.toLocaleString()}</span> },
          { key: "share", header: "Share", render: (d) => <span className="tabular-nums text-muted">{total ? Math.round((d.bytes / total) * 100) : 0}%</span> },
        ]}
      />
      <p className="mt-2 text-[12px] text-muted">The monthly figure is what counts against your plan. Per-domain figures come from the access logs, which are rotated, so they cover the last days only.</p>
    </>
  );
}

export function RawAccess() {
  const aid = useAccountId();
  const domains = useGet<Domain[]>(`/accounts/${aid}/domains`);
  const sites = domains.data?.filter((d) => d.kind !== "alias") ?? [];
  const link = (kind: string, d?: string) => `/api/v1/accounts/${aid}/logs/download?kind=${kind}${d ? `&domain=${encodeURIComponent(d)}` : ""}`;
  return (
    <>
      <PageHeader title="Raw Access" />
      <DataTable
        rows={sites}
        loading={domains.isLoading}
        error={domains.error}
        rowKey={(d) => d.id}
        search={(d) => d.name}
        empty={{ icon: FileText, title: "No domains" }}
        columns={[
          { key: "name", header: "Domain", sort: (d) => d.name, render: (d) => <span className="font-medium text-ink">{d.name}</span> },
          { key: "kind", header: "Type", render: (d) => <span className="capitalize text-muted">{d.kind}</span> },
        ]}
        actions={(d) => (
          <>
            <a href={link("access", d.name)} download>
              <Button size="sm" icon={Download}>
                Access log
              </Button>
            </a>
            <a href={link("error", d.name)} download>
              <Button size="sm" variant="ghost" icon={Download}>
                Error log
              </Button>
            </a>
          </>
        )}
      />
      <Card title="PHP error log" className="mt-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-[13px] text-muted">Errors from every PHP version your domains use.</p>
          <a href={link("php")} download>
            <Button size="sm" icon={Download}>
              Download
            </Button>
          </a>
        </div>
      </Card>
      <p className="mt-3 text-[12px] text-muted">Logs use the combined format (IP, time, request, status, size, referrer, browser) and can be loaded into tools such as GoAccess or AWStats. Up to the latest 64 MB of each log is included.</p>
    </>
  );
}

export function ResourceUsage() {
  const aid = useAccountId();
  const [range, setRange] = useState<"1h" | "6h" | "24h" | "7d">("24h");
  const m = useGet<{ samples: AccountSample[]; limits: { cpu_pct: number; mem_mb: number }; demo: boolean }>(`/accounts/${aid}/metrics?range=${range}`);
  const usage = useGet<Usage>(`/accounts/${aid}/usage`);
  const s = m.data?.samples ?? [];
  const times = s.map((x) => x.t);
  const peak = (f: (x: AccountSample) => number) => (s.length ? Math.max(...s.map(f)) : 0);
  const avg = (f: (x: AccountSample) => number) => (s.length ? s.reduce((a, x) => a + f(x), 0) / s.length : 0);
  const lim = m.data?.limits;
  const label = { "1h": "last hour", "6h": "last 6 hours", "24h": "last 24 hours", "7d": "last 7 days" }[range];
  return (
    <>
      <PageHeader
        title="Resource Usage"
        actions={
          <Segmented
            label="Time range"
            value={range}
            onChange={setRange}
            options={[
              { id: "1h", label: "1h" },
              { id: "6h", label: "6h" },
              { id: "24h", label: "24h" },
              { id: "7d", label: "7d" },
            ]}
          />
        }
      />
      {m.isLoading ? (
        <Loading />
      ) : m.error ? (
        <ErrorNote error={m.error} />
      ) : (
        <>
          <div className="mb-4 grid grid-cols-2 gap-3 lg:grid-cols-4">
            <Stat label="CPU peak" value={`${Math.round(peak((x) => x.cpu))}%`} hint={`limit ${lim?.cpu_pct ?? 0}%`} tone={lim && peak((x) => x.cpu) >= lim.cpu_pct * 0.9 ? "warn" : undefined} />
            <Stat label="CPU average" value={`${Math.round(avg((x) => x.cpu))}%`} hint={label} />
            <Stat label="Memory peak" value={formatMB(peak((x) => x.mem_mb))} hint={`limit ${formatMB(lim?.mem_mb ?? 0)}`} tone={lim && peak((x) => x.mem_mb) >= lim.mem_mb * 0.9 ? "warn" : undefined} />
            <Stat label="Processes peak" value={peak((x) => x.procs)} hint={label} />
          </div>
          <div className="grid gap-4 xl:grid-cols-2">
            <Card title="CPU" description={`Share of one core · ${label}`}>
              <TimeChart label={`CPU usage, ${label}`} times={times} series={[{ name: "CPU", color: "var(--series-1)", values: s.map((x) => x.cpu) }]} format={(v) => `${Math.round(v)}%`} max={lim?.cpu_pct} height={170} />
            </Card>
            <Card title="Memory" description={label}>
              <TimeChart label={`Memory usage, ${label}`} times={times} series={[{ name: "Memory", color: "var(--series-1)", values: s.map((x) => x.mem_mb) }]} format={formatMB} max={lim?.mem_mb} height={170} />
            </Card>
            <Card title="Processes" description={label}>
              <TimeChart label={`Process count, ${label}`} times={times} series={[{ name: "Processes", color: "var(--series-1)", values: s.map((x) => x.procs) }]} format={(v) => String(Math.round(v))} height={170} />
            </Card>
            {usage.data && (
              <Card title="Plan limits">
                <div className="space-y-4">
                  <div>
                    <div className="mb-1 text-[12.5px] text-muted">Disk</div>
                    <Meter used={usage.data.disk.used} limit={usage.data.disk.limit} format={formatMB} />
                  </div>
                  <div>
                    <div className="mb-1 text-[12.5px] text-muted">Bandwidth this month</div>
                    <Meter used={usage.data.bandwidth.used} limit={usage.data.bandwidth.limit} format={formatMB} />
                  </div>
                  <p className="text-[12px] text-muted">When the account reaches its CPU or memory limit, processes are slowed down rather than stopped. Pages may load more slowly until usage drops.</p>
                </div>
              </Card>
            )}
          </div>
          {m.data?.demo && <p className="mt-3 text-[12px] text-subtle">Dry-run mode: charts show demo data.</p>}
        </>
      )}
    </>
  );
}
