import { ExternalLink, Package, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import { timeAgo } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { CatalogApp, Domain, Installation } from "@/lib/types";
import { useAccountId, useBase } from "@/state/scope";
import { useAuth } from "@/state/auth";
import { Button, IconButton } from "@/components/ui/Button";
import { Checkbox, Field, Input, Select } from "@/components/ui/Form";
import { Badge, Card, EmptyState, Loading, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { Modal } from "@/components/ui/Modal";

const initials = (n: string) => n.slice(0, 2).toUpperCase();

export default function Installer() {
  const aid = useAccountId();
  const base = useBase();
  const { user } = useAuth();
  const key = `/accounts/${aid}/installations`;
  const catalog = useGet<CatalogApp[]>("/installer/catalog");
  const installs = useGet<Installation[]>(key, true, (d) => (d?.some((i) => i.status === "installing") ? 3000 : false));
  const domains = useGet<Domain[]>(`/accounts/${aid}/domains`);
  const [pick, setPick] = useState<CatalogApp | null>(null);
  const [f, setF] = useState({ domain_id: "", directory: "", title: "", admin_user: "admin", admin_password: "", admin_email: "" });
  const [remove, setRemove] = useState<Installation | null>(null);
  const [opts, setOpts] = useState({ delete_files: true, delete_database: true });
  const [cat, setCat] = useState("All");

  const categories = useMemo(() => ["All", ...new Set(catalog.data?.map((c) => c.category) ?? [])], [catalog.data]);
  const shown = catalog.data?.filter((c) => cat === "All" || c.category === cat) ?? [];

  const install = useMutate(() => api.post(key, { app: pick!.id, ...f }), {
    invalidate: [key],
    success: "Installation started",
    onSuccess: () => setPick(null),
  });
  const del = useMutate(() => api.del(`${key}/${remove!.id}`, opts), { invalidate: [key], success: "Installation removed", onSuccess: () => setRemove(null) });

  const open = (c: CatalogApp) => {
    setF({ domain_id: domains.data?.[0]?.id ?? "", directory: "", title: "", admin_user: "admin", admin_password: "", admin_email: user?.email ?? "" });
    setPick(c);
  };

  return (
    <>
      <PageHeader title="App Installer" description="One-click installs of popular open-source applications, with the database created for you." />

      <Card title="Installed applications" flush className="mb-6">
        {installs.isLoading ? (
          <Loading />
        ) : !installs.data?.length ? (
          <EmptyState icon={Package} title="Nothing installed yet" description="Pick an application below." />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Application</Th>
                <Th>URL</Th>
                <Th>Version</Th>
                <Th>Status</Th>
                <Th>Installed</Th>
                <Th className="w-24">
                  <span className="sr-only">Actions</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {installs.data.map((i) => (
                <Tr key={i.id}>
                  <Td className="font-medium text-ink">
                    {i.app === "wordpress" && i.status === "installed" ? (
                      <Link to={`${base}/wordpress?id=${i.id}`} className="hover:text-accent">
                        {i.app_name}
                      </Link>
                    ) : (
                      i.app_name
                    )}
                  </Td>
                  <Td>
                    <a href={i.url} target="_blank" rel="noreferrer noopener" className="inline-flex items-center gap-1 text-muted hover:text-accent">
                      {i.url.replace(/^https?:\/\//, "")}
                      <ExternalLink className="size-3" aria-hidden />
                    </a>
                  </Td>
                  <Td className="text-muted">{i.version || "—"}</Td>
                  <Td>
                    <Badge tone={i.status === "installed" ? "ok" : i.status === "failed" ? "bad" : "warn"}>
                      {i.status === "installing" ? "Installing…" : i.status === "installed" ? "Installed" : "Failed"}
                    </Badge>
                  </Td>
                  <Td className="text-muted">{timeAgo(i.created_at)}</Td>
                  <Td>
                    <IconButton icon={Trash2} label={`Remove ${i.app_name} at ${i.url}`} disabled={i.status === "installing"} onClick={() => setRemove(i)} />
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-[15px] font-semibold">Catalog</h2>
        <div className="flex flex-wrap gap-1" role="tablist">
          {categories.map((c) => (
            <button
              key={c}
              role="tab"
              aria-selected={cat === c}
              onClick={() => setCat(c)}
              className={`rounded-full px-3 py-1 text-[12.5px] ${cat === c ? "bg-accent-soft font-medium text-accent" : "text-muted hover:bg-surface2"}`}
            >
              {c}
            </button>
          ))}
        </div>
      </div>
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {shown.map((c) => (
          <div key={c.id} className="flex flex-col rounded-lg border border-line bg-surface p-4 shadow-card">
            <div className="flex items-start gap-3">
              <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-accent-soft text-[13px] font-semibold text-accent">{initials(c.name)}</div>
              <div className="min-w-0">
                <div className="text-[14px] font-semibold">{c.name}</div>
                <div className="text-[12px] text-muted">{c.category}</div>
              </div>
            </div>
            <p className="mt-3 flex-1 text-[13px] text-muted">{c.description}</p>
            <div className="mt-4 flex items-center justify-between">
              <span className="text-[11.5px] text-subtle">{c.method === "wpcli" ? "Fully automated" : c.method === "composer" ? "Composer project" : "Finish setup in browser"}</span>
              <Button size="sm" variant="primary" onClick={() => open(c)}>
                Install
              </Button>
            </div>
          </div>
        ))}
      </div>

      <Modal
        open={!!pick}
        onClose={() => setPick(null)}
        title={`Install ${pick?.name}`}
        description={pick?.needs_db ? "A database and database user are created automatically." : undefined}
        footer={
          <>
            <Button onClick={() => setPick(null)}>Cancel</Button>
            <Button
              variant="primary"
              loading={install.isPending}
              disabled={!f.domain_id || (pick?.method === "wpcli" && (!f.admin_password || !f.admin_email))}
              onClick={() => install.mutate()}
            >
              Install
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="grid grid-cols-[1fr_auto_1fr] items-end gap-2">
            <Field label="Domain">
              {(id) => (
                <Select id={id} value={f.domain_id} onChange={(e) => setF({ ...f, domain_id: e.target.value })}>
                  {domains.data?.map((d) => (
                    <option key={d.id} value={d.id}>
                      {d.name}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            <span className="pb-1.5 text-muted">/</span>
            <Field label="Directory" hint="Blank installs at the root.">
              {(id) => <Input id={id} value={f.directory} onChange={(e) => setF({ ...f, directory: e.target.value })} placeholder="blog" />}
            </Field>
          </div>
          {pick?.method === "wpcli" && (
            <>
              <Field label="Site title">{(id) => <Input id={id} value={f.title} onChange={(e) => setF({ ...f, title: e.target.value })} placeholder="My Site" />}</Field>
              <div className="grid grid-cols-2 gap-3">
                <Field label="Admin username">{(id) => <Input id={id} value={f.admin_user} onChange={(e) => setF({ ...f, admin_user: e.target.value })} />}</Field>
                <Field label="Admin email">{(id) => <Input id={id} type="email" value={f.admin_email} onChange={(e) => setF({ ...f, admin_email: e.target.value })} />}</Field>
              </div>
              <Field label="Admin password" hint="At least 12 characters.">
                {(id) => <Input id={id} type="password" value={f.admin_password} onChange={(e) => setF({ ...f, admin_password: e.target.value })} autoComplete="new-password" />}
              </Field>
            </>
          )}
          {pick?.webroot && <p className="text-[12.5px] text-muted">After installing, point the domain's document root at the {pick.webroot}/ folder.</p>}
        </div>
      </Modal>

      <Modal
        open={!!remove}
        onClose={() => setRemove(null)}
        title={`Remove ${remove?.app_name}?`}
        footer={
          <>
            <Button onClick={() => setRemove(null)}>Cancel</Button>
            <Button variant="danger" loading={del.isPending} onClick={() => del.mutate()}>
              Remove
            </Button>
          </>
        }
      >
        <div className="space-y-2 text-[13px]">
          <label className="flex items-center gap-2">
            <Checkbox checked={opts.delete_files} onChange={(e) => setOpts({ ...opts, delete_files: e.target.checked })} />
            Delete files in <span className="font-mono text-[12px]">{remove?.path}</span>
          </label>
          {remove?.db_name && (
            <label className="flex items-center gap-2">
              <Checkbox checked={opts.delete_database} onChange={(e) => setOpts({ ...opts, delete_database: e.target.checked })} />
              Delete database <span className="font-mono text-[12px]">{remove.db_name}</span>
            </label>
          )}
        </div>
      </Modal>
    </>
  );
}
