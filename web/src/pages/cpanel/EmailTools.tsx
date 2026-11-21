import {
  ArrowDown,
  ArrowRight,
  ArrowUp,
  CheckCircle2,
  CircleAlert,
  CircleX,
  ExternalLink,
  FlaskConical,
  Forward,
  Inbox,
  KeyRound,
  Mail,
  MailSearch,
  Pause,
  Pencil,
  Play,
  Plus,
  Route,
  Smartphone,
  Trash2,
  UsersRound,
  Wrench,
} from "lucide-react";
import { useMemo, useState } from "react";
import { api } from "@/lib/api";
import { formatDate, formatMB, timeAgo } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { Domain, MailAlias, Mailbox } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button, IconButton } from "@/components/ui/Button";
import { CopyField } from "@/components/ui/CopyField";
import { DataTable } from "@/components/ui/DataTable";
import { Field, Input, Select, Textarea } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, HelpNote, Loading, Meter, Mono, PageHeader, Stat, Tabs } from "@/components/ui/Layout";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

function useMailDomains() {
  const aid = useAccountId();
  const q = useGet<Domain[]>(`/accounts/${aid}/domains`);
  return { ...q, mail: q.data?.filter((d) => d.kind === "primary" || d.kind === "addon") ?? [] };
}

function AddressInput({ local, setLocal, domain, setDomain, domains, placeholder }: { local: string; setLocal: (v: string) => void; domain: string; setDomain: (v: string) => void; domains: Domain[]; placeholder?: string }) {
  return (
    <div className="grid grid-cols-[1fr_auto_1fr] items-end gap-2">
      <Field label="Address">{(id) => <Input id={id} value={local} onChange={(e) => setLocal(e.target.value)} placeholder={placeholder} autoFocus />}</Field>
      <span className="pb-1.5 text-muted">@</span>
      <Field label="Domain">
        {(id) => (
          <Select id={id} value={domain} onChange={(e) => setDomain(e.target.value)}>
            <option value="">Select…</option>
            {domains.map((d) => (
              <option key={d.id} value={d.id}>
                {d.name}
              </option>
            ))}
          </Select>
        )}
      </Field>
    </div>
  );
}

// ---- Email Accounts -----------------------------------------------------------

export function EmailAccounts() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/mailboxes`;
  const boxes = useGet<Mailbox[]>(key);
  const { mail } = useMailDomains();
  const [create, setCreate] = useState(false);
  const [f, setF] = useState({ local: "", domain: "", password: "", quota: "1024" });
  const [pw, setPw] = useState<Mailbox | null>(null);
  const [quota, setQuota] = useState<{ box: Mailbox; value: string } | null>(null);
  const [connect, setConnect] = useState<Mailbox | null>(null);
  const [del, setDel] = useState<Mailbox[] | null>(null);
  const [newPw, setNewPw] = useState("");

  const createBox = useMutate(() => api.post(key, { local_part: f.local, domain_id: f.domain, password: f.password, quota_mb: Number(f.quota) }), {
    invalidate: [key],
    success: "Email account created",
    onSuccess: () => {
      setCreate(false);
      setF({ local: "", domain: "", password: "", quota: "1024" });
    },
  });
  const changePw = useMutate(() => api.put(`${key}/${pw!.id}/password`, { password: newPw }), { success: "Password changed", onSuccess: () => (setPw(null), setNewPw("")) });
  const patch = useMutate((v: { id: string; body: Record<string, unknown> }) => api.patch(`${key}/${v.id}`, v.body), { invalidate: [key], success: "Saved", onSuccess: () => setQuota(null) });
  const remove = useMutate(async (list: Mailbox[]) => {
    for (const m of list) await api.del(`${key}/${m.id}`);
  }, { invalidate: [key], success: "Deleted", onSuccess: () => setDel(null) });

  const list = boxes.data ?? [];
  const used = list.reduce((s, m) => s + m.used_mb, 0);
  const full = list.filter((m) => m.quota_mb && m.used_mb / m.quota_mb >= 0.9).length;

  return (
    <>
      <PageHeader
        title="Email Accounts"
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setCreate(true)}>
            Create
          </Button>
        }
      />
      <div className="mb-4 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat label="Accounts" value={list.length} />
        <Stat label="Storage used" value={formatMB(used)} />
        <Stat label="Nearly full" value={full} tone={full ? "warn" : undefined} hint="90% of quota or more" />
        <Stat label="Suspended" value={list.filter((m) => !m.active).length} />
      </div>
      <DataTable
        rows={boxes.data}
        loading={boxes.isLoading}
        error={boxes.error}
        rowKey={(m) => m.id}
        search={(m) => m.address}
        searchPlaceholder="Search email accounts"
        empty={{ icon: Mail, title: "No email accounts", description: "Create an address such as info@yourdomain.com.", action: <Button icon={Plus} onClick={() => setCreate(true)}>Create</Button> }}
        bulk={[
          { label: "Suspend", icon: Pause, onRun: (rows) => rows.forEach((m) => patch.mutate({ id: m.id, body: { active: false } })) },
          { label: "Unsuspend", icon: Play, onRun: (rows) => rows.forEach((m) => patch.mutate({ id: m.id, body: { active: true } })) },
          { label: "Delete", icon: Trash2, danger: true, onRun: setDel },
        ]}
        initialSort={{ key: "address", dir: "asc" }}
        columns={[
          { key: "address", header: "Account", sort: (m) => m.address, render: (m) => <span className="font-medium text-ink">{m.address}</span> },
          { key: "storage", header: "Storage", sort: (m) => m.used_mb, className: "w-60", render: (m) => <Meter used={m.used_mb} limit={m.quota_mb} format={formatMB} /> },
          { key: "status", header: "Status", sort: (m) => (m.active ? 1 : 0), render: (m) => <Badge tone={m.active ? "ok" : "warn"}>{m.active ? "Active" : "Suspended"}</Badge> },
        ]}
        actions={(m) => (
          <>
            <IconButton icon={Smartphone} label={`Connect devices for ${m.address}`} onClick={() => setConnect(m)} />
            <IconButton icon={KeyRound} label={`Change password for ${m.address}`} onClick={() => setPw(m)} />
            <IconButton icon={Pencil} label={`Change quota for ${m.address}`} onClick={() => setQuota({ box: m, value: String(m.quota_mb) })} />
            <IconButton icon={m.active ? Pause : Play} label={m.active ? `Suspend ${m.address}` : `Unsuspend ${m.address}`} onClick={() => patch.mutate({ id: m.id, body: { active: !m.active } })} />
            <IconButton icon={Trash2} label={`Delete ${m.address}`} onClick={() => setDel([m])} />
          </>
        )}
      />

      <Modal
        open={create}
        onClose={() => setCreate(false)}
        title="Create an email account"
        footer={
          <>
            <Button onClick={() => setCreate(false)}>Cancel</Button>
            <Button variant="primary" loading={createBox.isPending} disabled={!f.local || !f.domain || f.password.length < 12} onClick={() => createBox.mutate()}>
              Create
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <AddressInput local={f.local} setLocal={(v) => setF({ ...f, local: v })} domain={f.domain} setDomain={(v) => setF({ ...f, domain: v })} domains={mail} />
          <Field label="Password" hint="At least 12 characters with a mix of letters, digits and symbols.">
            {(id) => <Input id={id} type="password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} autoComplete="new-password" />}
          </Field>
          <Field label="Storage space (MB)">{(id) => <Input id={id} type="number" min={1} value={f.quota} onChange={(e) => setF({ ...f, quota: e.target.value })} />}</Field>
        </div>
      </Modal>

      <Modal
        open={!!pw}
        onClose={() => setPw(null)}
        title={`Password for ${pw?.address}`}
        footer={
          <>
            <Button onClick={() => setPw(null)}>Cancel</Button>
            <Button variant="primary" loading={changePw.isPending} disabled={newPw.length < 12} onClick={() => changePw.mutate()}>
              Change password
            </Button>
          </>
        }
      >
        <Field label="New password" hint="Mail apps signed in with the old password will ask for the new one.">
          {(id) => <Input id={id} type="password" value={newPw} onChange={(e) => setNewPw(e.target.value)} autoComplete="new-password" autoFocus />}
        </Field>
      </Modal>

      <Modal
        open={!!quota}
        onClose={() => setQuota(null)}
        title={`Storage for ${quota?.box.address}`}
        footer={
          <>
            <Button onClick={() => setQuota(null)}>Cancel</Button>
            <Button variant="primary" loading={patch.isPending} onClick={() => quota && patch.mutate({ id: quota.box.id, body: { quota_mb: Number(quota.value) } })}>
              Save
            </Button>
          </>
        }
      >
        <Field label="Quota (MB)" hint={quota ? `Currently using ${formatMB(quota.box.used_mb)}.` : undefined}>
          {(id) => <Input id={id} type="number" min={1} value={quota?.value ?? ""} onChange={(e) => quota && setQuota({ ...quota, value: e.target.value })} autoFocus />}
        </Field>
      </Modal>

      <Modal open={!!connect} onClose={() => setConnect(null)} title="Connect devices" description={connect?.address} width="max-w-lg">
        {connect && <ClientSettings address={connect.address} />}
      </Modal>

      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        onConfirm={() => del && remove.mutate(del)}
        loading={remove.isPending}
        title={del?.length === 1 ? `Delete ${del[0]!.address}?` : `Delete ${del?.length} email accounts?`}
        message="All mail stored in these accounts is deleted permanently. Forwarders pointing at them stop working."
      />
    </>
  );
}

function ClientSettings({ address }: { address: string }) {
  const domain = address.split("@")[1] ?? "your-domain";
  const host = `mail.${domain}`;
  return (
    <div className="space-y-4 text-[13px]">
      <p className="text-muted">Use these settings in Outlook, Apple Mail, Thunderbird or the mail app on your phone. Most apps find them automatically when you enter the address and password.</p>
      <dl className="grid grid-cols-[auto_1fr] items-center gap-x-6 gap-y-2">
        <dt className="text-muted">Username</dt>
        <dd>
          <CopyField value={address} />
        </dd>
        <dt className="text-muted">Incoming server</dt>
        <dd>
          <CopyField value={host} />
        </dd>
        <dt className="text-muted">IMAP port</dt>
        <dd className="font-mono">993 (SSL/TLS)</dd>
        <dt className="text-muted">POP3 port</dt>
        <dd className="font-mono">995 (SSL/TLS)</dd>
        <dt className="text-muted">Outgoing server</dt>
        <dd>
          <CopyField value={host} />
        </dd>
        <dt className="text-muted">SMTP port</dt>
        <dd className="font-mono">465 (SSL/TLS) or 587 (STARTTLS)</dd>
        <dt className="text-muted">Authentication</dt>
        <dd>Required for incoming and outgoing mail, with the account password</dd>
      </dl>
      <p className="text-[12px] text-muted">Choose IMAP to keep mail in sync across devices. POP3 downloads mail to one device.</p>
    </div>
  );
}

// ---- Webmail ---------------------------------------------------------------------

export function Webmail() {
  const aid = useAccountId();
  const tools = useGet<{ webmail: string }>("/tools");
  const boxes = useGet<Mailbox[]>(`/accounts/${aid}/mailboxes`);
  return (
    <>
      <PageHeader title="Webmail" />
      <div className="grid gap-4 lg:grid-cols-[1fr_1.3fr]">
        <Card title="Roundcube" description="Read, write and organise mail in the browser.">
          <a href={tools.data?.webmail} target="_blank" rel="noreferrer noopener">
            <Button variant="primary" icon={ExternalLink}>
              Open webmail
            </Button>
          </a>
          <p className="mt-3 text-[12.5px] text-muted">Sign in with the full email address and that account's password, not your cPanel password.</p>
        </Card>
        <Card title="Your email accounts" flush>
          {!boxes.data?.length ? (
            <p className="p-4 text-[13px] text-muted">No email accounts yet.</p>
          ) : (
            <ul className="divide-y divide-line">
              {boxes.data.map((m) => (
                <li key={m.id} className="flex items-center justify-between gap-3 px-4 py-2.5 text-[13px]">
                  <span className="font-medium text-ink">{m.address}</span>
                  <span className="text-muted">{formatMB(m.used_mb)} used</span>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>
    </>
  );
}

// ---- Forwarders ------------------------------------------------------------------

interface DomainForward {
  id: string;
  domain: string;
  target: string;
  created_at: number;
}

export function Forwarders() {
  const aid = useAccountId();
  const [tab, setTab] = useState<"address" | "domain">("address");
  const alKey = `/accounts/${aid}/mail-aliases`;
  const dfKey = `/accounts/${aid}/domain-forwards`;
  const aliases = useGet<MailAlias[]>(alKey);
  const forwards = useGet<DomainForward[]>(dfKey);
  const { mail } = useMailDomains();
  const [open, setOpen] = useState(false);
  const [f, setF] = useState({ local: "", domain: "", dest: "", target: "" });
  const [del, setDel] = useState<{ ids: string[]; kind: "alias" | "domain"; label: string } | null>(null);
  const close = () => {
    setOpen(false);
    setF({ local: "", domain: "", dest: "", target: "" });
  };
  const add = useMutate(
    () => (tab === "address" ? api.post(alKey, { local_part: f.local, domain_id: f.domain, destination: f.dest }) : api.post(dfKey, { domain_id: f.domain, target: f.target })),
    { invalidate: [alKey, dfKey], success: "Forwarder added", onSuccess: close },
  );
  const remove = useMutate(async () => {
    for (const id of del!.ids) await api.del(`${del!.kind === "alias" ? alKey : dfKey}/${id}`);
  }, { invalidate: [alKey, dfKey], success: "Deleted", onSuccess: () => setDel(null) });

  // Catch-alls are managed on the Default Address page.
  const rows = aliases.data?.filter((a) => !a.source.startsWith("*@"));

  return (
    <>
      <PageHeader
        title="Forwarders"
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setOpen(true)}>
            {tab === "address" ? "Add forwarder" : "Add domain forwarder"}
          </Button>
        }
      />
      <Tabs
        tabs={[
          { id: "address", label: `Email account forwarders (${rows?.length ?? 0})` },
          { id: "domain", label: `Domain forwarders (${forwards.data?.length ?? 0})` },
        ]}
        value={tab}
        onChange={setTab}
      />
      {tab === "address" ? (
        <DataTable
          rows={rows}
          loading={aliases.isLoading}
          error={aliases.error}
          rowKey={(a) => a.id}
          search={(a) => `${a.source} ${a.destination}`}
          searchPlaceholder="Search forwarders"
          empty={{ icon: Forward, title: "No forwarders", description: "Forward mail for an address, such as sales@, to one or more other addresses." }}
          bulk={[{ label: "Delete", icon: Trash2, danger: true, onRun: (r) => setDel({ ids: r.map((x) => x.id), kind: "alias", label: `${r.length} forwarder(s)` }) }]}
          initialSort={{ key: "source", dir: "asc" }}
          columns={[
            { key: "source", header: "Address", sort: (a) => a.source, render: (a) => <span className="font-medium text-ink">{a.source}</span> },
            { key: "arrow", header: "", render: () => <ArrowRight className="size-3.5 text-subtle" aria-hidden />, className: "w-6 px-0" },
            { key: "dest", header: "Forward to", sort: (a) => a.destination, render: (a) => <span className="text-muted">{a.destination}</span> },
          ]}
          actions={(a) => <IconButton icon={Trash2} label={`Delete forwarder ${a.source}`} onClick={() => setDel({ ids: [a.id], kind: "alias", label: a.source })} />}
        />
      ) : (
        <DataTable
          rows={forwards.data}
          loading={forwards.isLoading}
          error={forwards.error}
          rowKey={(a) => a.id}
          empty={{ icon: Forward, title: "No domain forwarders", description: "Send all mail for one domain to the same address at another domain: info@old.com goes to info@new.com." }}
          columns={[
            { key: "domain", header: "Domain", sort: (a) => a.domain, render: (a) => <span className="font-medium text-ink">@{a.domain}</span> },
            { key: "arrow", header: "", render: () => <ArrowRight className="size-3.5 text-subtle" aria-hidden />, className: "w-6 px-0" },
            { key: "target", header: "Forward to", sort: (a) => a.target, render: (a) => <span className="text-muted">@{a.target}</span> },
            { key: "created", header: "Added", sort: (a) => a.created_at, render: (a) => <span className="text-muted">{timeAgo(a.created_at)}</span>, hideSm: true },
          ]}
          actions={(a) => <IconButton icon={Trash2} label={`Delete domain forwarder ${a.domain}`} onClick={() => setDel({ ids: [a.id], kind: "domain", label: a.domain })} />}
        />
      )}
      <Modal
        open={open}
        onClose={close}
        title={tab === "address" ? "Add a forwarder" : "Add a domain forwarder"}
        footer={
          <>
            <Button onClick={close}>Cancel</Button>
            <Button variant="primary" loading={add.isPending} disabled={!f.domain || (tab === "address" ? !f.local || !f.dest : !f.target)} onClick={() => add.mutate()}>
              Add
            </Button>
          </>
        }
      >
        {tab === "address" ? (
          <div className="space-y-4">
            <AddressInput local={f.local} setLocal={(v) => setF({ ...f, local: v })} domain={f.domain} setDomain={(v) => setF({ ...f, domain: v })} domains={mail} placeholder="sales" />
            <Field label="Forward to" hint="A full email address. Add more forwarders to send copies to several people.">
              {(id) => <Input id={id} type="email" value={f.dest} onChange={(e) => setF({ ...f, dest: e.target.value })} placeholder="you@example.com" />}
            </Field>
          </div>
        ) : (
          <div className="space-y-4">
            <Field label="Domain">
              {(id) => (
                <Select id={id} value={f.domain} onChange={(e) => setF({ ...f, domain: e.target.value })}>
                  <option value="">Select…</option>
                  {mail.map((d) => (
                    <option key={d.id} value={d.id}>
                      {d.name}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            <Field label="Forward all mail to domain" hint="Every address keeps its name: anna@ goes to anna@ at the new domain, including addresses that have a mailbox here.">
              {(id) => <Input id={id} value={f.target} onChange={(e) => setF({ ...f, target: e.target.value })} placeholder="example.com" />}
            </Field>
          </div>
        )}
      </Modal>
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={() => remove.mutate()} loading={remove.isPending} title={`Delete ${del?.label}?`} message="Mail will no longer be forwarded." />
    </>
  );
}

// ---- Email Routing -----------------------------------------------------------------

interface RoutingRow {
  id: string;
  name: string;
  routing: "local" | "remote";
  mx: string[];
}

export function EmailRouting() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/email-routing`;
  const q = useGet<RoutingRow[]>(key);
  const save = useMutate((v: { id: string; routing: string }) => api.put(`${key}/${v.id}`, { routing: v.routing }), { invalidate: [key], success: "Email routing saved" });
  return (
    <>
      <PageHeader title="Email Routing" />
      <HelpNote>
        <p>
          <strong className="text-ink">Local mail exchanger:</strong> this server receives the domain's mail into the email accounts you create here. Use this unless you use an outside mail service.
        </p>
        <p>
          <strong className="text-ink">Remote mail exchanger:</strong> mail is handled by another provider, such as Google Workspace or Microsoft 365. Point the domain's MX records at that provider in the Zone Editor. Mail sent from your sites to the domain then goes to the provider too, instead of staying on this server.
        </p>
      </HelpNote>
      <DataTable
        rows={q.data}
        loading={q.isLoading}
        error={q.error}
        rowKey={(r) => r.id}
        search={(r) => r.name}
        empty={{ icon: Route, title: "No domains" }}
        columns={[
          { key: "name", header: "Domain", sort: (r) => r.name, render: (r) => <span className="font-medium text-ink">{r.name}</span> },
          { key: "mx", header: "MX records", render: (r) => (r.mx.length ? <div className="space-y-0.5">{r.mx.map((m) => <Mono key={m} className="block">{m}</Mono>)}</div> : <span className="text-muted">none</span>) },
          {
            key: "routing",
            header: "Mail is received by",
            render: (r) => (
              <Select aria-label={`Email routing for ${r.name}`} className="h-7 w-56" value={r.routing} onChange={(e) => save.mutate({ id: r.id, routing: e.target.value })}>
                <option value="local">This server (local)</option>
                <option value="remote">Another server (remote)</option>
              </Select>
            ),
          },
        ]}
      />
    </>
  );
}

// ---- Default Address ---------------------------------------------------------------

interface DefaultRow {
  id: string;
  name: string;
  destination: string;
}

export function DefaultAddress() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/default-address`;
  const q = useGet<DefaultRow[]>(key);
  const [edit, setEdit] = useState<{ row: DefaultRow; mode: "reject" | "forward"; dest: string } | null>(null);
  const save = useMutate(() => api.put(`${key}/${edit!.row.id}`, { destination: edit!.mode === "forward" ? edit!.dest : "" }), { invalidate: [key], success: "Default address saved", onSuccess: () => setEdit(null) });
  return (
    <>
      <PageHeader title="Default Address" />
      <HelpNote>
        <p>Mail to an address that has no email account, forwarder or list is “unrouted”. By default it is rejected, and the sender gets a bounce message.</p>
        <p>You can instead collect it in one address (a catch-all). Catch-alls receive a lot of spam, because spammers guess addresses.</p>
      </HelpNote>
      <DataTable
        rows={q.data}
        loading={q.isLoading}
        error={q.error}
        rowKey={(r) => r.id}
        empty={{ icon: Inbox, title: "No domains" }}
        columns={[
          { key: "name", header: "Domain", sort: (r) => r.name, render: (r) => <span className="font-medium text-ink">{r.name}</span> },
          { key: "dest", header: "Unrouted mail", render: (r) => (r.destination ? <span>Forwarded to <strong className="font-medium text-ink">{r.destination}</strong></span> : <Badge>Rejected with a bounce</Badge>) },
        ]}
        actions={(r) => (
          <Button size="sm" icon={Pencil} onClick={() => setEdit({ row: r, mode: r.destination ? "forward" : "reject", dest: r.destination })}>
            Change
          </Button>
        )}
      />
      <Modal
        open={!!edit}
        onClose={() => setEdit(null)}
        title={`Default address for ${edit?.row.name}`}
        footer={
          <>
            <Button onClick={() => setEdit(null)}>Cancel</Button>
            <Button variant="primary" loading={save.isPending} disabled={edit?.mode === "forward" && !edit.dest} onClick={() => save.mutate()}>
              Save
            </Button>
          </>
        }
      >
        {edit && (
          <fieldset className="space-y-3 text-[13px]">
            <legend className="sr-only">What to do with unrouted mail</legend>
            <label className="flex items-start gap-2.5">
              <input type="radio" name="mode" className="mt-0.5 accent-[var(--accent)]" checked={edit.mode === "reject"} onChange={() => setEdit({ ...edit, mode: "reject" })} />
              <span>
                <span className="font-medium text-ink">Reject it</span>
                <span className="block text-muted">The sender gets a “no such user” bounce. Recommended.</span>
              </span>
            </label>
            <label className="flex items-start gap-2.5">
              <input type="radio" name="mode" className="mt-0.5 accent-[var(--accent)]" checked={edit.mode === "forward"} onChange={() => setEdit({ ...edit, mode: "forward" })} />
              <span className="flex-1">
                <span className="font-medium text-ink">Forward it to an address</span>
                <Input className="mt-1.5" type="email" aria-label="Forward unrouted mail to" placeholder="catchall@example.com" value={edit.dest} disabled={edit.mode !== "forward"} onChange={(e) => setEdit({ ...edit, dest: e.target.value })} />
              </span>
            </label>
          </fieldset>
        )}
      </Modal>
    </>
  );
}

// ---- Mailing Lists ----------------------------------------------------------------

interface MailingList {
  id: string;
  address: string;
  description: string;
  members: string[];
  created_at: number;
}

export function MailingLists() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/mailing-lists`;
  const q = useGet<MailingList[]>(key);
  const { mail } = useMailDomains();
  const [edit, setEdit] = useState<{ id?: string; local: string; domain: string; description: string; members: string } | null>(null);
  const [del, setDel] = useState<MailingList | null>(null);
  const members = (s: string) => s.split(/[\s,;]+/).map((x) => x.trim()).filter(Boolean);
  const save = useMutate(
    () =>
      edit!.id
        ? api.put(`${key}/${edit!.id}`, { description: edit!.description, members: members(edit!.members) })
        : api.post(key, { local_part: edit!.local, domain_id: edit!.domain, description: edit!.description, members: members(edit!.members) }),
    { invalidate: [key], success: "Mailing list saved", onSuccess: () => setEdit(null) },
  );
  const remove = useMutate(() => api.del(`${key}/${del!.id}`), { invalidate: [key], success: "Mailing list deleted", onSuccess: () => setDel(null) });
  const count = edit ? members(edit.members).length : 0;
  return (
    <>
      <PageHeader
        title="Mailing Lists"
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setEdit({ local: "", domain: "", description: "", members: "" })}>
            Create list
          </Button>
        }
      />
      <HelpNote>
        <p>A list is one address, such as team@yourdomain.com, that delivers every message to all of its members. Members can be anywhere, not only on this server.</p>
        <p>These are distribution lists: there is no archive or subscription page. Anyone can write to the list address, so share it only with people who should use it.</p>
      </HelpNote>
      <DataTable
        rows={q.data}
        loading={q.isLoading}
        error={q.error}
        rowKey={(l) => l.id}
        search={(l) => `${l.address} ${l.description} ${l.members.join(" ")}`}
        searchPlaceholder="Search lists and members"
        empty={{ icon: UsersRound, title: "No mailing lists", description: "Create a list to reach a group with one address." }}
        initialSort={{ key: "address", dir: "asc" }}
        columns={[
          { key: "address", header: "List address", sort: (l) => l.address, render: (l) => <div><div className="font-medium text-ink">{l.address}</div>{l.description && <div className="text-[12px] text-muted">{l.description}</div>}</div> },
          { key: "members", header: "Members", sort: (l) => l.members.length, render: (l) => <span title={l.members.join("\n")} className="tabular-nums">{l.members.length}</span> },
          { key: "created", header: "Created", sort: (l) => l.created_at, render: (l) => <span className="text-muted">{formatDate(l.created_at)}</span>, hideSm: true },
        ]}
        actions={(l) => (
          <>
            <IconButton icon={Pencil} label={`Edit ${l.address}`} onClick={() => setEdit({ id: l.id, local: "", domain: "", description: l.description, members: l.members.join("\n") })} />
            <IconButton icon={Trash2} label={`Delete ${l.address}`} onClick={() => setDel(l)} />
          </>
        )}
      />
      <Modal
        open={!!edit}
        onClose={() => setEdit(null)}
        width="max-w-lg"
        title={edit?.id ? "Edit mailing list" : "Create a mailing list"}
        footer={
          <>
            <Button onClick={() => setEdit(null)}>Cancel</Button>
            <Button variant="primary" loading={save.isPending} disabled={!edit || (!edit.id && (!edit.local || !edit.domain))} onClick={() => save.mutate()}>
              Save
            </Button>
          </>
        }
      >
        {edit && (
          <div className="space-y-4">
            {!edit.id && <AddressInput local={edit.local} setLocal={(v) => setEdit({ ...edit, local: v })} domain={edit.domain} setDomain={(v) => setEdit({ ...edit, domain: v })} domains={mail} placeholder="team" />}
            <Field label="Description (optional)">{(id) => <Input id={id} value={edit.description} onChange={(e) => setEdit({ ...edit, description: e.target.value })} placeholder="Everyone in the office" />}</Field>
            <Field label={`Members (${count})`} hint="One address per line. You can paste a list separated by commas.">
              {(id) => <Textarea id={id} rows={8} value={edit.members} onChange={(e) => setEdit({ ...edit, members: e.target.value })} placeholder={"anna@example.com\nben@example.org"} />}
            </Field>
          </div>
        )}
      </Modal>
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={() => remove.mutate()} loading={remove.isPending} title={`Delete ${del?.address}?`} message="Mail sent to the list address will bounce." />
    </>
  );
}

// ---- Email Filters --------------------------------------------------------------------

interface Rule {
  id: string;
  mailbox_id: string;
  name: string;
  field: string;
  op: string;
  value: string;
  action: string;
  dest: string;
  position: number;
}

const fieldLabel: Record<string, string> = { from: "From", to: "To or Cc", subject: "Subject", any: "Any header" };
const opLabel: Record<string, string> = { contains: "contains", not_contains: "does not contain", is: "is exactly", begins: "begins with", ends: "ends with" };
const actionLabel: Record<string, string> = { fileinto: "Move to folder", redirect: "Redirect to", discard: "Delete", keep: "Deliver and stop" };

export function EmailFilters() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/mail-filters`;
  const rules = useGet<Rule[]>(key);
  const boxes = useGet<Mailbox[]>(`/accounts/${aid}/mailboxes`);
  const [scope, setScope] = useState("");
  const [edit, setEdit] = useState<Omit<Rule, "id" | "position"> | null>(null);
  const [del, setDel] = useState<Rule | null>(null);
  const [test, setTest] = useState({ from: "", to: "", subject: "" });
  const [result, setResult] = useState<{ steps: { name: string; matched: boolean }[]; result: { action: string; dest: string; rule?: string } } | null>(null);

  const create = useMutate(() => api.post(key, edit), { invalidate: [key], success: "Filter created", onSuccess: () => setEdit(null) });
  const remove = useMutate(() => api.del(`${key}/${del!.id}`), { invalidate: [key], success: "Filter deleted", onSuccess: () => setDel(null) });
  const order = useMutate((ids: string[]) => api.post(`${key}/order`, { ids }), { invalidate: [key] });
  const runTest = useMutate(() => api.post<typeof result>(`${key}/test`, { mailbox_id: scope, ...test }), { onSuccess: (r) => setResult(r) });

  const all = rules.data ?? [];
  const list = useMemo(() => all.filter((r) => r.mailbox_id === scope), [all, scope]);
  const move = (i: number, d: -1 | 1) => {
    const ids = list.map((r) => r.id);
    const j = i + d;
    if (j < 0 || j >= ids.length) return;
    [ids[i], ids[j]] = [ids[j]!, ids[i]!];
    // Keep the other scope's rules after these.
    order.mutate([...ids, ...all.filter((r) => r.mailbox_id !== scope).map((r) => r.id)]);
  };
  const scopeName = scope ? boxes.data?.find((b) => b.id === scope)?.address : "all email accounts";

  return (
    <>
      <PageHeader
        title="Email Filters"
        actions={
          <>
            <Select aria-label="Filters for" className="w-64" value={scope} onChange={(e) => (setScope(e.target.value), setResult(null))}>
              <option value="">Global (all email accounts)</option>
              {boxes.data?.map((b) => (
                <option key={b.id} value={b.id}>
                  {b.address}
                </option>
              ))}
            </Select>
            <Button variant="primary" icon={Plus} onClick={() => setEdit({ mailbox_id: scope, name: "", field: "subject", op: "contains", value: "", action: "fileinto", dest: "" })}>
              Create filter
            </Button>
          </>
        }
      />
      <HelpNote>
        <p>Filters run in order, top to bottom, on every new message: first the global filters, then the ones of the receiving account. The first filter that matches decides what happens, and later filters are skipped.</p>
        <p>Filters run before the spam filter, so you can always deliver mail from someone important.</p>
      </HelpNote>
      <div className="grid gap-4 xl:grid-cols-[1.5fr_1fr]">
        <Card title={`Filters for ${scopeName}`} flush>
          {rules.isLoading ? (
            <Loading />
          ) : !list.length ? (
            <EmptyState icon={MailSearch} title="No filters" description="Create a filter to sort mail into folders, forward it or delete it." />
          ) : (
            <ol className="divide-y divide-line">
              {list.map((r, i) => (
                <li key={r.id} className="flex items-start gap-3 px-4 py-3">
                  <span className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded bg-surface2 text-[11px] font-semibold tabular-nums text-muted">{i + 1}</span>
                  <div className="min-w-0 flex-1 text-[13px]">
                    <div className="font-medium text-ink">{r.name}</div>
                    <div className="mt-0.5 text-muted">
                      If <strong className="font-medium text-ink">{fieldLabel[r.field]}</strong> {opLabel[r.op]} “<span className="text-ink">{r.value}</span>” → {actionLabel[r.action]}
                      {r.dest && <> <strong className="font-medium text-ink">{r.dest}</strong></>}
                    </div>
                  </div>
                  <div className="flex shrink-0">
                    <IconButton icon={ArrowUp} label={`Move ${r.name} up`} disabled={i === 0} onClick={() => move(i, -1)} />
                    <IconButton icon={ArrowDown} label={`Move ${r.name} down`} disabled={i === list.length - 1} onClick={() => move(i, 1)} />
                    <IconButton icon={Trash2} label={`Delete ${r.name}`} onClick={() => setDel(r)} />
                  </div>
                </li>
              ))}
            </ol>
          )}
        </Card>
        <Card title="Test filters" description="See what would happen to a message, without sending one.">
          <div className="space-y-3">
            <Field label="From">{(id) => <Input id={id} value={test.from} onChange={(e) => setTest({ ...test, from: e.target.value })} placeholder="billing@example.com" />}</Field>
            <Field label="To">{(id) => <Input id={id} value={test.to} onChange={(e) => setTest({ ...test, to: e.target.value })} />}</Field>
            <Field label="Subject">{(id) => <Input id={id} value={test.subject} onChange={(e) => setTest({ ...test, subject: e.target.value })} placeholder="Your invoice" />}</Field>
            <Button icon={FlaskConical} loading={runTest.isPending} onClick={() => runTest.mutate()}>
              Test
            </Button>
            {result && (
              <div className="rounded-md border border-line bg-surface2 p-3 text-[13px]">
                <div className="font-medium text-ink">
                  {result.result.action === "deliver" ? "Delivered to the Inbox" : `${actionLabel[result.result.action]} ${result.result.dest}`.trim()}
                </div>
                {result.result.rule && <div className="text-muted">by the filter “{result.result.rule}”</div>}
                {result.steps.length > 0 && (
                  <ul className="mt-2 space-y-0.5 text-[12px]">
                    {result.steps.map((s, i) => (
                      <li key={i} className={s.matched ? "text-ok" : "text-muted"}>
                        {s.matched ? "✓ matched" : "– no match"}: {s.name}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            )}
          </div>
        </Card>
      </div>
      <Modal
        open={!!edit}
        onClose={() => setEdit(null)}
        width="max-w-lg"
        title="Create a filter"
        description={`Applies to ${scopeName}.`}
        footer={
          <>
            <Button onClick={() => setEdit(null)}>Cancel</Button>
            <Button variant="primary" loading={create.isPending} disabled={!edit?.name || !edit.value || ((edit.action === "fileinto" || edit.action === "redirect") && !edit.dest)} onClick={() => create.mutate()}>
              Create
            </Button>
          </>
        }
      >
        {edit && (
          <div className="space-y-4">
            <Field label="Filter name">{(id) => <Input id={id} value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} placeholder="Invoices" autoFocus />}</Field>
            <div className="grid gap-2 sm:grid-cols-[1fr_1fr]">
              <Field label="When">
                {(id) => (
                  <Select id={id} value={edit.field} onChange={(e) => setEdit({ ...edit, field: e.target.value })}>
                    {Object.entries(fieldLabel).map(([k, v]) => (
                      <option key={k} value={k}>
                        {v}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
              <Field label="Comparison">
                {(id) => (
                  <Select id={id} value={edit.op} onChange={(e) => setEdit({ ...edit, op: e.target.value })}>
                    {Object.entries(opLabel).map(([k, v]) => (
                      <option key={k} value={k}>
                        {v}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
            </div>
            <Field label="Value" hint="Not case-sensitive.">{(id) => <Input id={id} value={edit.value} onChange={(e) => setEdit({ ...edit, value: e.target.value })} placeholder="invoice" />}</Field>
            <div className="grid gap-2 sm:grid-cols-[1fr_1fr]">
              <Field label="Then">
                {(id) => (
                  <Select id={id} value={edit.action} onChange={(e) => setEdit({ ...edit, action: e.target.value, dest: "" })}>
                    {Object.entries(actionLabel).map(([k, v]) => (
                      <option key={k} value={k}>
                        {v}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
              {(edit.action === "fileinto" || edit.action === "redirect") && (
                <Field label={edit.action === "fileinto" ? "Folder" : "Address"} hint={edit.action === "fileinto" ? "Created if it does not exist." : undefined}>
                  {(id) => <Input id={id} value={edit.dest} onChange={(e) => setEdit({ ...edit, dest: e.target.value })} placeholder={edit.action === "fileinto" ? "Billing" : "you@example.com"} />}
                </Field>
              )}
            </div>
          </div>
        )}
      </Modal>
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={() => remove.mutate()} loading={remove.isPending} title={`Delete the filter “${del?.name}”?`} message="New mail will no longer be sorted by it." />
    </>
  );
}

// ---- Track Delivery ---------------------------------------------------------------------

interface Delivery {
  time: string;
  queue_id: string;
  from: string;
  to: string;
  status: string;
  relay: string;
  detail: string;
}

export function TrackDelivery() {
  const aid = useAccountId();
  const [q, setQ] = useState("");
  const [status, setStatus] = useState("");
  const [applied, setApplied] = useState({ q: "", status: "" });
  const data = useGet<Delivery[]>(`/accounts/${aid}/mail-delivery?q=${encodeURIComponent(applied.q)}&status=${applied.status}`);
  const [open, setOpen] = useState<Delivery | null>(null);
  const tone = (s: string) => (s === "sent" ? "ok" : s === "deferred" ? "warn" : "bad");
  const rows = data.data ?? [];
  return (
    <>
      <PageHeader title="Track Delivery" />
      <form
        className="mb-4 flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          setApplied({ q, status });
        }}
      >
        <Field label="Address contains" className="w-72">
          {(id) => <Input id={id} value={q} onChange={(e) => setQ(e.target.value)} placeholder="client@example.com" />}
        </Field>
        <Field label="Result" className="w-44">
          {(id) => (
            <Select id={id} value={status} onChange={(e) => setStatus(e.target.value)}>
              <option value="">All</option>
              <option value="sent">Delivered</option>
              <option value="deferred">Delayed</option>
              <option value="bounced">Bounced</option>
            </Select>
          )}
        </Field>
        <Button type="submit" variant="primary" icon={MailSearch} loading={data.isFetching}>
          Run report
        </Button>
      </form>
      <div className="mb-4 grid grid-cols-3 gap-3">
        <Stat label="Delivered" value={rows.filter((r) => r.status === "sent").length} tone="ok" />
        <Stat label="Delayed" value={rows.filter((r) => r.status === "deferred").length} tone={rows.some((r) => r.status === "deferred") ? "warn" : undefined} />
        <Stat label="Bounced" value={rows.filter((r) => r.status === "bounced").length} tone={rows.some((r) => r.status === "bounced") ? "bad" : undefined} />
      </div>
      <DataTable
        rows={data.data}
        loading={data.isLoading}
        error={data.error}
        rowKey={(r) => `${r.queue_id}-${r.to}-${r.time}`}
        empty={{ icon: MailSearch, title: "No deliveries found", description: "Only messages sent from or to your domains are shown, from the recent mail log." }}
        columns={[
          { key: "time", header: "Time", sort: (r) => r.time, render: (r) => <span className="whitespace-nowrap text-muted">{new Date(r.time).toString() === "Invalid Date" ? r.time : new Date(r.time).toLocaleString()}</span> },
          { key: "from", header: "Sender", sort: (r) => r.from, render: (r) => <span className="break-all">{r.from || <span className="text-muted">(unknown)</span>}</span> },
          { key: "to", header: "Recipient", sort: (r) => r.to, render: (r) => <span className="break-all font-medium text-ink">{r.to}</span> },
          { key: "status", header: "Result", sort: (r) => r.status, render: (r) => <Badge tone={tone(r.status)}>{r.status === "sent" ? "Delivered" : r.status === "deferred" ? "Delayed" : "Bounced"}</Badge> },
        ]}
        actions={(r) => (
          <Button size="sm" variant="ghost" onClick={() => setOpen(r)}>
            Details
          </Button>
        )}
      />
      <Modal open={!!open} onClose={() => setOpen(null)} title="Delivery details" width="max-w-lg">
        {open && (
          <dl className="grid grid-cols-[auto_1fr] gap-x-5 gap-y-2 text-[13px]">
            <dt className="text-muted">Queue ID</dt>
            <dd className="font-mono">{open.queue_id}</dd>
            <dt className="text-muted">Sender</dt>
            <dd className="break-all">{open.from || "(unknown)"}</dd>
            <dt className="text-muted">Recipient</dt>
            <dd className="break-all">{open.to}</dd>
            <dt className="text-muted">Result</dt>
            <dd>
              <Badge tone={tone(open.status)}>{open.status}</Badge>
            </dd>
            <dt className="text-muted">Delivered via</dt>
            <dd className="break-all font-mono text-[12px]">{open.relay || "—"}</dd>
            <dt className="text-muted">Server reply</dt>
            <dd className="break-all">{open.detail || "—"}</dd>
          </dl>
        )}
      </Modal>
    </>
  );
}

// ---- Email Deliverability ----------------------------------------------------------------

interface Check {
  kind: string;
  status: "valid" | "missing" | "invalid";
  current: string;
  suggested: string;
  name: string;
  note: string;
}

const checkTitle: Record<string, string> = { spf: "SPF", dkim: "DKIM", dmarc: "DMARC", mx: "MX", ptr: "Reverse DNS (PTR)" };
const checkWhy: Record<string, string> = {
  spf: "Lists the servers allowed to send mail for the domain.",
  dkim: "Signs outgoing mail so receivers can tell it was not altered.",
  dmarc: "Tells receivers what to do with mail that fails SPF and DKIM.",
  mx: "Tells other servers where to deliver mail for the domain.",
  ptr: "Maps the server's IP address back to its host name.",
};

export function Deliverability() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/email-deliverability`;
  const q = useGet<{ domain: string; checks: Check[] }[]>(key);
  const repair = useMutate((v: { zone: string; kind: string }) => api.post(`${key}/${v.zone}/repair`, { kind: v.kind }), { invalidate: [key], success: "Record installed" });
  if (q.isLoading) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  return (
    <>
      <PageHeader title="Email Deliverability" />
      <div className="space-y-4">
        {q.data?.map((d) => {
          const bad = d.checks.filter((c) => c.status !== "valid").length;
          return (
            <Card key={d.domain} title={d.domain} actions={bad ? <Badge tone="warn">{bad} problem(s)</Badge> : <Badge tone="ok">All good</Badge>} flush>
              <ul className="divide-y divide-line">
                {d.checks.map((c) => (
                  <li key={c.kind} className="flex flex-wrap items-start gap-3 px-4 py-3">
                    {c.status === "valid" ? <CheckCircle2 className="mt-0.5 size-4 text-ok" aria-label="Valid" /> : c.status === "invalid" ? <CircleAlert className="mt-0.5 size-4 text-warn" aria-label="Problem" /> : <CircleX className="mt-0.5 size-4 text-bad" aria-label="Missing" />}
                    <div className="min-w-0 flex-1 text-[13px]">
                      <div className="font-medium text-ink">{checkTitle[c.kind]}</div>
                      <div className="text-muted">{checkWhy[c.kind]}</div>
                      {c.current && <Mono className="mt-1 block break-all text-muted">{c.current}</Mono>}
                      {c.status !== "valid" && c.suggested && (
                        <div className="mt-1.5 text-[12.5px]">
                          Suggested <Mono className="break-all">{c.name}</Mono>: <Mono className="break-all text-ink">{c.suggested}</Mono>
                        </div>
                      )}
                      {c.note && <div className="mt-1 text-[12.5px] text-muted">{c.note}</div>}
                    </div>
                    {c.status !== "valid" && c.kind !== "ptr" && (
                      <Button size="sm" icon={Wrench} loading={repair.isPending && repair.variables?.zone === d.domain && repair.variables.kind === c.kind} onClick={() => repair.mutate({ zone: d.domain, kind: c.kind })}>
                        Repair
                      </Button>
                    )}
                  </li>
                ))}
              </ul>
            </Card>
          );
        })}
      </div>
    </>
  );
}
