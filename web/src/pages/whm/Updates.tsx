import { ArrowUpCircle, CheckCircle2, PackageCheck, RefreshCw, RotateCcw } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { timeAgo } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { UpdateStatus } from "@/lib/types";
import { Button } from "@/components/ui/Button";
import { Select } from "@/components/ui/Form";
import { Badge, Card, ErrorNote, Loading, Mono, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { ConfirmDialog } from "@/components/ui/Modal";

interface SystemUpdates {
  running: boolean;
  packages: { name: string; version: string }[];
  log: string;
}

export default function Updates() {
  const q = useGet<UpdateStatus>("/server/updates", true, (d) => (d?.running ? 3000 : false));
  const sys = useGet<SystemUpdates>("/server/system-updates", true, (d) => (d?.running ? 4000 : false));
  const [confirm, setConfirm] = useState<"apply" | "rollback" | "system" | null>(null);
  const check = useMutate(() => api.post("/server/updates/check"), { invalidate: ["/server/updates"], success: "Checked for updates" });
  const apply = useMutate(() => api.post("/server/updates/apply"), {
    invalidate: ["/server/updates"],
    success: "Installing. The panel restarts by itself when done.",
    onSuccess: () => setConfirm(null),
  });
  const rollback = useMutate(() => api.post("/server/updates/rollback"), { success: "Rolling back; the panel restarts in a few seconds", onSuccess: () => setConfirm(null) });
  const tier = useMutate((t: string) => api.put("/server/updates/config", { tier: t }), { invalidate: ["/server/updates"], success: "Update preference saved" });
  const runSys = useMutate(() => api.post("/server/system-updates"), {
    invalidate: ["/server/system-updates"],
    success: "Package update started",
    onSuccess: () => setConfirm(null),
  });

  if (q.isLoading) return <Loading />;
  if (q.error || !q.data) return <ErrorNote error={q.error} />;
  const u = q.data;

  return (
    <>
      <PageHeader title="Updates" description="Keep BreeferCP and the server's operating system packages current." />
      <div className="grid gap-4 lg:grid-cols-2">
        <Card
          title="BreeferCP"
          actions={
            <Button size="sm" icon={RefreshCw} loading={check.isPending} onClick={() => check.mutate()}>
              Check now
            </Button>
          }
        >
          <div className="flex flex-wrap items-center gap-3">
            <div>
              <div className="text-[12px] text-muted">Installed</div>
              <div className="text-[22px] font-semibold tabular-nums">{u.current}</div>
            </div>
            {u.latest && (
              <div className="ml-6">
                <div className="text-[12px] text-muted">Latest release</div>
                <div className="text-[22px] font-semibold tabular-nums">{u.latest.version}</div>
              </div>
            )}
            <div className="ml-auto">
              {u.running ? (
                <Badge tone="accent">Installing…</Badge>
              ) : u.available ? (
                <Badge tone="warn">
                  <ArrowUpCircle className="size-3" aria-hidden /> Update available
                </Badge>
              ) : u.latest ? (
                <Badge tone="ok">
                  <CheckCircle2 className="size-3" aria-hidden /> Up to date
                </Badge>
              ) : null}
            </div>
          </div>
          {u.check_error && <p className="mt-3 text-[12.5px] text-warn">Could not check: {u.check_error}</p>}
          {u.error && <p className="mt-3 text-[12.5px] text-bad">Last update failed: {u.error}</p>}
          {u.checked_at && <p className="mt-3 text-[12.5px] text-muted">Checked {timeAgo(u.checked_at)}.</p>}
          {u.available && u.latest?.notes && (
            <details className="mt-3 rounded-md border border-line bg-surface2 px-3 py-2">
              <summary className="cursor-pointer text-[13px] font-medium">Release notes</summary>
              <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap font-sans text-[12.5px] text-muted">{u.latest.notes}</pre>
            </details>
          )}
          <div className="mt-4 flex flex-wrap gap-2">
            <Button variant="primary" icon={ArrowUpCircle} disabled={!u.available || u.running} onClick={() => setConfirm("apply")}>
              Update to {u.latest?.version ?? "latest"}
            </Button>
            {u.can_rollback && (
              <Button icon={RotateCcw} onClick={() => setConfirm("rollback")}>
                Roll back
              </Button>
            )}
          </div>
          <div className="mt-5 border-t border-line pt-4">
            <label className="flex flex-wrap items-center justify-between gap-3 text-[13px]">
              <span>
                <span className="font-medium">Update preference</span>
                <span className="block text-[12.5px] text-muted">Automatic installs new releases between 03:00 and 05:00 server time.</span>
              </span>
              <Select className="w-44" value={u.tier} onChange={(e) => tier.mutate(e.target.value)} aria-label="Update preference">
                <option value="manual">Manual</option>
                <option value="automatic">Automatic</option>
              </Select>
            </label>
          </div>
          <p className="mt-4 text-[12px] text-subtle">
            Releases are downloaded from GitHub and installed only if they match the published SHA-256 checksums. The previous version is kept for rollback.
          </p>
        </Card>

        <Card
          title="Operating system packages"
          description="apt upgrade for the server, including the web, mail and database servers."
          actions={
            <Button size="sm" variant="primary" icon={PackageCheck} disabled={sys.data?.running || !sys.data?.packages.length} onClick={() => setConfirm("system")}>
              {sys.data?.running ? "Updating…" : "Upgrade packages"}
            </Button>
          }
          flush
        >
          {sys.isLoading ? (
            <Loading />
          ) : sys.data?.packages.length ? (
            <div className="max-h-80 overflow-auto">
              <Table>
                <thead>
                  <tr>
                    <Th>Package</Th>
                    <Th>New version</Th>
                  </tr>
                </thead>
                <tbody>
                  {sys.data.packages.map((p) => (
                    <Tr key={p.name}>
                      <Td>
                        <Mono className="text-ink">{p.name}</Mono>
                      </Td>
                      <Td>
                        <Mono className="text-muted">{p.version}</Mono>
                      </Td>
                    </Tr>
                  ))}
                </tbody>
              </Table>
            </div>
          ) : (
            <p className="p-4 text-[13px] text-muted">All packages are up to date.</p>
          )}
          {sys.data?.log && (
            <details className="border-t border-line px-4 py-3" open={sys.data.running}>
              <summary className="cursor-pointer text-[13px] font-medium">Output of the last run</summary>
              <pre className="mt-2 max-h-64 overflow-auto rounded bg-surface2 p-2 font-mono text-[11.5px] text-muted">{sys.data.log}</pre>
            </details>
          )}
        </Card>
      </div>

      <ConfirmDialog
        open={confirm === "apply"}
        onClose={() => setConfirm(null)}
        onConfirm={() => apply.mutate()}
        loading={apply.isPending}
        confirmLabel="Update"
        title={`Update to ${u.latest?.version}?`}
        message="The panel restarts after installing (about 10 seconds). Hosted sites, mail and databases keep running."
      />
      <ConfirmDialog
        open={confirm === "rollback"}
        onClose={() => setConfirm(null)}
        onConfirm={() => rollback.mutate()}
        loading={rollback.isPending}
        confirmLabel="Roll back"
        title="Restore the previous version?"
        message="The binary and web interface replaced by the last update are restored and the panel restarts."
      />
      <ConfirmDialog
        open={confirm === "system"}
        onClose={() => setConfirm(null)}
        onConfirm={() => runSys.mutate()}
        loading={runSys.isPending}
        confirmLabel="Upgrade"
        title="Upgrade operating system packages?"
        message="Services restart as their packages are upgraded. Kernel updates take effect after a reboot."
      />
    </>
  );
}
