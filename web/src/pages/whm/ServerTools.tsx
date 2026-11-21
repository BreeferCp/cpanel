import { Mail, RefreshCw, Search, Send, Trash2, XOctagon } from "lucide-react";
import { useMemo, useState } from "react";
import { api } from "@/lib/api";
import { formatBytes, formatMB, formatUptime, percent, timeAgo } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { Proc, QueuedMail, ServerInfo as Info } from "@/lib/types";
import { Button, IconButton } from "@/components/ui/Button";
import { Input } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, Meter, Mono, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { ConfirmDialog } from "@/components/ui/Modal";

// ---- Server Information ----------------------------------------------------------

export function ServerInfo() {
  const q = useGet<Info>("/server/info");
  if (q.isLoading) return <Loading />;
  if (q.error || !q.data) return <ErrorNote error={q.error} />;
  const i = q.data;
  const rows: [string, string][] = [
    ["Operating system", i.os ?? "—"],
    ["Kernel", i.kernel ?? "—"],
    ["Processor", `${i.cpu ?? "—"}${i.cores ? ` · ${i.cores} cores` : ""}`],
    ["Uptime", i.uptime_s ? formatUptime(i.uptime_s) : "—"],
    ["Load average", i.load.map((l) => l.toFixed(2)).join("  ")],
    ["Control groups", i.cgroups ?? "—"],
    ["Web server", i.web_server === "openlitespeed" ? "OpenLiteSpeed" : "nginx"],
    ["Panel database", i.database === "postgres" ? "PostgreSQL" : "SQLite"],
    ["BreeferCP", i.panel_version],
  ];
  return (
    <>
      <PageHeader title="Server Information" description={i.demo ? "Sample values: this instance runs in dry-run mode." : "Hardware, operating system and storage of this server."} />
      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="System" flush>
          <dl className="divide-y divide-[var(--line)]">
            {rows.map(([k, v]) => (
              <div key={k} className="grid grid-cols-[160px_1fr] gap-3 px-4 py-2.5 text-[13px]">
                <dt className="text-muted">{k}</dt>
                <dd className="min-w-0 break-words text-ink">{v}</dd>
              </div>
            ))}
          </dl>
        </Card>
        <div className="space-y-4">
          <Card title="Memory">
            <Meter used={i.memory.used_mb} limit={i.memory.total_mb} format={formatMB} />
          </Card>
          <Card title="Storage" flush>
            <Table>
              <thead>
                <tr>
                  <Th>Mount</Th>
                  <Th>Used</Th>
                  <Th className="w-40">
                    <span className="sr-only">Usage</span>
                  </Th>
                </tr>
              </thead>
              <tbody>
                {i.disks.map((d) => (
                  <Tr key={d.mount}>
                    <Td>
                      <Mono className="text-ink">{d.mount}</Mono>
                      <div className="text-[11.5px] text-subtle">{d.fs}</div>
                    </Td>
                    <Td className="tabular-nums">
                      {formatMB(d.used_mb)} / {formatMB(d.total_mb)}
                    </Td>
                    <Td>
                      <div className="h-1.5 overflow-hidden rounded-full bg-surface2">
                        <div className={`h-full rounded-full ${percent(d.used_mb, d.total_mb) > 90 ? "bg-bad" : "bg-accent"}`} style={{ width: `${percent(d.used_mb, d.total_mb)}%` }} />
                      </div>
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          </Card>
        </div>
      </div>
    </>
  );
}

// ---- Process Manager ------------------------------------------------------------------

export function Processes() {
  const q = useGet<Proc[]>("/server/processes", true, () => 10_000);
  const [filter, setFilter] = useState("");
  const [kill, setKill] = useState<{ p: Proc; sig: "TERM" | "KILL" } | null>(null);
  const doKill = useMutate((v: { pid: number; sig: string }) => api.post(`/server/processes/${v.pid}/kill?signal=${v.sig}`), {
    invalidate: ["/server/processes"],
    success: "Signal sent",
    onSuccess: () => setKill(null),
  });
  const list = useMemo(() => {
    const s = filter.trim().toLowerCase();
    return (q.data ?? []).filter((p) => !s || p.user.toLowerCase().includes(s) || p.command.toLowerCase().includes(s) || String(p.pid) === s);
  }, [q.data, filter]);
  return (
    <>
      <PageHeader title="Process Manager" description="Running processes, largest memory use first. Refreshes every 10 seconds." />
      <div className="mb-4 flex items-center gap-2">
        <label className="relative w-72">
          <span className="sr-only">Filter processes</span>
          <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-subtle" aria-hidden />
          <Input className="pl-8" placeholder="Filter by user, command or PID" value={filter} onChange={(e) => setFilter(e.target.value)} />
        </label>
        <span className="text-[12.5px] text-muted">{list.length} processes</span>
      </div>
      <Card flush>
        {q.isLoading ? (
          <Loading />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>PID</Th>
                <Th>User</Th>
                <Th className="text-right">Memory</Th>
                <Th className="text-right">CPU time</Th>
                <Th>Command</Th>
                <Th className="w-20">
                  <span className="sr-only">Actions</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {list.map((p) => (
                <Tr key={p.pid}>
                  <Td className="tabular-nums text-muted">{p.pid}</Td>
                  <Td className="font-medium text-ink">{p.user}</Td>
                  <Td className="text-right tabular-nums">{p.mem_mb.toFixed(1)} MB</Td>
                  <Td className="text-right tabular-nums text-muted">{formatUptime(Math.round(p.cpu_seconds))}</Td>
                  <Td className="max-w-md">
                    <Mono className="block truncate text-muted" >{p.command}</Mono>
                  </Td>
                  <Td>
                    <div className="flex justify-end gap-1">
                      <IconButton icon={XOctagon} label={`Stop process ${p.pid}`} disabled={p.pid <= 1} onClick={() => setKill({ p, sig: "TERM" })} />
                    </div>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
      <ConfirmDialog
        open={!!kill}
        onClose={() => setKill(null)}
        onConfirm={() => kill && doKill.mutate({ pid: kill.p.pid, sig: kill.sig })}
        loading={doKill.isPending}
        confirmLabel={kill?.sig === "KILL" ? "Force kill" : "Stop"}
        title={`Stop process ${kill?.p.pid}?`}
        message={
          <div className="space-y-2">
            <Mono className="block break-all">{kill?.p.command}</Mono>
            <p>Sends SIGTERM so the process can exit cleanly.</p>
            {kill?.sig === "TERM" && (
              <button type="button" className="text-[12.5px] text-bad hover:underline" onClick={() => kill && setKill({ ...kill, sig: "KILL" })}>
                Use SIGKILL instead (cannot be ignored)
              </button>
            )}
          </div>
        }
      />
    </>
  );
}

// ---- Mail Queue Manager ---------------------------------------------------------------

export function MailQueue() {
  const q = useGet<QueuedMail[]>("/server/mail-queue");
  const [purge, setPurge] = useState(false);
  const act = useMutate((v: { action: string; id?: string }) => api.post(`/server/mail-queue/${v.action}`, { id: v.id ?? "" }), {
    invalidate: ["/server/mail-queue"],
    success: (_r, v) => (v.action === "flush" ? "Delivery retried for all messages" : v.action === "delete" ? "Deleted" : "Done"),
    onSuccess: () => setPurge(false),
  });
  const deferred = q.data?.filter((m) => m.queue === "deferred").length ?? 0;
  return (
    <>
      <PageHeader
        title="Mail Queue Manager"
        description="Messages waiting to be delivered. Deferred mail is retried automatically for up to five days."
        actions={
          <>
            <Button icon={Send} loading={act.isPending && act.variables?.action === "flush"} onClick={() => act.mutate({ action: "flush" })}>
              Retry all
            </Button>
            <Button variant="danger" icon={Trash2} disabled={!q.data?.length} onClick={() => setPurge(true)}>
              Delete all
            </Button>
          </>
        }
      />
      <Card flush>
        {q.isLoading ? (
          <Loading />
        ) : q.error ? (
          <div className="p-4">
            <ErrorNote error={q.error} />
          </div>
        ) : !q.data?.length ? (
          <EmptyState icon={Mail} title="The queue is empty" description="All mail has been delivered." />
        ) : (
          <>
            <div className="border-b border-line px-4 py-2 text-[12.5px] text-muted">
              {q.data.length} messages, {deferred} deferred
            </div>
            <Table>
              <thead>
                <tr>
                  <Th>Queued</Th>
                  <Th>From</Th>
                  <Th>To</Th>
                  <Th>Status</Th>
                  <Th className="w-20">
                    <span className="sr-only">Actions</span>
                  </Th>
                </tr>
              </thead>
              <tbody>
                {q.data.map((m) => (
                  <Tr key={m.id}>
                    <Td className="whitespace-nowrap text-muted">
                      {timeAgo(m.arrival)}
                      <div className="text-[11.5px] text-subtle">{formatBytes(m.size)}</div>
                    </Td>
                    <Td>
                      <Mono className="text-ink">{m.sender || "<>"}</Mono>
                    </Td>
                    <Td>
                      <Mono>{m.recipients?.join(", ")}</Mono>
                    </Td>
                    <Td className="max-w-sm">
                      <Badge tone={m.queue === "deferred" ? "warn" : m.queue === "hold" ? "neutral" : "accent"}>{m.queue}</Badge>
                      {m.reason && <div className="mt-1 line-clamp-2 text-[12px] text-muted">{m.reason}</div>}
                    </Td>
                    <Td>
                      <div className="flex justify-end gap-1">
                        <IconButton icon={RefreshCw} label={`Retry ${m.id}`} onClick={() => act.mutate({ action: "requeue", id: m.id })} />
                        <IconButton icon={Trash2} label={`Delete ${m.id}`} onClick={() => act.mutate({ action: "delete", id: m.id })} />
                      </div>
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          </>
        )}
      </Card>
      <ConfirmDialog
        open={purge}
        onClose={() => setPurge(false)}
        onConfirm={() => act.mutate({ action: "delete", id: "ALL" })}
        loading={act.isPending}
        title="Delete every queued message?"
        message="All messages in the queue are discarded and will not be delivered."
      />
    </>
  );
}
