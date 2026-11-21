import { ArrowRightLeft, CheckCircle2, Globe, KeyRound, LogIn, Pause, Play, Trash2, UserPlus, Users } from "lucide-react";
import { useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api } from "@/lib/api";
import { formatDate, formatMB, timeAgo } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { Account, Pkg } from "@/lib/types";
import { Button, IconButton } from "@/components/ui/Button";
import { CopyField } from "@/components/ui/CopyField";
import { DataTable, type Column } from "@/components/ui/DataTable";
import { Field, Input, Select } from "@/components/ui/Form";
import { Badge, Card, Loading, PageHeader, Stat } from "@/components/ui/Layout";
import { Modal } from "@/components/ui/Modal";
import { ModifyAccount } from "./ModifyAccount";

const statusBadge = (a: Account) => <Badge tone={a.status === "active" ? "ok" : a.status === "suspended" ? "warn" : "neutral"}>{a.status}</Badge>;

/** Columns shared by the account tables. */
function accountColumns(): Column<Account>[] {
  return [
    { key: "domain", header: "Domain", sort: (a) => a.primary_domain, render: (a) => <Link to={`/accounts/${a.id}`} className="font-medium text-ink hover:underline">{a.primary_domain}</Link> },
    { key: "user", header: "User", sort: (a) => a.username, render: (a) => <span className="font-mono text-[12.5px]">{a.username}</span> },
    { key: "package", header: "Package", sort: (a) => a.package_name, render: (a) => a.package_name },
    { key: "owner", header: "Owner", sort: (a) => a.owner_name, render: (a) => <span className="text-muted">{a.owner_name || "root"}</span>, hideSm: true },
    { key: "disk", header: "Disk", sort: (a) => a.disk_used_mb, render: (a) => <span className="tabular-nums">{formatMB(a.disk_used_mb)}</span>, hideSm: true },
    { key: "status", header: "Status", sort: (a) => a.status, render: statusBadge },
  ];
}

const accountSearch = (a: Account) => `${a.primary_domain} ${a.username} ${a.email} ${a.package_name} ${a.owner_name}`;

// ---- Create a New Account -------------------------------------------------------------

export function CreateAccount() {
  const pkgs = useGet<Pkg[]>("/packages");
  const nav = useNavigate();
  const [f, setF] = useState({ domain: "", username: "", email: "", package_id: "", password: "" });
  const [created, setCreated] = useState<{ account: Account; password?: string } | null>(null);
  const pkg = pkgs.data?.find((p) => p.id === (f.package_id || pkgs.data?.[0]?.id));
  // Suggest a username from the domain, as WHM does.
  const suggest = (d: string) => d.toLowerCase().replace(/[^a-z0-9]/g, "").slice(0, 8);
  const create = useMutate(() => api.post<{ account: Account; password?: string }>("/accounts", { ...f, package_id: f.package_id || pkg?.id, password: f.password || undefined }), {
    invalidate: ["/accounts"],
    success: "Account created",
    onSuccess: setCreated,
  });
  const sso = useMutate((id: string) => api.post<{ path: string; url: string }>(`/accounts/${id}/sso`), { onSuccess: (r) => window.open(r.url || r.path, "_blank", "noopener") });

  if (created)
    return (
      <>
        <PageHeader title="Create a New Account" />
        <Card className="max-w-xl">
          <div className="flex items-start gap-3">
            <CheckCircle2 className="mt-0.5 size-5 text-ok" aria-hidden />
            <div className="min-w-0 flex-1 space-y-3 text-[13px]">
              <p className="font-medium text-ink">{created.account.primary_domain} is ready.</p>
              <p className="text-muted">The Linux user, web root, PHP pool, DNS zone and resource limits were created. Point the domain's nameservers at this server to bring it online.</p>
              <Field label="Username">{() => <CopyField value={created.account.username} />}</Field>
              {created.password && <Field label="Password (shown once)">{() => <CopyField value={created.password!} />}</Field>}
              <div className="flex flex-wrap gap-2 pt-1">
                <Button icon={LogIn} loading={sso.isPending} onClick={() => sso.mutate(created.account.id)}>
                  Open cPanel
                </Button>
                <Button onClick={() => nav(`/accounts/${created.account.id}`)}>Manage account</Button>
                <Button variant="ghost" onClick={() => (setCreated(null), setF({ domain: "", username: "", email: "", package_id: f.package_id, password: "" }))}>
                  Create another
                </Button>
              </div>
            </div>
          </div>
        </Card>
      </>
    );

  return (
    <>
      <PageHeader title="Create a New Account" />
      <form className="grid max-w-5xl gap-4 lg:grid-cols-[1fr_20rem]" onSubmit={(e) => (e.preventDefault(), create.mutate())}>
        <div className="space-y-4">
          <Card title="Domain information">
            <div className="space-y-4">
              <Field label="Domain" hint="The account's primary domain, without www.">
                {(id) => <Input id={id} value={f.domain} onChange={(e) => setF({ ...f, domain: e.target.value.toLowerCase().trim(), username: f.username || "" })} onBlur={() => !f.username && f.domain && setF({ ...f, username: suggest(f.domain) })} placeholder="example.com" autoFocus />}
              </Field>
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label="Username" hint="3–16 lowercase letters or digits. Cannot be changed later.">
                  {(id) => <Input id={id} className="font-mono" value={f.username} onChange={(e) => setF({ ...f, username: e.target.value.toLowerCase() })} autoComplete="off" />}
                </Field>
                <Field label="Contact email">{(id) => <Input id={id} type="email" value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} />}</Field>
              </div>
              <Field label="Password" hint="Leave empty to generate a strong one. It is shown once after creation.">
                {(id) => <Input id={id} type="password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} autoComplete="new-password" />}
              </Field>
            </div>
          </Card>
          <Card title="Package">
            {pkgs.isLoading ? (
              <Loading />
            ) : !pkgs.data?.length ? (
              <p className="text-[13px] text-muted">
                No packages yet. <Link to="/packages/new" className="text-accent hover:underline">Add a package</Link> first.
              </p>
            ) : (
              <Field label="Choose a package">
                {(id) => (
                  <Select id={id} value={f.package_id || pkg?.id} onChange={(e) => setF({ ...f, package_id: e.target.value })}>
                    {pkgs.data.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
            )}
          </Card>
          <Button type="submit" variant="primary" icon={UserPlus} loading={create.isPending} disabled={!f.domain || !f.username || !f.email || !pkg}>
            Create account
          </Button>
        </div>
        {pkg && (
          <Card title={pkg.name} description="What this account gets." className="self-start">
            <dl className="grid grid-cols-[1fr_auto] gap-y-1.5 text-[13px]">
              {(
                [
                  ["Disk", pkg.disk_mb ? formatMB(pkg.disk_mb) : "Unlimited"],
                  ["Bandwidth", pkg.bandwidth_mb ? `${formatMB(pkg.bandwidth_mb)}/mo` : "Unlimited"],
                  ["Domains", pkg.max_domains || "Unlimited"],
                  ["Databases", pkg.max_databases || "Unlimited"],
                  ["Email accounts", pkg.max_mailboxes || "Unlimited"],
                  ["CPU", `${pkg.cpu_percent / 100} core`],
                  ["Memory", formatMB(pkg.memory_mb)],
                  ["PHP", pkg.php_versions.split(",")[0]],
                  ["Shell", pkg.shell_access ? "Allowed" : "No"],
                ] as [string, string | number][]
              ).map(([k, v]) => (
                <div key={k} className="contents">
                  <dt className="text-muted">{k}</dt>
                  <dd className="text-right font-medium text-ink">{v}</dd>
                </div>
              ))}
            </dl>
          </Card>
        )}
      </form>
    </>
  );
}

// ---- Modify an Account --------------------------------------------------------------------

export function ModifyAccountPage() {
  const q = useGet<Account[]>("/accounts");
  const [pick, setPick] = useState<Account | null>(null);
  return (
    <>
      <PageHeader title="Modify an Account" />
      <DataTable
        rows={q.data}
        loading={q.isLoading}
        error={q.error}
        rowKey={(a) => a.id}
        search={accountSearch}
        searchPlaceholder="Find an account"
        empty={{ icon: Users, title: "No accounts" }}
        columns={accountColumns()}
        actions={(a) => (
          <Button size="sm" onClick={() => setPick(a)}>
            Modify
          </Button>
        )}
      />
      <ModifyAccount account={pick} onClose={() => setPick(null)} invalidate={["/accounts"]} />
    </>
  );
}

// ---- Manage Account Suspension / List Suspended ------------------------------------------

export function ManageSuspension({ onlySuspended }: { onlySuspended?: boolean }) {
  const q = useGet<Account[]>("/accounts");
  const [target, setTarget] = useState<Account[] | null>(null);
  const [reason, setReason] = useState("");
  const suspend = useMutate(async () => {
    for (const a of target!) await api.post(`/accounts/${a.id}/suspend`, { reason });
  }, { invalidate: ["/accounts"], success: "Suspended", onSuccess: () => (setTarget(null), setReason("")) });
  const unsuspend = useMutate(async (list: Account[]) => {
    for (const a of list) await api.post(`/accounts/${a.id}/unsuspend`);
  }, { invalidate: ["/accounts"], success: "Reactivated" });
  const rows = onlySuspended ? q.data?.filter((a) => a.status === "suspended") : q.data;
  const cols = accountColumns();
  cols.splice(5, 0, { key: "reason", header: "Reason", render: (a) => <span className="text-muted">{a.suspend_reason || "—"}</span> });
  return (
    <>
      <PageHeader title={onlySuspended ? "List Suspended Accounts" : "Manage Account Suspension"} />
      {!onlySuspended && q.data && (
        <div className="mb-4 grid grid-cols-3 gap-3">
          <Stat label="Accounts" value={q.data.length} />
          <Stat label="Active" value={q.data.filter((a) => a.status === "active").length} tone="ok" />
          <Stat label="Suspended" value={q.data.filter((a) => a.status === "suspended").length} tone={q.data.some((a) => a.status === "suspended") ? "warn" : undefined} />
        </div>
      )}
      <DataTable
        rows={rows}
        loading={q.isLoading}
        error={q.error}
        rowKey={(a) => a.id}
        search={accountSearch}
        searchPlaceholder="Find an account"
        empty={{ icon: onlySuspended ? CheckCircle2 : Users, title: onlySuspended ? "No suspended accounts" : "No accounts" }}
        bulk={[
          { label: "Suspend", icon: Pause, onRun: (r) => setTarget(r.filter((a) => a.status === "active")) },
          { label: "Unsuspend", icon: Play, onRun: (r) => unsuspend.mutate(r.filter((a) => a.status === "suspended")) },
        ]}
        columns={cols}
        actions={(a) =>
          a.status === "suspended" ? (
            <Button size="sm" icon={Play} loading={unsuspend.isPending} onClick={() => unsuspend.mutate([a])}>
              Unsuspend
            </Button>
          ) : (
            <Button size="sm" icon={Pause} onClick={() => setTarget([a])}>
              Suspend
            </Button>
          )
        }
      />
      <Modal
        open={!!target}
        onClose={() => setTarget(null)}
        title={target?.length === 1 ? `Suspend ${target[0]!.primary_domain}?` : `Suspend ${target?.length} accounts?`}
        description="Websites, email, FTP and cron stop, and the customer is signed out. Nothing is deleted."
        footer={
          <>
            <Button onClick={() => setTarget(null)}>Cancel</Button>
            <Button variant="danger" loading={suspend.isPending} disabled={!target?.length} onClick={() => suspend.mutate()}>
              Suspend
            </Button>
          </>
        }
      >
        <Field label="Reason" hint="Shown to the customer in cPanel.">{(id) => <Input id={id} value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Overdue invoice" autoFocus />}</Field>
      </Modal>
    </>
  );
}

// ---- Terminate Accounts ------------------------------------------------------------------

export function TerminateAccounts() {
  const q = useGet<Account[]>("/accounts");
  const [target, setTarget] = useState<Account[] | null>(null);
  const [typed, setTyped] = useState("");
  const [progress, setProgress] = useState<string[]>([]);
  const terminate = useMutate(async () => {
    const done: string[] = [];
    for (const a of target!) {
      await api.del(`/accounts/${a.id}`);
      done.push(a.primary_domain);
      setProgress([...done]);
    }
  }, { invalidate: ["/accounts"], success: "Accounts terminated", onSuccess: () => (setTarget(null), setTyped(""), setProgress([])) });
  const phrase = target?.length === 1 ? target[0]!.username : `terminate ${target?.length}`;
  return (
    <>
      <PageHeader title="Terminate Accounts" />
      <DataTable
        rows={q.data}
        loading={q.isLoading}
        error={q.error}
        rowKey={(a) => a.id}
        search={accountSearch}
        searchPlaceholder="Find accounts to terminate"
        empty={{ icon: Users, title: "No accounts" }}
        bulk={[{ label: "Terminate", icon: Trash2, danger: true, onRun: setTarget }]}
        columns={[...accountColumns(), { key: "created", header: "Created", sort: (a) => a.created_at, render: (a) => <span className="text-muted">{formatDate(a.created_at)}</span>, hideSm: true }]}
        actions={(a) => <IconButton icon={Trash2} label={`Terminate ${a.primary_domain}`} onClick={() => setTarget([a])} />}
      />
      <Modal
        open={!!target}
        onClose={() => !terminate.isPending && setTarget(null)}
        title={target?.length === 1 ? `Terminate ${target[0]!.primary_domain}?` : `Terminate ${target?.length} accounts?`}
        footer={
          <>
            <Button onClick={() => setTarget(null)} disabled={terminate.isPending}>
              Cancel
            </Button>
            <Button variant="danger" loading={terminate.isPending} disabled={typed !== phrase} onClick={() => terminate.mutate()}>
              Terminate permanently
            </Button>
          </>
        }
      >
        <div className="space-y-3 text-[13px]">
          <p className="text-bad">The Linux users, all files, databases, email, DNS zones and backups of these accounts are deleted. This cannot be undone.</p>
          <ul className="max-h-40 overflow-auto rounded-md border border-line bg-surface2 px-3 py-2 font-mono text-[12px]">
            {target?.map((a) => (
              <li key={a.id} className={progress.includes(a.primary_domain) ? "text-muted line-through" : ""}>
                {a.primary_domain} ({a.username})
              </li>
            ))}
          </ul>
          <Field label={`Type “${phrase}” to confirm`}>{(id) => <Input id={id} value={typed} onChange={(e) => setTyped(e.target.value)} autoComplete="off" />}</Field>
        </div>
      </Modal>
    </>
  );
}

// ---- Password Modification -------------------------------------------------------------------

export function PasswordModification() {
  const q = useGet<Account[]>("/accounts");
  const [pick, setPick] = useState<Account | null>(null);
  const [pw, setPw] = useState("");
  const gen = () => {
    const a = new Uint32Array(18);
    crypto.getRandomValues(a);
    const cs = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789-_!";
    setPw(Array.from(a, (n) => cs[n % cs.length]).join(""));
  };
  const save = useMutate(() => api.post(`/accounts/${pick!.id}/password`, { password: pw }), { success: "Password changed. The customer's other sessions were signed out.", onSuccess: () => (setPick(null), setPw("")) });
  return (
    <>
      <PageHeader title="Password Modification" />
      <DataTable
        rows={q.data}
        loading={q.isLoading}
        error={q.error}
        rowKey={(a) => a.id}
        search={accountSearch}
        searchPlaceholder="Find an account"
        empty={{ icon: Users, title: "No accounts" }}
        columns={accountColumns()}
        actions={(a) => (
          <Button size="sm" icon={KeyRound} onClick={() => setPick(a)}>
            Change password
          </Button>
        )}
      />
      <Modal
        open={!!pick}
        onClose={() => setPick(null)}
        title={`New password for ${pick?.username}`}
        description="Used for cPanel. Email accounts and databases keep their own passwords."
        footer={
          <>
            <Button onClick={() => setPick(null)}>Cancel</Button>
            <Button variant="primary" loading={save.isPending} disabled={pw.length < 12} onClick={() => save.mutate()}>
              Change password
            </Button>
          </>
        }
      >
        <Field label="Password" hint="At least 12 characters. Copy it before saving: it is not shown again.">
          {(id) => (
            <div className="flex gap-2">
              <Input id={id} className="font-mono" value={pw} onChange={(e) => setPw(e.target.value)} autoComplete="new-password" autoFocus />
              <Button onClick={gen}>Generate</Button>
            </div>
          )}
        </Field>
      </Modal>
    </>
  );
}

// ---- Upgrade/Downgrade an Account ----------------------------------------------------------------

export function ChangePackage() {
  const q = useGet<Account[]>("/accounts");
  const pkgs = useGet<Pkg[]>("/packages");
  const [target, setTarget] = useState<Account[] | null>(null);
  const [pkg, setPkg] = useState("");
  const change = useMutate(async () => {
    for (const a of target!) if (a.package_id !== pkg) await api.patch(`/accounts/${a.id}`, { package_id: pkg });
  }, { invalidate: ["/accounts"], success: "Package changed", onSuccess: () => setTarget(null) });
  const chosen = pkgs.data?.find((p) => p.id === pkg);
  const over = useMemo(() => (chosen?.disk_mb ? target?.filter((a) => a.disk_used_mb > chosen.disk_mb) ?? [] : []), [chosen, target]);
  return (
    <>
      <PageHeader title="Upgrade/Downgrade an Account" />
      <DataTable
        rows={q.data}
        loading={q.isLoading}
        error={q.error}
        rowKey={(a) => a.id}
        search={accountSearch}
        searchPlaceholder="Find accounts"
        empty={{ icon: Users, title: "No accounts" }}
        bulk={[{ label: "Change package", icon: ArrowRightLeft, onRun: (r) => (setTarget(r), setPkg(r[0]!.package_id)) }]}
        columns={accountColumns()}
        actions={(a) => (
          <Button size="sm" icon={ArrowRightLeft} onClick={() => (setTarget([a]), setPkg(a.package_id))}>
            Change
          </Button>
        )}
      />
      <Modal
        open={!!target}
        onClose={() => setTarget(null)}
        title={target?.length === 1 ? `Package for ${target[0]!.primary_domain}` : `Package for ${target?.length} accounts`}
        description="New limits and PHP versions apply right away."
        footer={
          <>
            <Button onClick={() => setTarget(null)}>Cancel</Button>
            <Button variant="primary" loading={change.isPending} disabled={!pkg} onClick={() => change.mutate()}>
              Change package
            </Button>
          </>
        }
      >
        <div className="space-y-3">
          <Field label="New package">
            {(id) => (
              <Select id={id} value={pkg} onChange={(e) => setPkg(e.target.value)}>
                {pkgs.data?.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name} — {p.disk_mb ? formatMB(p.disk_mb) : "unlimited"} disk
                  </option>
                ))}
              </Select>
            )}
          </Field>
          {over.length > 0 && <p className="rounded-md bg-warn-soft px-3 py-2 text-[12.5px] text-warn">{over.map((a) => a.primary_domain).join(", ")} already use more disk than the new package allows. They keep their files but cannot upload more.</p>}
        </div>
      </Modal>
    </>
  );
}

// ---- List Domains / MultiPHP ------------------------------------------------------------------

export interface ServerDomain {
  id: string;
  name: string;
  kind: string;
  account_id: string;
  user: string;
  owner: string;
  status: string;
  php_version: string;
  doc_root: string;
  ssl_issuer: string;
  ssl_not_after: number;
  created_at: number;
}

const kindTone: Record<string, "accent" | "neutral"> = { primary: "accent", addon: "neutral", subdomain: "neutral", alias: "neutral" };

export function ListDomains() {
  const q = useGet<ServerDomain[]>("/server/domains");
  return (
    <>
      <PageHeader title="List Domains" />
      <DataTable
        rows={q.data}
        loading={q.isLoading}
        error={q.error}
        rowKey={(d) => d.id}
        search={(d) => `${d.name} ${d.user} ${d.owner} ${d.kind}`}
        searchPlaceholder="Search domains or users"
        empty={{ icon: Globe, title: "No domains" }}
        pageSize={50}
        initialSort={{ key: "name", dir: "asc" }}
        columns={[
          { key: "name", header: "Domain", sort: (d) => d.name, render: (d) => <a href={`http://${d.name}`} target="_blank" rel="noreferrer noopener" className="font-medium text-ink hover:underline">{d.name}</a> },
          { key: "kind", header: "Type", sort: (d) => d.kind, render: (d) => <Badge tone={kindTone[d.kind]}>{d.kind}</Badge> },
          { key: "user", header: "Account", sort: (d) => d.user, render: (d) => <Link to={`/accounts/${d.account_id}`} className="font-mono text-[12.5px] hover:underline">{d.user}</Link> },
          { key: "owner", header: "Owner", sort: (d) => d.owner, render: (d) => <span className="text-muted">{d.owner || "root"}</span>, hideSm: true },
          { key: "php", header: "PHP", sort: (d) => d.php_version, render: (d) => d.php_version, hideSm: true },
          { key: "ssl", header: "SSL", sort: (d) => d.ssl_not_after, render: (d) => sslBadge(d) },
          { key: "status", header: "Status", sort: (d) => d.status, render: (d) => <Badge tone={d.status === "active" ? "ok" : "warn"}>{d.status}</Badge>, hideSm: true },
        ]}
      />
    </>
  );
}

export function sslBadge(d: ServerDomain) {
  if (!d.ssl_not_after) return <Badge>None</Badge>;
  const days = Math.floor((d.ssl_not_after - Date.now() / 1000) / 86400);
  if (days < 0) return <Badge tone="bad">Expired</Badge>;
  if (days < 14) return <Badge tone="warn">{days} days left</Badge>;
  return <Badge tone="ok">Valid · {days}d</Badge>;
}

export function MultiPhp() {
  const q = useGet<ServerDomain[]>("/server/domains");
  const [target, setTarget] = useState<ServerDomain[] | null>(null);
  const [php, setPhp] = useState("8.3");
  const [result, setResult] = useState<{ updated: number; failed: { id: string; error: string }[] } | null>(null);
  const apply = useMutate(() => api.put<{ updated: number; failed: { id: string; error: string }[] }>("/server/domains/php", { domain_ids: target!.map((d) => d.id), php_version: php }), {
    invalidate: ["/server/domains"],
    onSuccess: (r) => (setResult(r), setTarget(null)),
  });
  const versions = [...new Set(["8.4", "8.3", "8.2", "8.1", "8.0", "7.4", ...(q.data?.map((d) => d.php_version) ?? [])])].sort().reverse();
  const byVersion = useMemo(() => {
    const m: Record<string, number> = {};
    for (const d of q.data ?? []) m[d.php_version] = (m[d.php_version] ?? 0) + 1;
    return m;
  }, [q.data]);
  return (
    <>
      <PageHeader title="MultiPHP Manager" />
      <div className="mb-4 flex flex-wrap gap-3">
        {Object.entries(byVersion)
          .sort()
          .reverse()
          .map(([v, n]) => (
            <Stat key={v} label={`PHP ${v}`} value={n} hint={n === 1 ? "domain" : "domains"} tone={Number(v) < 8.1 ? "warn" : undefined} />
          ))}
      </div>
      {result && (
        <div className={`mb-4 rounded-md px-3 py-2 text-[13px] ${result.failed.length ? "bg-warn-soft text-warn" : "bg-ok-soft text-ok"}`}>
          {result.updated} domain(s) updated.{result.failed.length > 0 && ` ${result.failed.length} failed: ${result.failed.map((f) => f.error).join("; ")}`}
        </div>
      )}
      <DataTable
        rows={q.data?.filter((d) => d.kind !== "alias")}
        loading={q.isLoading}
        error={q.error}
        rowKey={(d) => d.id}
        search={(d) => `${d.name} ${d.user} php${d.php_version}`}
        searchPlaceholder="Search domains, users or php8.1"
        empty={{ icon: Globe, title: "No domains" }}
        pageSize={50}
        bulk={[{ label: "Set PHP version", onRun: setTarget }]}
        columns={[
          { key: "name", header: "Domain", sort: (d) => d.name, render: (d) => <span className="font-medium text-ink">{d.name}</span> },
          { key: "user", header: "Account", sort: (d) => d.user, render: (d) => <span className="font-mono text-[12.5px]">{d.user}</span> },
          { key: "php", header: "PHP version", sort: (d) => d.php_version, render: (d) => <Badge tone={Number(d.php_version) < 8.1 ? "warn" : "neutral"}>PHP {d.php_version}</Badge> },
        ]}
        actions={(d) => (
          <Button size="sm" onClick={() => setTarget([d])}>
            Change
          </Button>
        )}
      />
      <p className="mt-2 text-[12px] text-muted">PHP 8.0 and older no longer receive security fixes from the PHP project. Here an administrator can set any installed version, even one outside the account's package.</p>
      <Modal
        open={!!target}
        onClose={() => setTarget(null)}
        title={target?.length === 1 ? `PHP for ${target[0]!.name}` : `PHP for ${target?.length} domains`}
        footer={
          <>
            <Button onClick={() => setTarget(null)}>Cancel</Button>
            <Button variant="primary" loading={apply.isPending} onClick={() => apply.mutate()}>
              Apply
            </Button>
          </>
        }
      >
        <Field label="PHP version" hint="Each account's PHP-FPM pool is reloaded. Sites keep running during the change.">
          {(id) => (
            <Select id={id} value={php} onChange={(e) => setPhp(e.target.value)}>
              {versions.map((v) => (
                <option key={v}>{v}</option>
              ))}
            </Select>
          )}
        </Field>
      </Modal>
    </>
  );
}

// ---- Backup Restoration -----------------------------------------------------------------------

interface ServerBackup {
  id: string;
  account_id: string;
  user: string;
  domain: string;
  kind: string;
  size_bytes: number;
  status: string;
  created_at: number;
}

export function BackupRestore() {
  const q = useGet<ServerBackup[]>("/server/backups");
  const [pick, setPick] = useState<ServerBackup | null>(null);
  const restore = useMutate(() => api.post(`/accounts/${pick!.account_id}/backups/${pick!.id}/restore`), { success: "Restore started. It runs in the background.", onSuccess: () => setPick(null) });
  const fmt = (n: number) => (n > 1 << 30 ? `${(n / (1 << 30)).toFixed(1)} GB` : `${Math.max(1, Math.round(n / (1 << 20)))} MB`);
  return (
    <>
      <PageHeader title="Backup Restoration" />
      <DataTable
        rows={q.data}
        loading={q.isLoading}
        error={q.error}
        rowKey={(b) => b.id}
        search={(b) => `${b.user} ${b.domain} ${b.kind}`}
        searchPlaceholder="Find backups by account or domain"
        empty={{ icon: Users, title: "No backups yet", description: "Turn on scheduled backups in Backup Configuration, or create them per account." }}
        initialSort={{ key: "created", dir: "desc" }}
        pageSize={50}
        columns={[
          { key: "domain", header: "Account", sort: (b) => b.domain, render: (b) => <div><div className="font-medium text-ink">{b.domain}</div><div className="font-mono text-[12px] text-muted">{b.user}</div></div> },
          { key: "kind", header: "Type", sort: (b) => b.kind, render: (b) => <Badge>{b.kind}</Badge> },
          { key: "created", header: "Taken", sort: (b) => b.created_at, render: (b) => <span title={formatDate(b.created_at)}>{timeAgo(b.created_at)}</span> },
          { key: "size", header: "Size", sort: (b) => b.size_bytes, render: (b) => <span className="tabular-nums">{fmt(b.size_bytes)}</span> },
          { key: "status", header: "Status", sort: (b) => b.status, render: (b) => <Badge tone={b.status === "complete" ? "ok" : b.status === "failed" ? "bad" : "warn"}>{b.status}</Badge> },
        ]}
        actions={(b) => (
          <Button size="sm" disabled={b.status !== "complete"} onClick={() => setPick(b)}>
            Restore
          </Button>
        )}
      />
      <Modal
        open={!!pick}
        onClose={() => setPick(null)}
        title={`Restore ${pick?.domain}?`}
        description={pick ? `From the ${pick.kind} backup taken ${formatDate(pick.created_at)}.` : undefined}
        footer={
          <>
            <Button onClick={() => setPick(null)}>Cancel</Button>
            <Button variant="danger" loading={restore.isPending} onClick={() => restore.mutate()}>
              Restore
            </Button>
          </>
        }
      >
        <p className="text-[13px] text-muted">Files and databases in the backup overwrite the current versions. Files created since the backup are kept. The account stays online during the restore.</p>
      </Modal>
    </>
  );
}
