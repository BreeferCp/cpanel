import { ArrowRightLeft, Check, DatabaseBackup, ExternalLink, FileType2, FolderTree, ListTree, Plus, Radar, Trash2, Wifi, Zap } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import { formatDate, formatMB, timeAgo } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { Database, Domain } from "@/lib/types";
import { useAccountId, useBase } from "@/state/scope";
import { Button, IconButton } from "@/components/ui/Button";
import { CopyField } from "@/components/ui/CopyField";
import { DataTable } from "@/components/ui/DataTable";
import { Checkbox, Field, Input, Select, Switch, Textarea } from "@/components/ui/Form";
import { Badge, Card, ErrorNote, HelpNote, Loading, Mono, PageHeader, Tabs } from "@/components/ui/Layout";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

function useDomains() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/domains`;
  return { key, q: useGet<Domain[]>(key) };
}

// ---- Subdomains and Aliases ----------------------------------------------------------

function DomainKindPage({ kind }: { kind: "subdomain" | "alias" }) {
  const { key, q } = useDomains();
  const base = useBase();
  const parents = q.data?.filter((d) => d.kind === "primary" || d.kind === "addon") ?? [];
  const rows = q.data?.filter((d) => d.kind === kind);
  const [open, setOpen] = useState(false);
  const [f, setF] = useState({ label: "", parent: "", name: "", doc_root: "" });
  const [del, setDel] = useState<Domain | null>(null);
  useEffect(() => {
    if (!f.parent && parents[0]) setF((x) => ({ ...x, parent: parents[0]!.name }));
  }, [parents, f.parent]);
  const full = kind === "subdomain" ? (f.label ? `${f.label.toLowerCase()}.${f.parent}` : "") : f.name.toLowerCase();
  const add = useMutate(() => api.post(key, { name: full, kind, doc_root: kind === "subdomain" ? f.doc_root || `public_html/${f.label.toLowerCase()}` : "" }), {
    invalidate: [key],
    success: kind === "subdomain" ? "Subdomain created" : "Alias created",
    onSuccess: () => {
      setOpen(false);
      setF({ label: "", parent: parents[0]?.name ?? "", name: "", doc_root: "" });
    },
  });
  const remove = useMutate(() => api.del(`${key}/${del!.id}`), { invalidate: [key], success: "Removed", onSuccess: () => setDel(null) });
  const sub = kind === "subdomain";
  return (
    <>
      <PageHeader
        title={sub ? "Subdomains" : "Aliases"}
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setOpen(true)}>
            {sub ? "Create subdomain" : "Create alias"}
          </Button>
        }
      />
      {!sub && (
        <HelpNote>
          <p>An alias (a “parked domain”) shows exactly the same site as your main domain. Register the domain first and point its nameservers at this server.</p>
          <p>To show a different site, add it as an addon domain under Domains instead.</p>
        </HelpNote>
      )}
      <DataTable
        rows={rows}
        loading={q.isLoading}
        error={q.error}
        rowKey={(d) => d.id}
        search={(d) => `${d.name} ${d.doc_root}`}
        empty={{ icon: sub ? ListTree : ArrowRightLeft, title: sub ? "No subdomains" : "No aliases", description: sub ? "Create blog.yourdomain.com or shop.yourdomain.com with its own folder." : "Show your site on another domain name." }}
        initialSort={{ key: "name", dir: "asc" }}
        columns={[
          { key: "name", header: sub ? "Subdomain" : "Alias", sort: (d) => d.name, render: (d) => <a href={`http://${d.name}`} target="_blank" rel="noreferrer noopener" className="font-medium text-ink hover:underline">{d.name}</a> },
          ...(sub ? [{ key: "root", header: "Document root", render: (d: Domain) => <Mono className="text-muted">{d.doc_root.replace(/^.*?(public_html)/, "$1")}</Mono> }] : []),
          { key: "ssl", header: "HTTPS", render: (d) => (d.has_ssl ? <Badge tone="ok">Secured</Badge> : <Link to={`${base}/ssl`} className="text-[12.5px] text-accent hover:underline">Get certificate</Link>) },
          { key: "php", header: "PHP", render: (d) => <span className="text-muted">{d.php_version}</span>, hideSm: true },
          { key: "created", header: "Created", sort: (d) => d.created_at, render: (d) => <span className="text-muted">{formatDate(d.created_at)}</span>, hideSm: true },
        ]}
        actions={(d) => <IconButton icon={Trash2} label={`Remove ${d.name}`} onClick={() => setDel(d)} />}
      />
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title={sub ? "Create a subdomain" : "Create an alias"}
        footer={
          <>
            <Button onClick={() => setOpen(false)}>Cancel</Button>
            <Button variant="primary" loading={add.isPending} disabled={!full} onClick={() => add.mutate()}>
              Create
            </Button>
          </>
        }
      >
        {sub ? (
          <div className="space-y-4">
            <div className="grid grid-cols-[1fr_auto_1fr] items-end gap-2">
              <Field label="Subdomain">{(id) => <Input id={id} value={f.label} onChange={(e) => setF({ ...f, label: e.target.value })} placeholder="blog" autoFocus />}</Field>
              <span className="pb-1.5 text-muted">.</span>
              <Field label="Domain">
                {(id) => (
                  <Select id={id} value={f.parent} onChange={(e) => setF({ ...f, parent: e.target.value })}>
                    {parents.map((d) => (
                      <option key={d.id}>{d.name}</option>
                    ))}
                  </Select>
                )}
              </Field>
            </div>
            <Field label="Document root" hint="The folder in your home directory that holds the subdomain's files.">
              {(id) => <Input id={id} className="font-mono" value={f.doc_root} onChange={(e) => setF({ ...f, doc_root: e.target.value })} placeholder={`public_html/${f.label || "blog"}`} />}
            </Field>
          </div>
        ) : (
          <Field label="Domain" hint="It must already be registered.">
            {(id) => <Input id={id} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="example.net" autoFocus />}
          </Field>
        )}
      </Modal>
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={() => remove.mutate()} loading={remove.isPending} title={`Remove ${del?.name}?`} message={sub ? "The subdomain stops working. Its files stay in your home directory." : "The alias stops showing your site."} confirmLabel="Remove" />
    </>
  );
}

export const Subdomains = () => <DomainKindPage kind="subdomain" />;
export const Aliases = () => <DomainKindPage kind="alias" />;

// ---- Dynamic DNS -------------------------------------------------------------------------

interface DdnsHost {
  id: string;
  host: string;
  zone: string;
  description: string;
  last_ip: string;
  last_update: number;
  created_at: number;
}

export function DynamicDns() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/ddns`;
  const q = useGet<DdnsHost[]>(key);
  const { q: doms } = useDomains();
  const zones = doms.data?.filter((d) => d.kind === "primary" || d.kind === "addon") ?? [];
  const [open, setOpen] = useState(false);
  const [f, setF] = useState({ name: "", zone: "", description: "" });
  const [made, setMade] = useState<{ host: string; path: string } | null>(null);
  const [del, setDel] = useState<DdnsHost | null>(null);
  const add = useMutate(() => api.post<{ path: string }>(key, { ...f, zone: f.zone || zones[0]?.name }), {
    invalidate: [key],
    onSuccess: (r) => {
      setMade({ host: `${f.name}.${f.zone || zones[0]?.name}`, path: r.path });
      setOpen(false);
      setF({ name: "", zone: "", description: "" });
    },
  });
  const remove = useMutate(() => api.del(`${key}/${del!.id}`), { invalidate: [key], success: "Removed", onSuccess: () => setDel(null) });
  const url = (p: string) => `${window.location.origin}${p}`;
  return (
    <>
      <PageHeader
        title="Dynamic DNS"
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setOpen(true)}>
            Create host
          </Button>
        }
      />
      <HelpNote>
        <p>Home and office internet connections often get a new IP address now and then. A dynamic DNS host such as home.yourdomain.com follows it: your router or a small script calls a secret address, and the panel points the host at the address the call came from.</p>
        <p>Most routers support this under “DDNS” with a custom URL. From a script: <Mono>curl -fsS "&lt;update URL&gt;"</Mono> every five minutes. Replies follow the dyndns convention: <Mono>good</Mono>, <Mono>nochg</Mono> or <Mono>badauth</Mono>.</p>
      </HelpNote>
      <DataTable
        rows={q.data}
        loading={q.isLoading}
        error={q.error}
        rowKey={(h) => h.id}
        search={(h) => `${h.host} ${h.description} ${h.last_ip}`}
        empty={{ icon: Wifi, title: "No dynamic DNS hosts" }}
        columns={[
          { key: "host", header: "Host", sort: (h) => h.host, render: (h) => <div><div className="font-medium text-ink">{h.host}</div>{h.description && <div className="text-[12px] text-muted">{h.description}</div>}</div> },
          { key: "ip", header: "Points to", render: (h) => (h.last_ip ? <Mono>{h.last_ip}</Mono> : <span className="text-muted">not updated yet</span>) },
          { key: "upd", header: "Last update", sort: (h) => h.last_update, render: (h) => <span className="text-muted">{h.last_update ? timeAgo(h.last_update) : "never"}</span> },
        ]}
        actions={(h) => <IconButton icon={Trash2} label={`Remove ${h.host}`} onClick={() => setDel(h)} />}
      />
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Create a dynamic DNS host"
        footer={
          <>
            <Button onClick={() => setOpen(false)}>Cancel</Button>
            <Button variant="primary" loading={add.isPending} disabled={!f.name} onClick={() => add.mutate()}>
              Create
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="grid grid-cols-[1fr_auto_1fr] items-end gap-2">
            <Field label="Host">{(id) => <Input id={id} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="home" autoFocus />}</Field>
            <span className="pb-1.5 text-muted">.</span>
            <Field label="Domain">
              {(id) => (
                <Select id={id} value={f.zone || zones[0]?.name} onChange={(e) => setF({ ...f, zone: e.target.value })}>
                  {zones.map((d) => (
                    <option key={d.id}>{d.name}</option>
                  ))}
                </Select>
              )}
            </Field>
          </div>
          <Field label="Description (optional)">{(id) => <Input id={id} value={f.description} onChange={(e) => setF({ ...f, description: e.target.value })} placeholder="Office router" />}</Field>
        </div>
      </Modal>
      <Modal open={!!made} onClose={() => setMade(null)} title={`${made?.host} is ready`} width="max-w-lg" footer={<Button variant="primary" onClick={() => setMade(null)}>Done</Button>}>
        {made && (
          <div className="space-y-3 text-[13px]">
            <p>Give this update URL to your router or script. It is shown only once: it contains the secret that allows changing this host.</p>
            <CopyField value={url(made.path)} />
          </div>
        )}
      </Modal>
      <ConfirmDialog open={!!del} onClose={() => setDel(null)} onConfirm={() => remove.mutate()} loading={remove.isPending} title={`Remove ${del?.host}?`} message="The update URL stops working. The DNS record stays until you delete it in the Zone Editor." confirmLabel="Remove" />
    </>
  );
}

// ---- Database Wizard ----------------------------------------------------------------------

const allPrivs = ["SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "DROP", "ALTER", "INDEX", "REFERENCES", "CREATE TEMPORARY TABLES", "LOCK TABLES", "EXECUTE", "CREATE VIEW", "SHOW VIEW", "CREATE ROUTINE", "ALTER ROUTINE", "EVENT", "TRIGGER"];

export function DatabaseWizard() {
  const aid = useAccountId();
  const base = useBase();
  const acct = useGet<{ sys_user: string }>(`/accounts/${aid}`);
  const prefix = acct.data ? `${acct.data.sys_user}_` : "";
  const [step, setStep] = useState(1);
  const [db, setDb] = useState({ name: "", id: "", full: "" });
  const [user, setUser] = useState({ name: "", password: "", id: "", full: "" });
  const [privs, setPrivs] = useState<string[]>(allPrivs);
  const mkDb = useMutate(() => api.post<{ id: string; name: string }>(`/accounts/${aid}/databases`, { name: db.name }), {
    invalidate: [`/accounts/${aid}/databases`],
    onSuccess: (r) => (setDb({ ...db, id: r.id, full: r.name }), setStep(2)),
  });
  const mkUser = useMutate(() => api.post<{ id: string; name: string }>(`/accounts/${aid}/db-users`, { name: user.name, password: user.password }), {
    invalidate: [`/accounts/${aid}/db-users`],
    onSuccess: (r) => (setUser({ ...user, id: r.id, full: r.name }), setStep(3)),
  });
  const grant = useMutate(() => api.put(`/accounts/${aid}/databases/${db.id}/grants/${user.id}`, { privileges: privs.length === allPrivs.length ? ["ALL"] : privs }), {
    invalidate: [`/accounts/${aid}/databases`],
    onSuccess: () => setStep(4),
  });
  const gen = () => {
    const a = new Uint32Array(20);
    crypto.getRandomValues(a);
    const cs = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789-_!";
    setUser({ ...user, password: Array.from(a, (n) => cs[n % cs.length]).join("") });
  };
  const steps = ["Create a database", "Create a user", "Set privileges", "Done"];
  return (
    <>
      <PageHeader title="Database Wizard" />
      <ol className="mb-5 flex flex-wrap gap-2 text-[12.5px]">
        {steps.map((s, i) => (
          <li key={s} className={`flex items-center gap-2 rounded-full border px-3 py-1 ${step === i + 1 ? "border-accent bg-accent-soft text-accent" : step > i + 1 ? "border-line text-ok" : "border-line text-muted"}`}>
            {step > i + 1 ? <Check className="size-3.5" aria-hidden /> : <span className="tabular-nums">{i + 1}</span>}
            {s}
          </li>
        ))}
      </ol>
      <Card className="max-w-xl">
        {step === 1 && (
          <form className="space-y-4" onSubmit={(e) => (e.preventDefault(), mkDb.mutate())}>
            <Field label="Database name" hint={`Letters, digits and underscores. It is saved as ${prefix}${db.name || "name"}.`}>
              {(id) => (
                <div className="flex items-center">
                  <span className="rounded-l-md border border-r-0 border-strong bg-surface2 px-2.5 py-1.5 font-mono text-[12.5px] text-muted">{prefix}</span>
                  <Input id={id} className="rounded-l-none" value={db.name} onChange={(e) => setDb({ ...db, name: e.target.value })} autoFocus />
                </div>
              )}
            </Field>
            <Button type="submit" variant="primary" loading={mkDb.isPending} disabled={!db.name}>
              Next step
            </Button>
          </form>
        )}
        {step === 2 && (
          <form className="space-y-4" onSubmit={(e) => (e.preventDefault(), mkUser.mutate())}>
            <Field label="Username" hint={`Saved as ${prefix}${user.name || "user"}.`}>
              {(id) => (
                <div className="flex items-center">
                  <span className="rounded-l-md border border-r-0 border-strong bg-surface2 px-2.5 py-1.5 font-mono text-[12.5px] text-muted">{prefix}</span>
                  <Input id={id} className="rounded-l-none" value={user.name} onChange={(e) => setUser({ ...user, name: e.target.value })} autoFocus />
                </div>
              )}
            </Field>
            <Field label="Password" hint="Your site's configuration file needs it; you do not need to remember it.">
              {(id) => (
                <div className="flex gap-2">
                  <Input id={id} className="font-mono" value={user.password} onChange={(e) => setUser({ ...user, password: e.target.value })} autoComplete="new-password" />
                  <Button onClick={gen}>Generate</Button>
                </div>
              )}
            </Field>
            <Button type="submit" variant="primary" loading={mkUser.isPending} disabled={!user.name || user.password.length < 12}>
              Create user
            </Button>
          </form>
        )}
        {step === 3 && (
          <div className="space-y-4">
            <p className="text-[13px] text-muted">
              Choose what <Mono className="text-ink">{user.full}</Mono> may do in <Mono className="text-ink">{db.full}</Mono>. Most applications need all privileges.
            </p>
            <label className="flex items-center gap-2 text-[13px] font-medium text-ink">
              <Checkbox checked={privs.length === allPrivs.length} onChange={(e) => setPrivs(e.target.checked ? allPrivs : [])} />
              ALL PRIVILEGES
            </label>
            <div className="grid gap-1.5 sm:grid-cols-2">
              {allPrivs.map((p) => (
                <label key={p} className="flex items-center gap-2 text-[12.5px]">
                  <Checkbox checked={privs.includes(p)} onChange={(e) => setPrivs(e.target.checked ? [...privs, p] : privs.filter((x) => x !== p))} />
                  {p}
                </label>
              ))}
            </div>
            <Button variant="primary" loading={grant.isPending} disabled={!privs.length} onClick={() => grant.mutate()}>
              Make changes
            </Button>
          </div>
        )}
        {step === 4 && (
          <div className="space-y-3 text-[13px]">
            <p className="font-medium text-ok">The database is ready.</p>
            <dl className="grid grid-cols-[auto_1fr] items-center gap-x-5 gap-y-2">
              <dt className="text-muted">Host</dt>
              <dd><CopyField value="localhost" /></dd>
              <dt className="text-muted">Database</dt>
              <dd><CopyField value={db.full} /></dd>
              <dt className="text-muted">Username</dt>
              <dd><CopyField value={user.full} /></dd>
              <dt className="text-muted">Password</dt>
              <dd><CopyField value={user.password} /></dd>
            </dl>
            <div className="flex gap-2 pt-2">
              <Button onClick={() => (setStep(1), setDb({ name: "", id: "", full: "" }), setUser({ name: "", password: "", id: "", full: "" }), setPrivs(allPrivs))}>Create another</Button>
              <Link to={`${base}/databases`}>
                <Button variant="ghost">Go to MySQL Databases</Button>
              </Link>
            </div>
          </div>
        )}
      </Card>
    </>
  );
}

// ---- phpMyAdmin -----------------------------------------------------------------------

export function PhpMyAdmin() {
  const aid = useAccountId();
  const dbs = useGet<Database[]>(`/accounts/${aid}/databases`);
  const pma = useMutate(() => api.post<{ url: string }>(`/accounts/${aid}/phpmyadmin`), { onSuccess: (r) => window.open(r.url, "_blank", "noopener") });
  return (
    <>
      <PageHeader
        title="phpMyAdmin"
        actions={
          <Button variant="primary" icon={ExternalLink} loading={pma.isPending} disabled={!dbs.data?.length} onClick={() => pma.mutate()}>
            Open phpMyAdmin
          </Button>
        }
      />
      <HelpNote>
        <p>You are signed in automatically. The panel creates a temporary database login that can see only your databases and is removed after two hours.</p>
      </HelpNote>
      <DataTable
        rows={dbs.data}
        loading={dbs.isLoading}
        error={dbs.error}
        rowKey={(d) => d.id}
        search={(d) => d.name}
        empty={{ icon: DatabaseBackup, title: "No databases", description: "Create a database first with the Database Wizard." }}
        columns={[
          { key: "name", header: "Database", sort: (d) => d.name, render: (d) => <Mono className="text-ink">{d.name}</Mono> },
          { key: "size", header: "Size", sort: (d) => d.size_mb, render: (d) => <span className="tabular-nums">{formatMB(d.size_mb)}</span> },
          { key: "users", header: "Users", render: (d) => <span className="text-muted">{d.users.length ? d.users.join(", ") : "none"}</span> },
        ]}
      />
    </>
  );
}

// ---- Indexes ---------------------------------------------------------------------------

interface IndexRow {
  id: string;
  domain_id: string;
  domain: string;
  path: string;
  created_at: number;
}

export function Indexes() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/indexes`;
  const q = useGet<IndexRow[]>(key);
  const { q: doms } = useDomains();
  const sites = doms.data?.filter((d) => d.kind !== "alias") ?? [];
  const [f, setF] = useState({ domain_id: "", path: "" });
  const add = useMutate(() => api.post(key, { domain_id: f.domain_id || sites[0]?.id, path: f.path || "/" }), { invalidate: [key], success: "Index listing turned on", onSuccess: () => setF({ ...f, path: "" }) });
  const remove = useMutate((id: string) => api.del(`${key}/${id}`), { invalidate: [key], success: "Index listing turned off" });
  return (
    <>
      <PageHeader title="Indexes" />
      <HelpNote>
        <p>When someone opens a folder that has no index.html or index.php, the server normally answers “403 Forbidden”. With an index listing it shows the files in the folder instead, which is handy for a downloads folder.</p>
        <p>Turn it on only for folders whose files may be public: listings reveal file names.</p>
      </HelpNote>
      <Card title="Show file listing for a folder" className="mb-4">
        <form className="flex flex-wrap items-end gap-2" onSubmit={(e) => (e.preventDefault(), add.mutate())}>
          <Field label="Domain" className="w-56">
            {(id) => (
              <Select id={id} value={f.domain_id || sites[0]?.id} onChange={(e) => setF({ ...f, domain_id: e.target.value })}>
                {sites.map((d) => (
                  <option key={d.id} value={d.id}>
                    {d.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Folder (URL path)" className="w-64">
            {(id) => <Input id={id} className="font-mono" value={f.path} onChange={(e) => setF({ ...f, path: e.target.value })} placeholder="/downloads" />}
          </Field>
          <Button type="submit" variant="primary" icon={Plus} loading={add.isPending}>
            Turn on listing
          </Button>
        </form>
      </Card>
      <DataTable
        rows={q.data}
        loading={q.isLoading}
        error={q.error}
        rowKey={(r) => r.id}
        search={(r) => `${r.domain}${r.path}`}
        empty={{ icon: FolderTree, title: "No listings", description: "Every folder without an index page answers 403 Forbidden." }}
        columns={[
          { key: "url", header: "Folder", sort: (r) => r.domain + r.path, render: (r) => <Mono className="text-ink">{r.domain}{r.path}</Mono> },
          { key: "mode", header: "Setting", render: () => <Badge tone="accent">Show file list</Badge> },
          { key: "created", header: "Since", sort: (r) => r.created_at, render: (r) => <span className="text-muted">{formatDate(r.created_at)}</span>, hideSm: true },
        ]}
        actions={(r) => (
          <Button size="sm" variant="ghost" loading={remove.isPending && remove.variables === r.id} onClick={() => remove.mutate(r.id)}>
            Turn off
          </Button>
        )}
      />
    </>
  );
}

// ---- MIME Types -------------------------------------------------------------------------

interface MimeRow {
  id: string;
  ext: string;
  type: string;
  created_at: number;
}

const commonMime = [
  ["webmanifest", "application/manifest+json"],
  ["wasm", "application/wasm"],
  ["glb", "model/gltf-binary"],
  ["avif", "image/avif"],
  ["mjs", "text/javascript"],
  ["vtt", "text/vtt"],
];

export function MimeTypes() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/mime-types`;
  const q = useGet<MimeRow[]>(key);
  const [f, setF] = useState({ ext: "", type: "" });
  const add = useMutate(() => api.post(key, f), { invalidate: [key], success: "MIME type added", onSuccess: () => setF({ ext: "", type: "" }) });
  const remove = useMutate((id: string) => api.del(`${key}/${id}`), { invalidate: [key], success: "MIME type removed" });
  return (
    <>
      <PageHeader title="MIME Types" />
      <HelpNote>
        <p>A MIME type tells the browser what a file is, so it can show, play or download it. The server knows all common types; add one only for an unusual extension that downloads or displays wrongly.</p>
        <p>Script extensions such as .php cannot be changed here.</p>
      </HelpNote>
      <Card title="Add a MIME type" className="mb-4">
        <form className="flex flex-wrap items-end gap-2" onSubmit={(e) => (e.preventDefault(), add.mutate())}>
          <Field label="Extension" className="w-40">{(id) => <Input id={id} className="font-mono" value={f.ext} onChange={(e) => setF({ ...f, ext: e.target.value })} placeholder="webmanifest" />}</Field>
          <Field label="MIME type" className="w-72">{(id) => <Input id={id} className="font-mono" value={f.type} onChange={(e) => setF({ ...f, type: e.target.value })} placeholder="application/manifest+json" />}</Field>
          <Button type="submit" variant="primary" icon={Plus} loading={add.isPending} disabled={!f.ext || !f.type}>
            Add
          </Button>
        </form>
        <div className="mt-3 flex flex-wrap items-center gap-1.5 text-[12px] text-muted">
          Common:
          {commonMime.map(([e, t]) => (
            <button key={e} type="button" onClick={() => setF({ ext: e!, type: t! })} className="rounded border border-line px-1.5 py-0.5 font-mono hover:border-accent hover:text-accent">
              .{e}
            </button>
          ))}
        </div>
      </Card>
      <DataTable
        rows={q.data}
        loading={q.isLoading}
        error={q.error}
        rowKey={(r) => r.id}
        search={(r) => `${r.ext} ${r.type}`}
        empty={{ icon: FileType2, title: "No custom MIME types", description: "The server's built-in types apply." }}
        columns={[
          { key: "ext", header: "Extension", sort: (r) => r.ext, render: (r) => <Mono className="text-ink">.{r.ext}</Mono> },
          { key: "type", header: "MIME type", sort: (r) => r.type, render: (r) => <Mono>{r.type}</Mono> },
        ]}
        actions={(r) => <IconButton icon={Trash2} label={`Remove .${r.ext}`} onClick={() => remove.mutate(r.id)} />}
      />
    </>
  );
}

// ---- Optimize Website ----------------------------------------------------------------------

export function Optimize() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/optimize`;
  const q = useGet<{ mode: "off" | "all" | "custom"; types: string[]; defaults: string[] }>(key);
  const [mode, setMode] = useState<"off" | "all" | "custom">("off");
  const [types, setTypes] = useState("");
  useEffect(() => {
    if (q.data) {
      setMode(q.data.mode);
      setTypes((q.data.types.length ? q.data.types : q.data.defaults).join(" "));
    }
  }, [q.data]);
  const save = useMutate(() => api.put(key, { mode, types: mode === "custom" ? types.split(/\s+/).filter(Boolean) : [] }), { invalidate: [key], success: "Compression settings applied" });
  if (q.isLoading) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  const opt = (id: typeof mode, title: string, desc: string) => (
    <label className={`flex cursor-pointer items-start gap-3 rounded-lg border p-3 ${mode === id ? "border-accent bg-accent-soft/40" : "border-line"}`}>
      <input type="radio" name="gzip" className="mt-0.5 accent-[var(--accent)]" checked={mode === id} onChange={() => setMode(id)} />
      <span>
        <span className="block text-[13px] font-medium text-ink">{title}</span>
        <span className="block text-[12.5px] text-muted">{desc}</span>
      </span>
    </label>
  );
  return (
    <>
      <PageHeader title="Optimize Website" />
      <Card className="max-w-2xl" title="Compress content" description="The server compresses text files (HTML, CSS, JavaScript, JSON, SVG) before sending them. Pages usually get 60–80% smaller and load faster; images and videos are already compressed and are left alone.">
        <div className="space-y-2.5">
          {opt("off", "Disabled", "Send files as they are.")}
          {opt("all", "Compress all content", "Recommended: HTML, CSS, JavaScript, JSON, XML, SVG and fonts.")}
          {opt("custom", "Compress the specified MIME types", "Only the types you list, separated by spaces.")}
          {mode === "custom" && <Textarea aria-label="MIME types to compress" rows={3} value={types} onChange={(e) => setTypes(e.target.value)} />}
        </div>
        <div className="mt-4 flex items-center gap-3">
          <Button variant="primary" icon={Zap} loading={save.isPending} onClick={() => save.mutate()}>
            Update settings
          </Button>
          <span className="text-[12px] text-muted">Applies to every domain of the account.</span>
        </div>
      </Card>
    </>
  );
}

// ---- Track DNS ------------------------------------------------------------------------------

export function TrackDns() {
  const aid = useAccountId();
  const [tab, setTab] = useState<"lookup" | "trace">("lookup");
  const [host, setHost] = useState("");
  const [type, setType] = useState("A");
  const lookup = useMutate(() => api.post<{ host: string; type: string; records: string[]; message: string }>(`/accounts/${aid}/dns-lookup`, { host, type }));
  const trace = useMutate(() => api.post<{ output: string }>(`/accounts/${aid}/traceroute`, { host }));
  return (
    <>
      <PageHeader title="Track DNS" />
      <Tabs
        tabs={[
          { id: "lookup", label: "Domain lookup" },
          { id: "trace", label: "Trace route" },
        ]}
        value={tab}
        onChange={setTab}
      />
      <Card className="mb-4">
        <form className="flex flex-wrap items-end gap-2" onSubmit={(e) => (e.preventDefault(), tab === "lookup" ? lookup.mutate() : trace.mutate())}>
          <Field label="Host name or IP address" className="w-72">{(id) => <Input id={id} value={host} onChange={(e) => setHost(e.target.value)} placeholder="example.com" autoFocus />}</Field>
          {tab === "lookup" && (
            <Field label="Record type" className="w-32">
              {(id) => (
                <Select id={id} value={type} onChange={(e) => setType(e.target.value)}>
                  {["A", "AAAA", "MX", "TXT", "NS", "CNAME", "PTR"].map((t) => (
                    <option key={t}>{t}</option>
                  ))}
                </Select>
              )}
            </Field>
          )}
          <Button type="submit" variant="primary" icon={Radar} loading={lookup.isPending || trace.isPending} disabled={!host}>
            {tab === "lookup" ? "Look up" : "Trace"}
          </Button>
        </form>
        <p className="mt-2 text-[12px] text-muted">{tab === "lookup" ? "Asks the server's resolver, so you see what the server sees, which is useful right after changing records." : "Shows the network path from this server to the host. Can take up to 45 seconds."}</p>
      </Card>
      {tab === "lookup" && lookup.data && (
        <Card title={`${lookup.data.type} records for ${lookup.data.host}`}>
          {lookup.data.records.length ? (
            <ul className="space-y-1">
              {lookup.data.records.map((r) => (
                <li key={r}>
                  <Mono className="break-all text-ink">{r}</Mono>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-[13px] text-muted">{lookup.data.message || "No records."}</p>
          )}
        </Card>
      )}
      {tab === "trace" && trace.data && (
        <Card title={`Route to ${host}`}>
          <pre className="overflow-x-auto whitespace-pre font-mono text-[12px] text-ink">{trace.data.output}</pre>
        </Card>
      )}
    </>
  );
}

// ---- Contact Information ---------------------------------------------------------------------

const noticeLabels: Record<string, [string, string]> = {
  disk_quota: ["Disk space", "When the account uses 80% of its disk space."],
  bandwidth: ["Bandwidth", "When the account uses 80% of its monthly bandwidth."],
};

export function ContactInfo() {
  const q = useGet<{ email: string; secondary_email: string; notify: Record<string, boolean> }>("/me/contact");
  const [f, setF] = useState<{ email: string; secondary_email: string; notify: Record<string, boolean> } | null>(null);
  useEffect(() => {
    if (q.data) setF(q.data);
  }, [q.data]);
  const save = useMutate(() => api.put("/me/contact", f), { invalidate: ["/me/contact", "/me"], success: "Contact information saved" });
  if (q.isLoading || !f) return <Loading />;
  return (
    <>
      <PageHeader title="Contact Information" />
      <div className="grid max-w-4xl gap-4 lg:grid-cols-2">
        <Card title="Addresses">
          <div className="space-y-4">
            <Field label="Contact email" hint="Notices and password resets go here.">{(id) => <Input id={id} type="email" value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} />}</Field>
            <Field label="Second address (optional)" hint="Gets a copy of every notice.">{(id) => <Input id={id} type="email" value={f.secondary_email} onChange={(e) => setF({ ...f, secondary_email: e.target.value })} />}</Field>
          </div>
        </Card>
        <Card title="Email me when…">
          <ul className="space-y-3">
            {Object.entries(noticeLabels).map(([k, [t, d]]) => (
              <li key={k} className="flex items-start justify-between gap-4">
                <div className="text-[13px]">
                  <div className="font-medium text-ink">{t}</div>
                  <div className="text-muted">{d}</div>
                </div>
                <Switch label={t} checked={!!f.notify[k]} onChange={(v) => setF({ ...f, notify: { ...f.notify, [k]: v } })} />
              </li>
            ))}
          </ul>
        </Card>
      </div>
      <Button className="mt-4" variant="primary" loading={save.isPending} onClick={() => save.mutate()}>
        Save
      </Button>
    </>
  );
}
