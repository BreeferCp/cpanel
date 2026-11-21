import { AppWindow, ExternalLink, KeyRound, RefreshCw, Wrench } from "lucide-react";
import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import type { Installation, WPInfo, WPItem } from "@/lib/types";
import { useAccountId, useBase } from "@/state/scope";
import { Button } from "@/components/ui/Button";
import { Field, Input, Switch } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { Modal } from "@/components/ui/Modal";

export default function WordPress() {
  const aid = useAccountId();
  const base = useBase();
  const [params, setParams] = useSearchParams();
  const installs = useGet<Installation[]>(`/accounts/${aid}/installations`);
  const sites = installs.data?.filter((i) => i.app === "wordpress" && i.status === "installed") ?? [];
  const selected = sites.find((s) => s.id === params.get("id")) ?? sites[0];

  if (installs.isLoading) return <Loading />;
  return (
    <>
      <PageHeader title="WordPress Manager" description="Updates, plugins, themes, maintenance mode and admin access for your WordPress sites." />
      {!sites.length ? (
        <Card>
          <EmptyState
            icon={AppWindow}
            title="No WordPress sites"
            description="Install WordPress from the App Installer and it will appear here."
            action={
              <Link to={`${base}/installer`}>
                <Button variant="primary">Open App Installer</Button>
              </Link>
            }
          />
        </Card>
      ) : (
        <div className="grid gap-5 lg:grid-cols-[240px_1fr]">
          <nav className="space-y-1" aria-label="WordPress sites">
            {sites.map((s) => (
              <button
                key={s.id}
                onClick={() => setParams({ id: s.id })}
                className={`block w-full rounded-md px-3 py-2 text-left text-[13px] ${s.id === selected?.id ? "bg-accent-soft font-medium text-accent" : "text-ink hover:bg-surface2"}`}
              >
                {s.url.replace(/^https?:\/\//, "")}
              </button>
            ))}
          </nav>
          {selected && <Site aid={aid} site={selected} />}
        </div>
      )}
    </>
  );
}

function Site({ aid, site }: { aid: string; site: Installation }) {
  const key = `/accounts/${aid}/installations/${site.id}/wordpress`;
  const q = useGet<WPInfo>(key);
  const [reset, setReset] = useState(false);
  const [pw, setPw] = useState({ user: site.admin_user || "admin", password: "" });

  const act = useMutate((v: { action: string; target?: string; password?: string }) => api.post(key, v), {
    invalidate: [key],
    success: (_r, v) => (v.action === "reset-password" ? "Password updated" : "Done"),
    onSuccess: (_r, v) => v.action === "reset-password" && setReset(false),
  });
  const busy = (action: string, target?: string) => act.isPending && act.variables?.action === action && act.variables?.target === target;

  if (q.isLoading) return <Loading />;
  if (q.error || !q.data) return <ErrorNote error={q.error} />;
  const info = q.data;
  const updates = info.plugins.filter((p) => p.update === "available").length;

  const itemTable = (items: WPItem[], kind: "plugin" | "theme") => (
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
                {kind === "plugin" &&
                  (p.status === "active" ? (
                    <Button size="sm" variant="ghost" loading={busy("plugin-deactivate", p.name)} onClick={() => act.mutate({ action: "plugin-deactivate", target: p.name })}>
                      Deactivate
                    </Button>
                  ) : (
                    <Button size="sm" variant="ghost" loading={busy("plugin-activate", p.name)} onClick={() => act.mutate({ action: "plugin-activate", target: p.name })}>
                      Activate
                    </Button>
                  ))}
                {kind === "theme" && p.status !== "active" && (
                  <Button size="sm" variant="ghost" onClick={() => act.mutate({ action: "theme-activate", target: p.name })}>
                    Activate
                  </Button>
                )}
              </div>
            </Td>
          </Tr>
        ))}
      </tbody>
    </Table>
  );

  return (
    <div className="min-w-0 space-y-5">
      <Card
        title={site.url.replace(/^https?:\/\//, "")}
        description={site.path}
        actions={
          <>
            <a href={`${site.url}/wp-admin/`} target="_blank" rel="noreferrer noopener">
              <Button size="sm" icon={ExternalLink}>
                wp-admin
              </Button>
            </a>
            <Button size="sm" icon={KeyRound} onClick={() => setReset(true)}>
              Reset admin password
            </Button>
          </>
        }
      >
        <div className="grid gap-4 sm:grid-cols-3">
          <div>
            <div className="text-[12px] text-muted">WordPress</div>
            <div className="mt-1 flex items-center gap-2">
              <span className="text-[18px] font-semibold tabular-nums">{info.version}</span>
              {info.update_version ? <Badge tone="warn">{info.update_version} available</Badge> : <Badge tone="ok">Up to date</Badge>}
            </div>
            {info.update_version && (
              <Button size="sm" variant="primary" className="mt-2" icon={RefreshCw} loading={busy("core-update")} onClick={() => act.mutate({ action: "core-update" })}>
                Update core
              </Button>
            )}
          </div>
          <div>
            <div className="text-[12px] text-muted">Plugins</div>
            <div className="mt-1 text-[18px] font-semibold tabular-nums">{info.plugins.length}</div>
            {updates > 0 && (
              <Button size="sm" className="mt-2" loading={busy("plugins-update-all")} onClick={() => act.mutate({ action: "plugins-update-all" })}>
                Update all ({updates})
              </Button>
            )}
          </div>
          <div>
            <div className="text-[12px] text-muted">Maintenance mode</div>
            <div className="mt-2 flex items-center gap-2">
              <Switch
                label="Maintenance mode"
                checked={info.maintenance}
                onChange={(v) => act.mutate({ action: v ? "maintenance-on" : "maintenance-off" })}
              />
              <span className="text-[13px]">{info.maintenance ? "Visitors see a maintenance page" : "Off"}</span>
            </div>
            <Button size="sm" variant="ghost" className="mt-2" icon={Wrench} onClick={() => act.mutate({ action: "cache-flush" })}>
              Flush object cache
            </Button>
          </div>
        </div>
      </Card>
      <Card title="Plugins" flush>
        {itemTable(info.plugins, "plugin")}
      </Card>
      <Card title="Themes" flush>
        {itemTable(info.themes, "theme")}
      </Card>

      <Modal
        open={reset}
        onClose={() => setReset(false)}
        title="Reset WordPress password"
        footer={
          <>
            <Button onClick={() => setReset(false)}>Cancel</Button>
            <Button variant="primary" loading={busy("reset-password", pw.user)} disabled={pw.password.length < 12} onClick={() => act.mutate({ action: "reset-password", target: pw.user, password: pw.password })}>
              Set password
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="WordPress username">{(id) => <Input id={id} value={pw.user} onChange={(e) => setPw({ ...pw, user: e.target.value })} />}</Field>
          <Field label="New password" hint="At least 12 characters.">
            {(id) => <Input id={id} type="password" value={pw.password} onChange={(e) => setPw({ ...pw, password: e.target.value })} autoComplete="new-password" />}
          </Field>
        </div>
      </Modal>
    </div>
  );
}
