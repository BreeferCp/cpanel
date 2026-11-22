import { Ban, FileLock2, Plus, Trash2, UserPlus, X } from "lucide-react";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { Domain, HotlinkPolicy, IPBlock, ProtectedDir } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button, IconButton } from "@/components/ui/Button";
import { Field, Input, Select, Switch } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, Mono, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { Modal } from "@/components/ui/Modal";

// ---- Directory Privacy ----------------------------------------------------------

function LeechProtection({ dir }: { dir: ProtectedDir }) {
  const aid = useAccountId();
  const key = `/accounts/${aid}/protected-dirs`;
  const [limit, setLimit] = useState(String(dir.leech_limit || ""));
  const save = useMutate(() => api.put(`${key}/${dir.id}/leech`, { limit: Number(limit || 0) }), { invalidate: [key], success: "Leech Protection saved" });
  return (
    <form
      className="mt-4 flex flex-wrap items-end gap-2 border-t border-line pt-3"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <Field label="Leech Protection" hint="Block a login used from more than this many addresses in 2 hours. Empty is off." className="min-w-0 max-w-sm flex-1">
        {(id) => <Input id={id} type="number" min={0} max={1000} value={limit} onChange={(e) => setLimit(e.target.value)} />}
      </Field>
      <Button type="submit" size="sm" loading={save.isPending} disabled={Number(limit || 0) === (dir.leech_limit || 0)}>
        Save limit
      </Button>
      {!!dir.blocked_users?.length && <p className="w-full text-[12.5px] text-warn">Blocked for sharing: {dir.blocked_users.join(", ")}. Set a new password to unblock.</p>}
    </form>
  );
}

export function DirectoryPrivacy() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/protected-dirs`;
  const q = useGet<ProtectedDir[]>(key);
  const domains = useGet<Domain[]>(`/accounts/${aid}/domains`);
  const [open, setOpen] = useState(false);
  const [f, setF] = useState({ domain_id: "", path: "/private", realm: "Restricted area" });
  const [userFor, setUserFor] = useState<ProtectedDir | null>(null);
  const [u, setU] = useState({ user: "", password: "" });

  const create = useMutate(() => api.post(key, { ...f, domain_id: f.domain_id || domains.data?.[0]?.id }), {
    invalidate: [key],
    success: "Directory protected",
    onSuccess: () => setOpen(false),
  });
  const remove = useMutate((id: string) => api.del(`${key}/${id}`), { invalidate: [key], success: "Protection removed" });
  const addUser = useMutate(() => api.put(`${key}/${userFor?.id}/users/${encodeURIComponent(u.user)}`, { password: u.password }), {
    invalidate: [key],
    success: "User saved",
    onSuccess: () => (setUserFor(null), setU({ user: "", password: "" })),
  });
  const delUser = useMutate((x: { id: string; user: string }) => api.del(`${key}/${x.id}/users/${encodeURIComponent(x.user)}`), {
    invalidate: [key],
    success: "User removed",
  });

  return (
    <>
      <PageHeader
        title="Directory Privacy"
        description="Require a user name and password to open a folder of your website (HTTP basic authentication, sent over HTTPS)."
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setOpen(true)}>
            Protect a directory
          </Button>
        }
      />
      {q.isLoading ? (
        <Loading />
      ) : !q.data?.length ? (
        <Card>
          <EmptyState icon={FileLock2} title="No protected directories" description="Useful for staging sites, admin areas and private downloads." />
        </Card>
      ) : (
        <div className="space-y-4">
          {q.data.map((d) => (
            <Card
              key={d.id}
              title={`${d.domain}${d.path}`}
              description={`Prompt: “${d.realm}”`}
              actions={
                <>
                  <Button size="sm" icon={UserPlus} onClick={() => setUserFor(d)}>
                    Add user
                  </Button>
                  <IconButton icon={Trash2} label={`Remove protection from ${d.path}`} onClick={() => remove.mutate(d.id)} />
                </>
              }
            >
              {d.users.length ? (
                <div className="flex flex-wrap gap-2">
                  {d.users.map((name) => (
                    <span key={name} className="inline-flex items-center gap-1 rounded-md border border-line bg-surface2 py-0.5 pl-2 pr-0.5 text-[12.5px]">
                      {name}
                      <IconButton icon={X} label={`Remove ${name}`} onClick={() => delUser.mutate({ id: d.id, user: name })} />
                    </span>
                  ))}
                </div>
              ) : (
                <p className="text-[13px] text-warn">No users yet: nobody can open this directory until you add one.</p>
              )}
              <LeechProtection dir={d} />
            </Card>
          ))}
        </div>
      )}

      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Protect a directory"
        footer={
          <>
            <Button onClick={() => setOpen(false)}>Cancel</Button>
            <Button variant="primary" loading={create.isPending} onClick={() => create.mutate()}>
              Protect
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="Domain">
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
          <Field label="Path" hint="The URL path, e.g. /private or /members/area">
            {(id) => <Input id={id} className="font-mono" value={f.path} onChange={(e) => setF({ ...f, path: e.target.value })} />}
          </Field>
          <Field label="Name shown in the password prompt">{(id) => <Input id={id} value={f.realm} onChange={(e) => setF({ ...f, realm: e.target.value })} />}</Field>
        </div>
      </Modal>
      <Modal
        open={!!userFor}
        onClose={() => setUserFor(null)}
        title={`Add user to ${userFor?.path}`}
        description="Adding an existing user name changes its password."
        footer={
          <>
            <Button onClick={() => setUserFor(null)}>Cancel</Button>
            <Button variant="primary" loading={addUser.isPending} onClick={() => addUser.mutate()}>
              Save user
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="User name">{(id) => <Input id={id} value={u.user} onChange={(e) => setU({ ...u, user: e.target.value })} autoFocus />}</Field>
          <Field label="Password" hint="8 characters or more.">
            {(id) => <Input id={id} type="password" value={u.password} onChange={(e) => setU({ ...u, password: e.target.value })} autoComplete="new-password" />}
          </Field>
        </div>
      </Modal>
    </>
  );
}

// ---- IP Blocker ----------------------------------------------------------------------

export function IpBlocker() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/ip-blocks`;
  const q = useGet<IPBlock[]>(key);
  const [cidr, setCidr] = useState("");
  const add = useMutate(() => api.post(key, { cidr }), { invalidate: [key], success: "Address blocked", onSuccess: () => setCidr("") });
  const remove = useMutate((id: string) => api.del(`${key}/${id}`), { invalidate: [key], success: "Address unblocked" });
  return (
    <>
      <PageHeader title="IP Blocker" description="Refuse access to all of your websites from specific addresses or ranges. Blocked visitors see 403 Forbidden." />
      <Card className="mb-4">
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            add.mutate();
          }}
        >
          <Field label="IP address or range" className="min-w-64 flex-1">
            {(id) => <Input id={id} className="font-mono" placeholder="203.0.113.7 or 203.0.113.0/24" value={cidr} onChange={(e) => setCidr(e.target.value)} />}
          </Field>
          <Button type="submit" variant="primary" icon={Ban} loading={add.isPending} disabled={!cidr.trim()}>
            Block
          </Button>
        </form>
      </Card>
      <Card flush>
        {q.isLoading ? (
          <Loading />
        ) : !q.data?.length ? (
          <EmptyState icon={Ban} title="Nothing blocked" />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Address</Th>
                <Th>Blocked</Th>
                <Th className="w-10">
                  <span className="sr-only">Actions</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((b) => (
                <Tr key={b.id}>
                  <Td>
                    <Mono className="text-ink">{b.cidr}</Mono>
                  </Td>
                  <Td className="text-muted">{formatDate(b.created_at)}</Td>
                  <Td>
                    <IconButton icon={Trash2} label={`Unblock ${b.cidr}`} onClick={() => remove.mutate(b.id)} />
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

// ---- Hotlink Protection -----------------------------------------------------------------

export function Hotlink() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/hotlink`;
  const q = useGet<HotlinkPolicy>(key);
  const [f, setF] = useState({ enabled: false, allowed: "", extensions: "", allow_empty: true });
  useEffect(() => {
    if (q.data) setF({ ...q.data, allowed: q.data.allowed.join(" "), extensions: q.data.extensions.join(" ") });
  }, [q.data]);
  const split = (s: string) => s.split(/[\s,]+/).filter(Boolean);
  const save = useMutate(() => api.put(key, { ...f, allowed: split(f.allowed), extensions: split(f.extensions) }), { invalidate: [key], success: "Hotlink protection saved" });
  if (q.isLoading) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  return (
    <>
      <PageHeader
        title="Hotlink Protection"
        description="Stop other websites from embedding your images and media, which uses your bandwidth. Your own domains are always allowed."
      />
      <Card>
        <div className="max-w-xl space-y-5">
          <div className="flex items-center justify-between">
            <div>
              <div className="text-[13px] font-medium">Protection</div>
              <div className="text-[12.5px] text-muted">{f.enabled ? <Badge tone="ok">Enabled</Badge> : <Badge>Disabled</Badge>}</div>
            </div>
            <Switch checked={f.enabled} onChange={(v) => setF({ ...f, enabled: v })} label="Hotlink protection" />
          </div>
          <Field label="Protected file extensions" hint="Separated by spaces, without dots.">
            {(id) => <Input id={id} className="font-mono" value={f.extensions} onChange={(e) => setF({ ...f, extensions: e.target.value })} />}
          </Field>
          <Field label="Also allow these sites" hint="Host names such as partner.example or *.cdn.example">
            {(id) => <Input id={id} className="font-mono" value={f.allowed} onChange={(e) => setF({ ...f, allowed: e.target.value })} />}
          </Field>
          <label className="flex items-center justify-between gap-3 text-[13px]">
            <span>
              Allow direct requests
              <span className="block text-[12.5px] text-muted">Visitors who open a file directly or whose browser hides the referrer.</span>
            </span>
            <Switch checked={f.allow_empty} onChange={(v) => setF({ ...f, allow_empty: v })} label="Allow direct requests" />
          </label>
          <div className="flex justify-end">
            <Button variant="primary" loading={save.isPending} onClick={() => save.mutate()}>
              Save
            </Button>
          </div>
        </div>
      </Card>
    </>
  );
}
