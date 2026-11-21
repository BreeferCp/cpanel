import { useGet } from "@/lib/hooks";
import { formatMB } from "@/lib/format";
import type { DiskUsage as DU } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Card, ErrorNote, Loading, Meter, Mono, PageHeader } from "@/components/ui/Layout";

export default function DiskUsage() {
  const aid = useAccountId();
  const q = useGet<DU>(`/accounts/${aid}/disk-usage`);
  if (q.isLoading) return <Loading />;
  if (q.error || !q.data) return <ErrorNote error={q.error} />;
  const d = q.data;
  const rows = [...d.directories, ...(d.databases_mb ? [{ path: "MySQL databases", size_mb: d.databases_mb }] : [])].sort((a, b) => b.size_mb - a.size_mb);
  const total = rows.reduce((s, r) => s + r.size_mb, 0);
  const max = Math.max(1, ...rows.map((r) => r.size_mb));

  return (
    <>
      <PageHeader title="Disk Usage" description="What is using space in your account. Sizes are measured periodically." />
      <Card className="mb-5">
        <div className="mb-2 text-[12.5px] font-medium text-muted">Account total</div>
        <Meter used={Math.max(d.used_mb, total)} limit={d.limit_mb} format={formatMB} />
      </Card>
      <Card title="By folder" flush>
        <ul className="divide-y divide-line">
          {rows.map((r) => (
            <li key={r.path} className="grid grid-cols-[minmax(120px,200px)_1fr_80px] items-center gap-4 px-4 py-2.5 text-[13px]">
              <Mono className="truncate text-ink">{r.path}</Mono>
              <div className="h-2 overflow-hidden rounded-full bg-surface2" aria-hidden>
                <div className="h-full rounded-full bg-accent" style={{ width: `${Math.max(1, (r.size_mb / max) * 100)}%` }} />
              </div>
              <span className="text-right tabular-nums text-muted">{formatMB(r.size_mb)}</span>
            </li>
          ))}
        </ul>
      </Card>
    </>
  );
}
