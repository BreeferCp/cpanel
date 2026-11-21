import { Copy, Package as PackageIcon, Pencil, Plus, Trash2 } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "@/lib/api";
import { formatLimit, formatMB } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { Account, FeatureList, Pkg } from "@/lib/types";
import { Button, IconButton } from "@/components/ui/Button";
import { DataTable } from "@/components/ui/DataTable";
import { Field, Input, Select, Switch } from "@/components/ui/Form";
import { Badge, Card, ErrorNote, Loading, PageHeader } from "@/components/ui/Layout";
import { ConfirmDialog } from "@/components/ui/Modal";

const blank: Omit<Pkg, "id"> = {
  name: "",
  disk_mb: 10240,
  bandwidth_mb: 0,
  max_domains: 5,
  max_databases: 5,
  max_mailboxes: 10,
  max_cron: 5,
  max_apps: 2,
  cpu_percent: 100,
  memory_mb: 1024,
  io_weight: 100,
  max_procs: 200,
  php_versions: "8.3,8.2",
  shell_access: false,
  max_ftp: 5,
  feature_list_id: "",
};

type NumKey = { [K in keyof Pkg]: Pkg[K] extends number ? K : never }[keyof Pkg];

export default function Packages() {
  const q = useGet<Pkg[]>("/packages");
  const accounts = useGet<Account[]>("/accounts");
  const lists = useGet<FeatureList[]>("/feature-lists");
  const nav = useNavigate();
  const [del, setDel] = useState<Pkg | null>(null);
  const remove = useMutate((id: string) => api.del(`/packages/${id}`), { invalidate: ["/packages"], success: "Package deleted", onSuccess: () => setDel(null) });
  const used = (id: string) => accounts.data?.filter((a) => a.package_id === id).length ?? 0;
  const listName = (id: string) => lists.data?.find((l) => l.id === id)?.name;

  return (
    <>
      <PageHeader
        title="Packages"
        description="The plans you sell. A limit of 0 means unlimited."
        actions={
          <Link to="/packages/new">
            <Button variant="primary" icon={Plus}>
              Add a package
            </Button>
          </Link>
        }
      />
      <DataTable
        rows={q.data}
        loading={q.isLoading}
        error={q.error}
        rowKey={(p) => p.id}
        search={(p) => p.name}
        searchPlaceholder="Search packages"
        empty={{ icon: PackageIcon, title: "No packages", description: "Packages define disk, bandwidth, CPU and memory limits for each account.", action: <Link to="/packages/new"><Button icon={Plus}>Add a package</Button></Link> }}
        initialSort={{ key: "name", dir: "asc" }}
        columns={[
          { key: "name", header: "Package", sort: (p) => p.name, render: (p) => <Link to={`/packages/${p.id}`} className="font-medium text-ink hover:underline">{p.name}</Link> },
          { key: "accounts", header: "Accounts", sort: (p) => used(p.id), render: (p) => <span className="tabular-nums">{used(p.id)}</span> },
          { key: "disk", header: "Disk", sort: (p) => p.disk_mb || Infinity, render: (p) => formatLimit(p.disk_mb, "mb") },
          { key: "bw", header: "Bandwidth", sort: (p) => p.bandwidth_mb || Infinity, render: (p) => formatLimit(p.bandwidth_mb, "mb"), hideSm: true },
          { key: "domains", header: "Domains", sort: (p) => p.max_domains || Infinity, render: (p) => formatLimit(p.max_domains), hideSm: true },
          { key: "mail", header: "Mailboxes", sort: (p) => p.max_mailboxes || Infinity, render: (p) => formatLimit(p.max_mailboxes), hideSm: true },
          { key: "cpu", header: "CPU / RAM", render: (p) => <span className="whitespace-nowrap">{p.cpu_percent / 100} core · {formatMB(p.memory_mb)}</span>, hideSm: true },
          { key: "features", header: "Features", render: (p) => (p.feature_list_id ? <Badge>{listName(p.feature_list_id) ?? "custom"}</Badge> : <span className="text-muted">all</span>), hideSm: true },
        ]}
        actions={(p) => (
          <>
            <IconButton icon={Pencil} label={`Edit ${p.name}`} onClick={() => nav(`/packages/${p.id}`)} />
            <IconButton icon={Copy} label={`Duplicate ${p.name}`} onClick={() => nav(`/packages/new?from=${p.id}`)} />
            <IconButton icon={Trash2} label={`Delete ${p.name}`} onClick={() => setDel(p)} />
          </>
        )}
      />
      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        onConfirm={() => del && remove.mutate(del.id)}
        loading={remove.isPending}
        title={`Delete ${del?.name}?`}
        message={del && used(del.id) ? `${used(del.id)} account(s) still use this package. Move them to another package first.` : "The package is removed. No account uses it."}
      />
    </>
  );
}

/** Add a Package / Edit a Package, as its own page. */
export function PackageEditor() {
  const { id } = useParams();
  const isNew = !id || id === "new";
  const from = new URLSearchParams(window.location.search).get("from");
  const q = useGet<Pkg[]>("/packages");
  const lists = useGet<FeatureList[]>("/feature-lists");
  const nav = useNavigate();
  const [p, setP] = useState<Omit<Pkg, "id"> & { id?: string }>({ ...blank });
  useEffect(() => {
    const src = q.data?.find((x) => x.id === (isNew ? from : id));
    if (src) setP(isNew ? { ...src, id: undefined, name: `${src.name} copy` } : src);
  }, [q.data, id, isNew, from]);
  const save = useMutate(() => (p.id ? api.put(`/packages/${p.id}`, p) : api.post("/packages", p)), {
    invalidate: ["/packages"],
    success: "Package saved",
    onSuccess: () => nav("/packages"),
  });
  if (q.isLoading) return <Loading />;
  if (!isNew && !q.data?.some((x) => x.id === id)) return <ErrorNote error="Package not found" />;

  const num = (key: NumKey, label: string, hint?: string, unit?: string) => (
    <Field label={label} hint={hint ?? (p[key] === 0 ? "Unlimited" : unit === "mb" ? formatMB(p[key]) : undefined)}>
      {(fid) => <Input id={fid} type="number" min={0} value={String(p[key])} onChange={(e) => setP({ ...p, [key]: Number(e.target.value) })} />}
    </Field>
  );
  const section = (title: string, desc: string, children: ReactNode) => (
    <Card title={title} description={desc}>
      {children}
    </Card>
  );

  return (
    <>
      <PageHeader title={isNew ? "Add a Package" : `Edit ${p.name || "package"}`} description="A limit of 0 means unlimited. Changes apply to every account on the package." />
      <form
        className="max-w-4xl space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        {section(
          "Package",
          "The name customers and billing systems see.",
          <Field label="Package name">{(fid) => <Input id={fid} value={p.name} onChange={(e) => setP({ ...p, name: e.target.value })} autoFocus={isNew} placeholder="Business" />}</Field>,
        )}
        {section(
          "Resources",
          "Storage and transfer limits.",
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {num("disk_mb", "Disk quota (MB)", undefined, "mb")}
            {num("bandwidth_mb", "Monthly bandwidth (MB)", undefined, "mb")}
          </div>,
        )}
        {section(
          "Limits",
          "How many of each item an account may create.",
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {num("max_domains", "Domains")}
            {num("max_databases", "Databases")}
            {num("max_mailboxes", "Email accounts")}
            {num("max_ftp", "FTP accounts")}
            {num("max_cron", "Cron jobs")}
            {num("max_apps", "Applications")}
          </div>,
        )}
        {section(
          "Isolation",
          "Enforced by Linux cgroups v2. A site at its limit slows down instead of affecting others.",
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {num("cpu_percent", "CPU (%)", "100 = one full core")}
            {num("memory_mb", "Memory (MB)", undefined, "mb")}
            {num("io_weight", "Disk I/O weight", "1–10000, default 100")}
            {num("max_procs", "Max processes")}
          </div>,
        )}
        {section(
          "Software and features",
          "What customers on this package can use.",
          <div className="space-y-4">
            <Field label="PHP versions" hint="Comma separated; the first is the default for new domains.">
              {(fid) => <Input id={fid} className="font-mono" value={p.php_versions} onChange={(e) => setP({ ...p, php_versions: e.target.value })} />}
            </Field>
            <Field label="Feature list" hint="Which cPanel tools the package includes. Manage lists in Feature Manager.">
              {(fid) => (
                <Select id={fid} value={p.feature_list_id} onChange={(e) => setP({ ...p, feature_list_id: e.target.value })}>
                  <option value="">All features</option>
                  {lists.data?.map((l) => (
                    <option key={l.id} value={l.id}>
                      {l.name}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            <div className="flex items-center gap-2.5">
              <Switch label="Shell access" checked={p.shell_access} onChange={(v) => setP({ ...p, shell_access: v })} />
              <div className="text-[13px]">
                <div className="font-medium text-ink">Allow shell access</div>
                <div className="text-muted">Lets you give accounts SSH and the browser terminal. Each account still starts without a shell.</div>
              </div>
            </div>
          </div>,
        )}
        <div className="flex gap-2">
          <Button type="submit" variant="primary" loading={save.isPending} disabled={!p.name}>
            {isNew ? "Add package" : "Save changes"}
          </Button>
          <Link to="/packages">
            <Button>Cancel</Button>
          </Link>
        </div>
      </form>
    </>
  );
}
