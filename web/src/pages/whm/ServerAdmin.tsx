import { AlarmClock, CheckCircle2, CircleAlert, CircleX, GlobeLock, Info, KeyRound, LockKeyhole, Plus, Power, RefreshCw, ShieldAlert, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import { Button, IconButton } from "@/components/ui/Button";
import { DataTable } from "@/components/ui/DataTable";
import { Field, Input, Select, Switch, Textarea } from "@/components/ui/Form";
import { Badge, Card, ErrorNote, HelpNote, Loading, Mono, PageHeader, Stat } from "@/components/ui/Layout";
import { ConfirmDialog } from "@/components/ui/Modal";
import { sslBadge, type ServerDomain } from "./AccountFunctions";

// ---- Security Advisor ----------------------------------------------------------------

interface Advice {
  id: string;
  status: "ok" | "warn" | "bad" | "info";
  title: string;
  detail: string;
  fix: string;
}

const adviceIcon = {
  bad: <CircleX className="size-4 text-bad" aria-label="Critical" />,
  warn: <CircleAlert className="size-4 text-warn" aria-label="Warning" />,
  info: <Info className="size-4 text-accent" aria-label="Information" />,
  ok: <CheckCircle2 className="size-4 text-ok" aria-label="OK" />,
};

export function SecurityAdvisor() {
  const q = useGet<Advice[]>("/server/security-advisor");
  const list = q.data ?? [];
  const count = (s: Advice["status"]) => list.filter((a) => a.status === s).length;
  return (
    <>
      <PageHeader
        title="Security Advisor"
        actions={
          <Button icon={RefreshCw} loading={q.isFetching} onClick={() => void q.refetch()}>
            Scan again
          </Button>
        }
      />
      {q.isLoading ? (
        <Loading />
      ) : q.error ? (
        <ErrorNote error={q.error} />
      ) : (
        <>
          <div className="mb-4 grid grid-cols-2 gap-3 lg:grid-cols-4">
            <Stat label="Critical" value={count("bad")} tone={count("bad") ? "bad" : undefined} />
            <Stat label="Warnings" value={count("warn")} tone={count("warn") ? "warn" : undefined} />
            <Stat label="Advice" value={count("info")} />
            <Stat label="Passed" value={count("ok")} tone="ok" />
          </div>
          <Card flush>
            <ul className="divide-y divide-line">
              {list.map((a) => (
                <li key={a.id} className="flex items-start gap-3 px-4 py-3">
                  <span className="mt-0.5">{adviceIcon[a.status]}</span>
                  <div className="min-w-0 flex-1 text-[13px]">
                    <div className="font-medium text-ink">{a.title}</div>
                    {a.detail && <div className="mt-0.5 text-muted">{a.detail}</div>}
                  </div>
                  {a.fix && a.status !== "ok" && (
                    <Link to={a.fix}>
                      <Button size="sm">Fix</Button>
                    </Link>
                  )}
                </li>
              ))}
            </ul>
          </Card>
        </>
      )}
    </>
  );
}

// ---- SSH settings ------------------------------------------------------------------

interface SshConf {
  password_auth: boolean;
  root_login: "yes" | "prohibit-password" | "no";
  port: number;
}

export function SshSettings() {
  const q = useGet<SshConf>("/server/ssh");
  const [f, setF] = useState<SshConf | null>(null);
  useEffect(() => {
    if (q.data) setF(q.data);
  }, [q.data]);
  const save = useMutate(() => api.put("/server/ssh", f), { invalidate: ["/server/ssh", "/server/security-advisor"], success: "SSH settings applied" });
  if (q.isLoading || !f) return <Loading />;
  return (
    <>
      <PageHeader title="SSH Settings" />
      <HelpNote>
        <p>Changes are checked with <Mono>sshd -t</Mono> before SSH is reloaded, and rolled back if they are invalid. Open sessions stay connected.</p>
        <p>Keep a second SSH session open while you change these, so you can undo a mistake.</p>
      </HelpNote>
      <Card className="max-w-2xl">
        <div className="space-y-5">
          <div className="flex items-start justify-between gap-4">
            <div className="text-[13px]">
              <div className="font-medium text-ink">Password authentication</div>
              <div className="text-muted">Off means only SSH keys work. Add a root key first.</div>
            </div>
            <Switch label="Password authentication" checked={f.password_auth} onChange={(v) => setF({ ...f, password_auth: v })} />
          </div>
          <Field label="Root login" hint="“Keys only” is the safe default.">
            {(id) => (
              <Select id={id} value={f.root_login} onChange={(e) => setF({ ...f, root_login: e.target.value as SshConf["root_login"] })}>
                <option value="prohibit-password">Keys only</option>
                <option value="no">Not allowed</option>
                <option value="yes">Password or key (not recommended)</option>
              </Select>
            )}
          </Field>
          <Field label="Port" hint="22, or 1024–65535. A new port is opened in the firewall automatically.">
            {(id) => <Input id={id} type="number" className="w-32" value={f.port} onChange={(e) => setF({ ...f, port: Number(e.target.value) })} />}
          </Field>
          <Button variant="primary" loading={save.isPending} onClick={() => save.mutate()}>
            Save
          </Button>
        </div>
      </Card>
    </>
  );
}

// ---- Root SSH keys ----------------------------------------------------------------

interface RootKey {
  fingerprint: string;
  type: string;
  comment: string;
  managed: boolean;
}

export function RootKeys() {
  const q = useGet<RootKey[]>("/server/root-keys");
  const [key, setKey] = useState("");
  const [del, setDel] = useState<RootKey | null>(null);
  const add = useMutate(() => api.post("/server/root-keys", { public_key: key }), { invalidate: ["/server/root-keys"], success: "Key authorized", onSuccess: () => setKey("") });
  const remove = useMutate(() => api.del(`/server/root-keys/${encodeURIComponent(del!.fingerprint)}`), { invalidate: ["/server/root-keys"], success: "Key removed", onSuccess: () => setDel(null) });
  return (
    <>
      <PageHeader title="Manage root's SSH Keys" />
      <Card title="Authorize a key" className="mb-4">
        <form className="space-y-3" onSubmit={(e) => (e.preventDefault(), add.mutate())}>
          <Textarea aria-label="Public key" rows={3} value={key} onChange={(e) => setKey(e.target.value)} placeholder="ssh-ed25519 AAAA… you@laptop" />
          <div className="flex items-center gap-3">
            <Button type="submit" variant="primary" icon={Plus} loading={add.isPending} disabled={!key.trim()}>
              Authorize
            </Button>
            <span className="text-[12px] text-muted">Paste the contents of your id_ed25519.pub. Key options such as command= are refused.</span>
          </div>
        </form>
      </Card>
      <DataTable
        rows={q.data}
        loading={q.isLoading}
        error={q.error}
        rowKey={(k) => k.fingerprint}
        empty={{ icon: KeyRound, title: "No keys", description: "Root can log in only with a password." }}
        columns={[
          { key: "comment", header: "Key", render: (k) => <div><div className="font-medium text-ink">{k.comment || "(no comment)"}</div><Mono className="text-muted">{k.fingerprint}</Mono></div> },
          { key: "type", header: "Type", render: (k) => <Mono>{k.type}</Mono> },
          { key: "managed", header: "Added by", render: (k) => (k.managed ? <Badge tone="accent">BreeferCP</Badge> : <Badge>authorized_keys</Badge>) },
        ]}
        actions={(k) => <IconButton icon={Trash2} label={`Remove ${k.comment || k.fingerprint}`} onClick={() => setDel(k)} />}
      />
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={() => remove.mutate()} loading={remove.isPending} title="Remove this key?" message={<>Anyone using <Mono>{del?.fingerprint}</Mono> can no longer log in as root.</>} confirmLabel="Remove" />
    </>
  );
}

// ---- Root password ----------------------------------------------------------------

export function RootPassword() {
  const [f, setF] = useState({ current_password: "", password: "", confirm: "" });
  const save = useMutate(() => api.post("/server/root-password", { current_password: f.current_password, password: f.password }), { success: "Root password changed", onSuccess: () => setF({ current_password: "", password: "", confirm: "" }) });
  return (
    <>
      <PageHeader title="Change Root Password" />
      <Card className="max-w-lg">
        <form className="space-y-4" onSubmit={(e) => (e.preventDefault(), save.mutate())}>
          <p className="flex gap-2 text-[13px] text-muted">
            <LockKeyhole className="mt-0.5 size-4 shrink-0 text-subtle" aria-hidden />
            This is the operating system's root password (console and SSH), not your panel password.
          </p>
          <Field label="Your panel password" hint="Confirms it is really you.">{(id) => <Input id={id} type="password" value={f.current_password} onChange={(e) => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" />}</Field>
          <Field label="New root password" hint="At least 12 characters.">{(id) => <Input id={id} type="password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} autoComplete="new-password" />}</Field>
          <Field label="Repeat" error={f.confirm && f.confirm !== f.password ? "The passwords do not match" : undefined}>{(id) => <Input id={id} type="password" value={f.confirm} onChange={(e) => setF({ ...f, confirm: e.target.value })} autoComplete="new-password" />}</Field>
          <Button type="submit" variant="primary" loading={save.isPending} disabled={!f.current_password || f.password.length < 12 || f.password !== f.confirm}>
            Change password
          </Button>
        </form>
      </Card>
    </>
  );
}

// ---- Server time -----------------------------------------------------------------

export function ServerTime() {
  const q = useGet<{ timezone: string; now: string; ntp_synced: boolean; timezones: string[] }>("/server/time");
  const [tz, setTz] = useState("");
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, []);
  useEffect(() => {
    if (q.data) setTz(q.data.timezone);
  }, [q.data]);
  const save = useMutate((sync: boolean) => api.put("/server/time", { timezone: tz, sync_now: sync }), { invalidate: ["/server/time"], success: "Server time updated" });
  if (q.isLoading || !q.data) return q.error ? <ErrorNote error={q.error} /> : <Loading />;
  let clock = "";
  try {
    clock = new Date(now).toLocaleString(undefined, { timeZone: q.data.timezone, dateStyle: "full", timeStyle: "medium" });
  } catch {
    clock = new Date(now).toLocaleString();
  }
  return (
    <>
      <PageHeader title="Server Time" />
      <div className="grid max-w-3xl gap-4 md:grid-cols-2">
        <Card title="Current time">
          <div className="flex items-start gap-3">
            <AlarmClock className="mt-1 size-5 text-subtle" aria-hidden />
            <div>
              <div className="text-[16px] font-semibold tabular-nums text-ink">{clock}</div>
              <div className="mt-1 text-[13px] text-muted">{q.data.timezone}</div>
              <div className="mt-2">{q.data.ntp_synced ? <Badge tone="ok">Synchronised with NTP</Badge> : <Badge tone="warn">Not synchronised</Badge>}</div>
            </div>
          </div>
        </Card>
        <Card title="Time zone" description="Used for logs, cron jobs and scheduled backups.">
          <div className="space-y-3">
            <Select aria-label="Time zone" value={tz} onChange={(e) => setTz(e.target.value)}>
              {q.data.timezones.map((t) => (
                <option key={t}>{t}</option>
              ))}
            </Select>
            <div className="flex gap-2">
              <Button variant="primary" loading={save.isPending && save.variables === false} disabled={tz === q.data.timezone} onClick={() => save.mutate(false)}>
                Change time zone
              </Button>
              <Button loading={save.isPending && save.variables === true} onClick={() => save.mutate(true)}>
                Sync clock now
              </Button>
            </div>
          </div>
        </Card>
      </div>
    </>
  );
}

// ---- AutoSSL ------------------------------------------------------------------------

export function AutoSsl() {
  const q = useGet<ServerDomain[]>("/server/domains");
  const run = useMutate(() => api.post<{ queued: number; message: string }>("/server/autossl"), { invalidate: ["/server/domains"], success: (r) => `${r.message} ${r.queued} domain(s) without a certificate.` });
  const now = Date.now() / 1000;
  const rows = q.data?.filter((d) => d.status === "active");
  const none = rows?.filter((d) => !d.ssl_not_after).length ?? 0;
  const soon = rows?.filter((d) => d.ssl_not_after && d.ssl_not_after - now < 14 * 86400).length ?? 0;
  return (
    <>
      <PageHeader
        title="Manage AutoSSL"
        actions={
          <Button variant="primary" icon={GlobeLock} loading={run.isPending} onClick={() => run.mutate()}>
            Run AutoSSL for all users
          </Button>
        }
      />
      <HelpNote>
        <p>AutoSSL requests free Let's Encrypt certificates for domains that have none, and renews certificates 30 days before they expire. It runs on its own twice a day.</p>
        <p>A domain gets a certificate only when its DNS already points at this server. Domains that fail are retried at the next run.</p>
      </HelpNote>
      <div className="mb-4 grid grid-cols-3 gap-3">
        <Stat label="Domains" value={rows?.length ?? 0} />
        <Stat label="Without certificate" value={none} tone={none ? "warn" : "ok"} />
        <Stat label="Expiring within 14 days" value={soon} tone={soon ? "bad" : undefined} />
      </div>
      <DataTable
        rows={rows}
        loading={q.isLoading}
        error={q.error}
        rowKey={(d) => d.id}
        search={(d) => `${d.name} ${d.user} ${d.ssl_issuer}`}
        searchPlaceholder="Search domains"
        empty={{ icon: GlobeLock, title: "No domains" }}
        pageSize={50}
        initialSort={{ key: "ssl", dir: "asc" }}
        columns={[
          { key: "name", header: "Domain", sort: (d) => d.name, render: (d) => <span className="font-medium text-ink">{d.name}</span> },
          { key: "user", header: "Account", sort: (d) => d.user, render: (d) => <span className="font-mono text-[12.5px]">{d.user}</span> },
          { key: "issuer", header: "Issuer", sort: (d) => d.ssl_issuer, render: (d) => <span className="text-muted">{d.ssl_issuer === "letsencrypt" ? "Let's Encrypt" : d.ssl_issuer || "—"}</span> },
          { key: "ssl", header: "Status", sort: (d) => d.ssl_not_after || 0, render: sslBadge },
          { key: "exp", header: "Expires", sort: (d) => d.ssl_not_after, render: (d) => <span className="text-muted">{d.ssl_not_after ? new Date(d.ssl_not_after * 1000).toLocaleDateString() : "—"}</span>, hideSm: true },
        ]}
      />
    </>
  );
}

// ---- Reboot --------------------------------------------------------------------------

export function Reboot() {
  const [mode, setMode] = useState<"graceful" | "force" | null>(null);
  const run = useMutate((m: string) => api.post<{ dry_run: boolean }>("/server/reboot", { mode: m }), {
    success: (r) => (r.dry_run ? "Dry-run: the server was not rebooted." : "The server reboots in 5 seconds. The panel is back in a minute or two."),
    onSuccess: () => setMode(null),
  });
  return (
    <>
      <PageHeader title="Reboot Server" />
      <div className="grid max-w-3xl gap-4 md:grid-cols-2">
        <Card title="Graceful reboot" description="Stops every service cleanly, then restarts. Use this normally, for example after a kernel update.">
          <Button icon={Power} onClick={() => setMode("graceful")}>
            Graceful reboot
          </Button>
        </Card>
        <Card title="Forceful reboot" description="Restarts immediately without stopping services. Only when a graceful reboot hangs; unsaved data can be lost.">
          <Button variant="danger" icon={ShieldAlert} onClick={() => setMode("force")}>
            Forceful reboot
          </Button>
        </Card>
      </div>
      <ConfirmDialog
        open={!!mode}
        onClose={() => setMode(null)}
        onConfirm={() => mode && run.mutate(mode)}
        loading={run.isPending}
        title={mode === "force" ? "Force a reboot now?" : "Reboot the server?"}
        message="All websites, email and the panel are offline until the server is back up."
        confirmLabel="Reboot"
      />
    </>
  );
}
