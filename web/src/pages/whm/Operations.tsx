import { Archive, Globe, HardDrive, Megaphone, ShieldCheck } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import { formatMB, timeAgo } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { BackupConfig, QuotaRow, ZoneRow } from "@/lib/types";
import { useAuth } from "@/state/auth";
import { Button } from "@/components/ui/Button";
import { Checkbox, Field, Input, Select, Switch, Textarea } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, Mono, PageHeader, Segmented, Table, Td, Th, Tr } from "@/components/ui/Layout";

const days = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

// ---- Backup Configuration ---------------------------------------------------------------

export function BackupConfiguration() {
  const q = useGet<BackupConfig>("/server/backup-config");
  const [f, setF] = useState<BackupConfig | null>(null);
  useEffect(() => {
    if (q.data) setF(q.data);
  }, [q.data]);
  const save = useMutate(() => api.put("/server/backup-config", f), { invalidate: ["/server/backup-config"], success: "Backup schedule saved" });
  if (q.isLoading || !f) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  return (
    <>
      <PageHeader
        title="Backup Configuration"
        description="Back up every active account on a schedule. Scheduled backups appear in each customer's Backups page, where they can be restored."
      />
      <Card>
        <div className="max-w-xl space-y-5">
          <div className="flex items-center justify-between">
            <div>
              <div className="text-[13px] font-medium">Scheduled backups</div>
              <div className="text-[12.5px] text-muted">{f.last_run ? `Last run ${timeAgo(f.last_run)}` : "Never run"}</div>
            </div>
            <Switch checked={f.enabled} onChange={(v) => setF({ ...f, enabled: v })} label="Scheduled backups" />
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <Field label="Frequency">
              {(id) => (
                <Select id={id} value={f.frequency} onChange={(e) => setF({ ...f, frequency: e.target.value as BackupConfig["frequency"] })}>
                  <option value="daily">Daily</option>
                  <option value="weekly">Weekly</option>
                </Select>
              )}
            </Field>
            {f.frequency === "weekly" && (
              <Field label="Day">
                {(id) => (
                  <Select id={id} value={f.weekday} onChange={(e) => setF({ ...f, weekday: Number(e.target.value) })}>
                    {days.map((d, i) => (
                      <option key={d} value={i}>
                        {d}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
            )}
            <Field label="Start at (server time)">
              {(id) => (
                <Select id={id} value={f.hour} onChange={(e) => setF({ ...f, hour: Number(e.target.value) })}>
                  {Array.from({ length: 24 }, (_, h) => (
                    <option key={h} value={h}>
                      {String(h).padStart(2, "0")}:00
                    </option>
                  ))}
                </Select>
              )}
            </Field>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Contents">
              {(id) => (
                <Select id={id} value={f.kind} onChange={(e) => setF({ ...f, kind: e.target.value as BackupConfig["kind"] })}>
                  <option value="full">Files, email and databases</option>
                  <option value="files">Files and email only</option>
                  <option value="databases">Databases only</option>
                </Select>
              )}
            </Field>
            <Field label="Keep per account" hint="Older scheduled backups are deleted.">
              {(id) => <Input id={id} type="number" min={1} max={60} value={f.retention} onChange={(e) => setF({ ...f, retention: Number(e.target.value) })} />}
            </Field>
          </div>
          <p className="text-[12.5px] text-muted">
            Backups are stored on this server under the panel data directory. Copy that directory off-site (rsync, rclone or your provider's snapshots) to survive a disk failure.
          </p>
          <div className="flex justify-end">
            <Button variant="primary" icon={Archive} loading={save.isPending} onClick={() => save.mutate()}>
              Save schedule
            </Button>
          </div>
        </div>
      </Card>
    </>
  );
}

// ---- Email All Users / announcements --------------------------------------------------------

export function Broadcast() {
  const { user } = useAuth();
  const root = user?.role === "root";
  const [f, setF] = useState({ audience: "users", subject: "", body: "", email: true, announcement: false });
  const send = useMutate(() => api.post<{ sent: number }>("/server/broadcast", f), {
    success: (r) => (f.email ? `Sent to ${r.sent} recipient(s)` : "Announcement published"),
    onSuccess: () => setF({ ...f, subject: "", body: "" }),
  });
  return (
    <>
      <PageHeader
        title="Email All Users"
        description={root ? "Email your customers or resellers, and optionally show the subject as a notice at the top of every cPanel." : "Email all of your customers."}
      />
      <Card>
        <div className="max-w-2xl space-y-4">
          {root && (
            <Field label="Recipients">
              {() => (
                <Segmented
                  label="Recipients"
                  value={f.audience}
                  onChange={(v) => setF({ ...f, audience: v })}
                  options={[
                    { id: "users", label: "cPanel users" },
                    { id: "resellers", label: "Resellers" },
                    { id: "all", label: "Everyone" },
                  ]}
                />
              )}
            </Field>
          )}
          <Field label="Subject">{(id) => <Input id={id} value={f.subject} onChange={(e) => setF({ ...f, subject: e.target.value })} placeholder="Scheduled maintenance on Sunday 02:00 UTC" />}</Field>
          <Field label="Message">
            {(id) => <Textarea id={id} className="min-h-40 font-sans text-[13px]" value={f.body} onChange={(e) => setF({ ...f, body: e.target.value })} />}
          </Field>
          <div className="space-y-2">
            <label className="flex items-center gap-2 text-[13px]">
              <Checkbox checked={f.email} onChange={(e) => setF({ ...f, email: e.target.checked })} /> Send by email
            </label>
            {root && (
              <label className="flex items-center gap-2 text-[13px]">
                <Checkbox checked={f.announcement} onChange={(e) => setF({ ...f, announcement: e.target.checked })} /> Also show the subject as a cPanel announcement
              </label>
            )}
          </div>
          <div className="flex justify-end">
            <Button variant="primary" icon={Megaphone} loading={send.isPending} disabled={!f.subject || !f.body || (!f.email && !f.announcement)} onClick={() => send.mutate()}>
              Send
            </Button>
          </div>
        </div>
      </Card>
    </>
  );
}

// ---- DNS zones ---------------------------------------------------------------------------------

export function DnsZones() {
  const q = useGet<ZoneRow[]>("/server/dns-zones");
  const [filter, setFilter] = useState("");
  const list = (q.data ?? []).filter((z) => !filter || z.zone.includes(filter.toLowerCase()) || z.user.includes(filter.toLowerCase()));
  return (
    <>
      <PageHeader title="DNS Zones" description="Every zone hosted on this server's nameservers. Open a zone to edit its records or DNSSEC." />
      <div className="mb-4">
        <Input className="w-72" placeholder="Filter zones or users" value={filter} onChange={(e) => setFilter(e.target.value)} aria-label="Filter zones" />
      </div>
      <Card flush>
        {q.isLoading ? (
          <Loading />
        ) : !list.length ? (
          <EmptyState icon={Globe} title="No zones" />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Zone</Th>
                <Th>Account</Th>
                <Th>DNSSEC</Th>
                <Th className="w-24">
                  <span className="sr-only">Actions</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {list.map((z) => (
                <Tr key={z.zone}>
                  <Td>
                    <Mono className="text-ink">{z.zone}</Mono>
                  </Td>
                  <Td className="text-muted">{z.user}</Td>
                  <Td>
                    {z.dnssec ? (
                      <Badge tone="ok">
                        <ShieldCheck className="size-3" aria-hidden /> Signed
                      </Badge>
                    ) : (
                      <span className="text-muted">Off</span>
                    )}
                  </Td>
                  <Td>
                    <Link to={`/accounts/${z.account_id}/dns`}>
                      <Button size="sm">Edit zone</Button>
                    </Link>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
    </>
  );
}

// ---- Disk usage report -----------------------------------------------------------------------------

export function QuotaReport() {
  const q = useGet<QuotaRow[]>("/server/quota-report");
  const [only, setOnly] = useState<"all" | "over">("all");
  const rows = (q.data ?? []).filter((r) => only === "all" || r.over || r.percent >= 90);
  return (
    <>
      <PageHeader
        title="Disk Usage Report"
        description="Accounts by disk use against their package quota. Usage is measured hourly."
        actions={
          <Segmented
            label="Show"
            value={only}
            onChange={setOnly}
            options={[
              { id: "all", label: "All accounts" },
              { id: "over", label: "Near or over quota" },
            ]}
          />
        }
      />
      <Card flush>
        {q.isLoading ? (
          <Loading />
        ) : !rows.length ? (
          <EmptyState icon={HardDrive} title={only === "over" ? "No account is near its quota" : "No accounts"} />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Account</Th>
                <Th className="text-right">Disk used</Th>
                <Th className="text-right">Quota</Th>
                <Th className="w-48">Usage</Th>
                <Th className="text-right">Bandwidth</Th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <Tr key={r.id}>
                  <Td>
                    <Link to={`/accounts/${r.id}`} className="font-medium text-ink hover:text-accent">
                      {r.domain}
                    </Link>
                    <div className="text-[11.5px] text-subtle">
                      {r.user}
                      {r.status !== "active" && ` · ${r.status}`}
                    </div>
                  </Td>
                  <Td className="text-right tabular-nums">{formatMB(r.used_mb)}</Td>
                  <Td className="text-right tabular-nums text-muted">{r.limit_mb ? formatMB(r.limit_mb) : "Unlimited"}</Td>
                  <Td>
                    {r.limit_mb ? (
                      <div className="flex items-center gap-2">
                        <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-surface2">
                          <div className={`h-full rounded-full ${r.percent >= 90 ? "bg-bad" : "bg-accent"}`} style={{ width: `${Math.min(100, r.percent)}%` }} />
                        </div>
                        <span className="w-10 text-right text-[12px] tabular-nums text-muted">{Math.round(r.percent)}%</span>
                      </div>
                    ) : (
                      <span className="text-muted">—</span>
                    )}
                  </Td>
                  <Td className="text-right tabular-nums text-muted">{formatMB(r.bw_used_mb)}</Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
    </>
  );
}
