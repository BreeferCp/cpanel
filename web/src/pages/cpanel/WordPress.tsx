import {
  AppWindow,
  Archive,
  CheckCircle2,
  Copy,
  ExternalLink,
  KeyRound,
  Plus,
  RefreshCw,
  RotateCcw,
  ScanSearch,
  ShieldCheck,
  Trash2,
  Upload,
  Wrench,
  XCircle,
} from "lucide-react";
import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api } from "@/lib/api";
import { formatBytes, formatDate, timeAgo } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { Domain, InstallBackup, Installation, WPCheck, WPInfo, WPItem, WPSite } from "@/lib/types";
import { useAccountId, useBase } from "@/state/scope";
import { Button, IconButton } from "@/components/ui/Button";
import { Field, Input, Select, Switch } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, PageHeader, Table, Tabs, Td, Th, Tr } from "@/components/ui/Layout";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

type Tab = "overview" | "plugins" | "themes" | "security" | "backups" | "staging";

export default function WordPress() {
  const aid = useAccountId();
  const base = useBase();
  const [params, setParams] = useSearchParams();
  const key = `/accounts/${aid}/installations`;
  const installs = useGet<Installation[]>(key, true, (d) => (d?.some((i) => i.status === "installing") ? 3000 : false));
  const all = installs.data?.filter((i) => i.app === "wordpress") ?? [];
  const sites = all.filter((i) => i.status === "installed");
  const selected = sites.find((s) => s.id === params.get("id")) ?? sites[0];
  const scan = useMutate(() => api.post<{ found: number; added: string[] }>(`/accounts/${aid}/wordpress/scan`), {
    invalidate: [key],
    success: (r) => (r.added.length ? `Added ${r.added.length} site(s)` : r.found ? "All sites are already listed" : "No WordPress sites found"),
  });

  if (installs.isLoading) return <Loading />;
  return (
    <>
      <PageHeader
        title="WordPress Toolkit"
        description="Updates, security, backups, staging and cloning for every WordPress site in your account."
        actions={
          <>
            <Button icon={ScanSearch} loading={scan.isPending} onClick={() => scan.mutate()}>
              Scan for sites
            </Button>
            <Link to={`${base}/installer`}>
              <Button variant="primary" icon={Plus}>
                Install WordPress
              </Button>
            </Link>
          </>
        }
      />
      {!sites.length ? (
        <Card>
          <EmptyState
            icon={AppWindow}
            title={all.length ? "Your site is being prepared" : "No WordPress sites"}
            description="Install WordPress from the App Installer, or scan your home directory for sites installed another way."
          />
        </Card>
      ) : (
        <div className="grid gap-5 lg:grid-cols-[240px_1fr]">
          <nav className="space-y-1" aria-label="WordPress sites">
            {all.map((s) => (
              <button
                key={s.id}
                disabled={s.status !== "installed"}
                onClick={() => setParams({ id: s.id })}
                className={`block w-full rounded-md px-3 py-2 text-left text-[13px] disabled:opacity-60 ${s.id === selected?.id ? "bg-accent-soft font-medium text-accent" : "text-ink hover:bg-surface2"}`}
              >
                <span className="block truncate">{s.url.replace(/^https?:\/\//, "")}</span>
                <span className="text-[11.5px] text-muted">{s.status === "installed" ? `WordPress ${s.version || ""}` : s.status === "failed" ? "Failed" : "Working…"}</span>
              </button>
            ))}
          </nav>
          {selected && <Site key={selected.id} aid={aid} site={selected} />}
        </div>
      )}
    </>
  );
}

function Site({ aid, site }: { aid: string; site: Installation }) {
  const [tab, setTab] = useState<Tab>("overview");
  const base = `/accounts/${aid}/installations/${site.id}`;
  const key = `${base}/wordpress`;
  const q = useGet<WPInfo>(key);
  const meta = useGet<WPSite>(`${key}/site`);
  const [reset, setReset] = useState(false);

  if (q.isLoading) return <Loading />;
  if (q.error || !q.data) return <ErrorNote error={q.error} />;
  const info = q.data;
  const pluginUpdates = info.plugins.filter((p) => p.update === "available").length;
  const themeUpdates = info.themes.filter((p) => p.update === "available").length;
  const isStaging = !!meta.data?.policy.staging_of;

  return (
    <div className="min-w-0 space-y-4">
      <Card
        title={site.url.replace(/^https?:\/\//, "")}
        description={site.path}
        actions={
          <>
            {isStaging && <Badge tone="warn">Staging</Badge>}
            <a href={`${site.url}/wp-admin/`} target="_blank" rel="noreferrer noopener">
              <Button size="sm" icon={ExternalLink}>
                wp-admin
              </Button>
            </a>
            <Button size="sm" icon={KeyRound} onClick={() => setReset(true)}>
              Reset password
            </Button>
          </>
        }
      >
        <Tabs<Tab>
          value={tab}
          onChange={setTab}
          tabs={[
            { id: "overview", label: "Overview" },
            { id: "plugins", label: `Plugins${pluginUpdates ? ` (${pluginUpdates})` : ""}` },
            { id: "themes", label: `Themes${themeUpdates ? ` (${themeUpdates})` : ""}` },
            { id: "security", label: "Security" },
            { id: "backups", label: "Backups" },
            { id: "staging", label: "Clone & staging" },
          ]}
        />
      </Card>
      {tab === "overview" && <Overview base={base} info={info} meta={meta.data} />}
      {tab === "plugins" && <Items base={base} kind="plugin" items={info.plugins} />}
      {tab === "themes" && <Items base={base} kind="theme" items={info.themes} />}
      {tab === "security" && <Security base={base} />}
      {tab === "backups" && <Backups base={base} />}
      {tab === "staging" && <Staging aid={aid} base={base} site={site} isStaging={isStaging} />}
      <ResetPassword open={reset} onClose={() => setReset(false)} wpKey={key} defaultUser={meta.data?.site.admins?.[0] ?? site.admin_user} />
    </div>
  );
}

function useWPAction(key: string) {
  return useMutate((v: { action: string; target?: string; password?: string }) => api.post(key, v), {
    invalidate: [key],
    success: (_r, v) => (v.action === "reset-password" ? "Password updated" : "Done"),
  });
}

function Overview({ base, info, meta }: { base: string; info: WPInfo; meta?: WPSite }) {
  const key = `${base}/wordpress`;
  const act = useWPAction(key);
  const busy = (a: string) => act.isPending && act.variables?.action === a;
  const setting = useMutate((v: { name: string; on: boolean }) => api.put(`${key}/settings`, v), { invalidate: [`${key}/site`], success: "Saved" });
  const policy = useMutate((v: Partial<WPSite["policy"]>) => api.put(`${base}/autoupdate`, { ...meta?.policy, ...v }), {
    invalidate: [`${key}/site`],
    success: "Update policy saved",
  });
  const smart = useMutate(() => api.post(`${base}/update-now`), {
    invalidate: [`${key}/site`, key],
    success: "Smart update started: a backup is taken first and restored automatically if the site breaks",
  });
  const p = meta?.policy;
  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <Card title="WordPress core">
        <div className="flex flex-wrap items-center gap-3">
          <span className="text-[22px] font-semibold tabular-nums">{info.version}</span>
          {info.update_version ? <Badge tone="warn">{info.update_version} available</Badge> : <Badge tone="ok">Up to date</Badge>}
        </div>
        <div className="mt-3 flex flex-wrap gap-2">
          <Button size="sm" variant="primary" icon={RefreshCw} loading={smart.isPending} onClick={() => smart.mutate()}>
            Smart update now
          </Button>
          {info.update_version && (
            <Button size="sm" loading={busy("core-update")} onClick={() => act.mutate({ action: "core-update" })}>
              Update core only
            </Button>
          )}
          <Button size="sm" variant="ghost" icon={Wrench} loading={busy("cache-flush")} onClick={() => act.mutate({ action: "cache-flush" })}>
            Flush cache
          </Button>
        </div>
        {p?.last_update_at ? (
          <p className="mt-3 text-[12.5px] text-muted">
            Last update {timeAgo(p.last_update_at)}: {p.last_update_result}
          </p>
        ) : null}
      </Card>
      <Card title="Automatic updates" description="Run nightly. Each run takes a backup, updates, checks that the home page still loads, and rolls back if it does not.">
        <div className="space-y-3">
          <Field label="WordPress core">
            {(id) => (
              <Select id={id} value={p?.auto_core ?? "minor"} onChange={(e) => policy.mutate({ auto_core: e.target.value as WPSite["policy"]["auto_core"] })}>
                <option value="off">Off</option>
                <option value="minor">Security and minor releases</option>
                <option value="major">All releases</option>
              </Select>
            )}
          </Field>
          <Toggle label="Plugins" checked={!!p?.auto_plugins} onChange={(v) => policy.mutate({ auto_plugins: v })} />
          <Toggle label="Themes" checked={!!p?.auto_themes} onChange={(v) => policy.mutate({ auto_themes: v })} />
        </div>
      </Card>
      <Card title="Site settings">
        <div className="space-y-3">
          <Toggle
            label="Maintenance mode"
            hint="Visitors see a short maintenance notice."
            checked={info.maintenance}
            onChange={(v) => act.mutate({ action: v ? "maintenance-on" : "maintenance-off" })}
          />
          <Toggle label="Search engine indexing" hint="Off asks search engines not to index the site." checked={!!meta?.site.indexing} onChange={(on) => setting.mutate({ name: "indexing", on })} />
          <Toggle label="Debug mode" hint="Writes PHP notices to wp-content/debug.log. Keep off on live sites." checked={!!meta?.site.debug} onChange={(on) => setting.mutate({ name: "debug", on })} />
        </div>
      </Card>
      <Card title="Details">
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-[13px]">
          <dt className="text-muted">Site URL</dt>
          <dd className="truncate font-mono text-[12.5px]">{meta?.site.site_url}</dd>
          <dt className="text-muted">Database</dt>
          <dd className="font-mono text-[12.5px]">{meta?.site.db_name}</dd>
          <dt className="text-muted">Table prefix</dt>
          <dd className="font-mono text-[12.5px]">{meta?.site.table_prefix}</dd>
          <dt className="text-muted">Administrators</dt>
          <dd>{meta?.site.admins?.join(", ") || "—"}</dd>
        </dl>
      </Card>
    </div>
  );
}

function Toggle({ label, hint, checked, onChange }: { label: string; hint?: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <div>
        <div className="text-[13px] font-medium">{label}</div>
        {hint && <div className="text-[12.5px] text-muted">{hint}</div>}
      </div>
      <Switch checked={checked} onChange={onChange} label={label} />
    </div>
  );
}

function Items({ base, kind, items }: { base: string; kind: "plugin" | "theme"; items: WPItem[] }) {
  const key = `${base}/wordpress`;
  const act = useWPAction(key);
  const busy = (a: string, t?: string) => act.isPending && act.variables?.action === a && act.variables?.target === t;
  const [slug, setSlug] = useState("");
  const install = useMutate(() => api.post(`${key}/install-item`, { kind, slug, activate: kind === "plugin" }), {
    invalidate: [key],
    success: `${kind === "plugin" ? "Plugin" : "Theme"} installed`,
    onSuccess: () => setSlug(""),
  });
  const updates = items.filter((i) => i.update === "available").length;
  return (
    <Card
      flush
      title={kind === "plugin" ? "Plugins" : "Themes"}
      actions={
        updates > 0 && (
          <Button size="sm" loading={busy(`${kind}s-update-all`)} onClick={() => act.mutate({ action: `${kind}s-update-all` })}>
            Update all ({updates})
          </Button>
        )
      }
    >
      <form
        className="flex flex-wrap items-end gap-2 border-b border-line p-4"
        onSubmit={(e) => {
          e.preventDefault();
          install.mutate();
        }}
      >
        <Field label={`Install a ${kind} from wordpress.org`} hint="Use the slug from its address, e.g. wordpress-seo" className="min-w-60 flex-1">
          {(id) => <Input id={id} className="font-mono" value={slug} onChange={(e) => setSlug(e.target.value.trim())} placeholder={kind === "plugin" ? "wordpress-seo" : "astra"} />}
        </Field>
        <Button type="submit" icon={Plus} loading={install.isPending} disabled={!slug}>
          Install
        </Button>
      </form>
      <Table>
        <thead>
          <tr>
            <Th>{kind === "plugin" ? "Plugin" : "Theme"}</Th>
            <Th>Version</Th>
            <Th>Status</Th>
            <Th className="text-right">Actions</Th>
          </tr>
        </thead>
        <tbody>
          {items.map((p) => (
            <Tr key={p.name}>
              <Td>
                <div className="font-medium text-ink">{p.title || p.name}</div>
                <div className="font-mono text-[11.5px] text-subtle">{p.name}</div>
              </Td>
              <Td className="tabular-nums text-muted">
                {p.version}
                {p.update === "available" && (
                  <span className="ml-2">
                    <Badge tone="warn">update</Badge>
                  </span>
                )}
              </Td>
              <Td>
                <Badge tone={p.status === "active" ? "ok" : "neutral"}>{p.status}</Badge>
              </Td>
              <Td>
                <div className="flex justify-end gap-1.5">
                  {p.update === "available" && (
                    <Button size="sm" loading={busy(`${kind}-update`, p.name)} onClick={() => act.mutate({ action: `${kind}-update`, target: p.name })}>
                      Update
                    </Button>
                  )}
                  {kind === "plugin" ? (
                    <>
                      <Button
                        size="sm"
                        variant="ghost"
                        loading={busy(p.status === "active" ? "plugin-deactivate" : "plugin-activate", p.name)}
                        onClick={() => act.mutate({ action: p.status === "active" ? "plugin-deactivate" : "plugin-activate", target: p.name })}
                      >
                        {p.status === "active" ? "Deactivate" : "Activate"}
                      </Button>
                      {p.status !== "active" && (
                        <IconButton icon={Trash2} label={`Delete ${p.name}`} onClick={() => act.mutate({ action: "plugin-delete", target: p.name })} />
                      )}
                    </>
                  ) : (
                    p.status !== "active" && (
                      <Button size="sm" variant="ghost" onClick={() => act.mutate({ action: "theme-activate", target: p.name })}>
                        Activate
                      </Button>
                    )
                  )}
                </div>
              </Td>
            </Tr>
          ))}
        </tbody>
      </Table>
    </Card>
  );
}

function Security({ base }: { base: string }) {
  const key = `${base}/wordpress/security`;
  const q = useGet<WPCheck[]>(key);
  const fix = useMutate((id: string) => api.post(`${key}/${id}`), { invalidate: [key], success: "Fixed" });
  if (q.isLoading) return <Loading />;
  if (q.error || !q.data) return <ErrorNote error={q.error} />;
  const bad = q.data.filter((c) => !c.ok);
  const fixable = bad.filter((c) => c.fixable);
  return (
    <Card
      flush
      title="Security check"
      description={bad.length ? `${bad.length} of ${q.data.length} checks need attention` : "All checks passed"}
      actions={
        fixable.length > 1 && (
          <Button size="sm" variant="primary" icon={ShieldCheck} onClick={() => fixable.forEach((c) => fix.mutate(c.id))}>
            Fix all ({fixable.length})
          </Button>
        )
      }
    >
      <ul className="divide-y divide-[var(--line)]">
        {q.data.map((c) => (
          <li key={c.id} className="flex items-center gap-3 px-4 py-3">
            {c.ok ? <CheckCircle2 className="size-4 shrink-0 text-ok" aria-label="Passed" /> : <XCircle className="size-4 shrink-0 text-warn" aria-label="Needs attention" />}
            <div className="min-w-0 flex-1">
              <div className="text-[13px] text-ink">{c.label}</div>
              {!c.ok && c.detail && <div className="text-[12.5px] text-muted">{c.detail}</div>}
            </div>
            {!c.ok && c.fixable && (
              <Button size="sm" loading={fix.isPending && fix.variables === c.id} onClick={() => fix.mutate(c.id)}>
                Fix
              </Button>
            )}
          </li>
        ))}
      </ul>
    </Card>
  );
}

function Backups({ base }: { base: string }) {
  const key = `${base}/backups`;
  const q = useGet<InstallBackup[]>(key, true, (d) => (d?.some((b) => b.status === "running") ? 3000 : false));
  const [restore, setRestore] = useState<InstallBackup | null>(null);
  const create = useMutate(() => api.post(key), { invalidate: [key], success: "Backup started" });
  const doRestore = useMutate((id: string) => api.post(`${key}/${id}/restore`), {
    invalidate: [key],
    success: "Site restored",
    onSuccess: () => setRestore(null),
  });
  const remove = useMutate((id: string) => api.del(`${key}/${id}`), { invalidate: [key], success: "Backup deleted" });
  const reasons: Record<string, string> = { manual: "Manual", "pre-update": "Before update", "pre-restore": "Before restore", "pre-push": "Before push from staging" };
  return (
    <Card
      flush
      title="Backups"
      description="Files and database of this site. Automatic backups are also taken before every update, restore and push."
      actions={
        <Button size="sm" variant="primary" icon={Archive} loading={create.isPending} onClick={() => create.mutate()}>
          Back up now
        </Button>
      }
    >
      {q.isLoading ? (
        <Loading />
      ) : !q.data?.length ? (
        <EmptyState icon={Archive} title="No backups yet" />
      ) : (
        <Table>
          <thead>
            <tr>
              <Th>Created</Th>
              <Th>Type</Th>
              <Th>Size</Th>
              <Th className="text-right">Actions</Th>
            </tr>
          </thead>
          <tbody>
            {q.data.map((b) => (
              <Tr key={b.id}>
                <Td>{formatDate(b.created_at)}</Td>
                <Td>
                  {reasons[b.reason] ?? b.reason}
                  {b.status !== "complete" && (
                    <span className="ml-2">
                      <Badge tone={b.status === "failed" ? "bad" : "neutral"}>{b.status}</Badge>
                    </span>
                  )}
                </Td>
                <Td className="tabular-nums text-muted">{b.size_bytes ? formatBytes(b.size_bytes) : "—"}</Td>
                <Td>
                  <div className="flex justify-end gap-1">
                    <Button size="sm" icon={RotateCcw} disabled={b.status !== "complete"} onClick={() => setRestore(b)}>
                      Restore
                    </Button>
                    <IconButton icon={Trash2} label="Delete backup" onClick={() => remove.mutate(b.id)} />
                  </div>
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      )}
      <ConfirmDialog
        open={!!restore}
        onClose={() => setRestore(null)}
        onConfirm={() => restore && doRestore.mutate(restore.id)}
        loading={doRestore.isPending}
        confirmLabel="Restore"
        title="Restore this backup?"
        message={<>The site's files and database are replaced with the backup from {restore && formatDate(restore.created_at)}. A safety backup of the current state is taken first.</>}
      />
    </Card>
  );
}

function Staging({ aid, base, site, isStaging }: { aid: string; base: string; site: Installation; isStaging: boolean }) {
  const domains = useGet<Domain[]>(`/accounts/${aid}/domains`);
  const [mode, setMode] = useState<"staging" | "clone" | null>(null);
  const [f, setF] = useState({ domain_id: "", directory: "staging" });
  const [push, setPush] = useState(false);
  const clone = useMutate(() => api.post(`${base}/clone`, { ...f, domain_id: f.domain_id || domains.data?.[0]?.id, staging: mode === "staging" }), {
    invalidate: [`/accounts/${aid}/installations`],
    success: mode === "staging" ? "Staging copy is being created" : "Clone is being created",
    onSuccess: () => setMode(null),
  });
  const doPush = useMutate(() => api.post(`${base}/push`), {
    success: "Pushing to live: a backup of the live site is taken first",
    onSuccess: () => setPush(false),
  });
  return (
    <div className="grid gap-4 md:grid-cols-2">
      {isStaging ? (
        <Card title="Push to live" description="Copy this staging site's files and database over the live site. The live site keeps its own wp-config.php and database credentials.">
          <Button variant="primary" icon={Upload} onClick={() => setPush(true)}>
            Push to live
          </Button>
        </Card>
      ) : (
        <Card title="Staging" description="A private copy for testing updates and changes. Search engines are asked not to index it. When it looks right, push it back to live.">
          <Button variant="primary" icon={Copy} onClick={() => (setMode("staging"), setF({ ...f, directory: "staging" }))}>
            Create staging copy
          </Button>
        </Card>
      )}
      <Card title="Clone" description="An independent copy on another domain or folder, with its own database. URLs inside the copy are rewritten.">
        <Button icon={Copy} onClick={() => (setMode("clone"), setF({ ...f, directory: "" }))}>
          Clone site
        </Button>
      </Card>
      <Modal
        open={!!mode}
        onClose={() => setMode(null)}
        title={mode === "staging" ? "Create staging copy" : "Clone site"}
        description={`From ${site.url}`}
        footer={
          <>
            <Button onClick={() => setMode(null)}>Cancel</Button>
            <Button variant="primary" loading={clone.isPending} onClick={() => clone.mutate()}>
              {mode === "staging" ? "Create staging" : "Clone"}
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="Domain" hint="Tip: create a subdomain such as staging.yourdomain.com first under Domains.">
            {(id) => (
              <Select id={id} value={f.domain_id || domains.data?.[0]?.id} onChange={(e) => setF({ ...f, domain_id: e.target.value })}>
                {domains.data?.map((d) => (
                  <option key={d.id} value={d.id}>
                    {d.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Folder" hint="Inside the domain's document root; leave empty to use the root.">
            {(id) => <Input id={id} className="font-mono" value={f.directory} onChange={(e) => setF({ ...f, directory: e.target.value })} />}
          </Field>
        </div>
      </Modal>
      <ConfirmDialog
        open={push}
        onClose={() => setPush(false)}
        onConfirm={() => doPush.mutate()}
        loading={doPush.isPending}
        confirmLabel="Push to live"
        title="Replace the live site?"
        message="The live site's files and database are replaced with this staging copy. A backup of the live site is taken first and appears in its Backups tab."
      />
    </div>
  );
}

function ResetPassword({ open, onClose, wpKey, defaultUser }: { open: boolean; onClose: () => void; wpKey: string; defaultUser: string }) {
  const [pw, setPw] = useState({ user: "", password: "" });
  const act = useMutate((v: { action: string; target?: string; password?: string }) => api.post(wpKey, v), { success: "Password updated", onSuccess: onClose });
  const user = pw.user || defaultUser;
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Reset WordPress password"
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" loading={act.isPending} disabled={pw.password.length < 12} onClick={() => act.mutate({ action: "reset-password", target: user, password: pw.password })}>
            Set password
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="WordPress username">{(id) => <Input id={id} value={user} onChange={(e) => setPw({ ...pw, user: e.target.value })} />}</Field>
        <Field label="New password" hint="At least 12 characters.">
          {(id) => <Input id={id} type="password" value={pw.password} onChange={(e) => setPw({ ...pw, password: e.target.value })} autoComplete="new-password" />}
        </Field>
      </div>
    </Modal>
  );
}
