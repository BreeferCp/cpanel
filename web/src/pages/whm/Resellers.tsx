import { Ban, Crown, Pencil, Plus, Trash2, UsersRound } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { formatLimit, formatMB } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { ResellerFull } from "@/lib/types";
import { useAuth } from "@/state/auth";
import { Button, IconButton } from "@/components/ui/Button";
import { Field, Input, Switch } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

interface Form {
  id?: string;
  username: string;
  email: string;
  password: string;
  is_master: boolean;
  max_accounts: number;
  max_disk_mb: number;
  max_resellers: number;
}

const blank: Form = { username: "", email: "", password: "", is_master: false, max_accounts: 50, max_disk_mb: 102400, max_resellers: 0 };

function Usage({ used, limit, fmt = String }: { used: number; limit: number; fmt?: (n: number) => string }) {
  const p = limit ? Math.min(100, (used / limit) * 100) : 0;
  return (
    <div className="min-w-28">
      <div className="text-[12.5px] tabular-nums">
        {fmt(used)} <span className="text-muted">/ {limit ? fmt(limit) : "∞"}</span>
      </div>
      {limit > 0 && (
        <div className="mt-1 h-1 overflow-hidden rounded-full bg-surface2">
          <div className={`h-full rounded-full ${p >= 90 ? "bg-bad" : "bg-accent"}`} style={{ width: `${Math.max(p, used ? 2 : 0)}%` }} />
        </div>
      )}
    </div>
  );
}

export default function Resellers() {
  const { user } = useAuth();
  const isRoot = user?.role === "root";
  const q = useGet<ResellerFull[]>("/resellers");
  const [form, setForm] = useState<Form | null>(null);
  const [del, setDel] = useState<ResellerFull | null>(null);

  const save = useMutate(
    (f: Form) => {
      const body = { email: f.email, is_master: f.is_master, max_accounts: f.max_accounts, max_disk_mb: f.max_disk_mb, max_resellers: f.max_resellers };
      return f.id
        ? api.patch(`/resellers/${f.id}`, f.password ? { ...body, password: f.password } : body)
        : api.post("/resellers", { ...body, username: f.username, password: f.password });
    },
    { invalidate: ["/resellers"], success: (_r, f) => (f.id ? "Reseller updated" : "Reseller created"), onSuccess: () => setForm(null) },
  );
  const toggle = useMutate((r: ResellerFull) => api.patch(`/resellers/${r.id}`, { suspended: !r.suspended }), {
    invalidate: ["/resellers"],
    success: (_x, r) => (r.suspended ? "Reseller reactivated" : "Reseller suspended"),
  });
  const remove = useMutate((id: string) => api.del(`/resellers/${id}`), { invalidate: ["/resellers"], success: "Reseller deleted", onSuccess: () => setDel(null) });

  const num = (k: "max_accounts" | "max_disk_mb" | "max_resellers", label: string, hint?: string) => (
    <Field label={label} hint={hint}>
      {(id) => <Input id={id} type="number" min={0} value={String(form?.[k] ?? 0)} onChange={(e) => form && setForm({ ...form, [k]: Number(e.target.value) })} />}
    </Field>
  );

  return (
    <>
      <PageHeader
        title="Resellers"
        description={
          isRoot
            ? "Resellers sell hosting from this server with their own packages and customers. Master resellers can also create resellers."
            : "Resellers you manage. Each one sees only its own customers."
        }
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setForm({ ...blank })}>
            Create reseller
          </Button>
        }
      />
      <Card flush>
        {q.isLoading ? (
          <Loading />
        ) : q.error ? (
          <div className="p-4">
            <ErrorNote error={q.error} />
          </div>
        ) : !q.data?.length ? (
          <EmptyState icon={UsersRound} title="No resellers" description="Create a reseller to let someone else sell hosting from this server." />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Reseller</Th>
                {isRoot && <Th>Parent</Th>}
                <Th>Accounts</Th>
                <Th>Disk allocated</Th>
                <Th>Sub-resellers</Th>
                <Th>Status</Th>
                <Th className="w-28">
                  <span className="sr-only">Actions</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((r) => (
                <Tr key={r.id}>
                  <Td>
                    <div className="flex items-center gap-2 font-medium text-ink">
                      {r.username}
                      {r.is_master && (
                        <Badge tone="accent">
                          <Crown className="size-3" aria-hidden />
                          Master
                        </Badge>
                      )}
                    </div>
                    <div className="text-[12px] text-muted">{r.email}</div>
                  </Td>
                  {isRoot && <Td className="text-muted">{r.owner_name || "root"}</Td>}
                  <Td>
                    <Usage used={r.accounts} limit={r.max_accounts} />
                  </Td>
                  <Td>
                    <Usage used={r.disk_allocated_mb} limit={r.max_disk_mb} fmt={formatMB} />
                    <div className="mt-0.5 text-[11.5px] text-subtle">{formatMB(r.disk_used_mb)} in use</div>
                  </Td>
                  <Td className="text-muted">{r.is_master ? `${r.resellers} / ${formatLimit(r.max_resellers)}` : "—"}</Td>
                  <Td>
                    <Badge tone={r.suspended ? "bad" : "ok"}>{r.suspended ? "Suspended" : "Active"}</Badge>
                  </Td>
                  <Td>
                    <div className="flex justify-end">
                      <IconButton
                        icon={Pencil}
                        label={`Edit ${r.username}`}
                        onClick={() => setForm({ id: r.id, username: r.username, email: r.email, password: "", is_master: r.is_master, max_accounts: r.max_accounts, max_disk_mb: r.max_disk_mb, max_resellers: r.max_resellers })}
                      />
                      <IconButton icon={Ban} label={r.suspended ? `Reactivate ${r.username}` : `Suspend ${r.username}`} onClick={() => toggle.mutate(r)} />
                      <IconButton icon={Trash2} label={`Delete ${r.username}`} onClick={() => setDel(r)} />
                    </div>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <Modal
        open={!!form}
        onClose={() => setForm(null)}
        title={form?.id ? `Edit ${form.username}` : "Create reseller"}
        width="max-w-lg"
        footer={
          <>
            <Button onClick={() => setForm(null)}>Cancel</Button>
            <Button variant="primary" loading={save.isPending} disabled={!form?.email || (!form?.id && (!form?.username || !form?.password))} onClick={() => form && save.mutate(form)}>
              {form?.id ? "Save changes" : "Create reseller"}
            </Button>
          </>
        }
      >
        {form && (
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-3">
              <Field label="Username">
                {(id) => <Input id={id} value={form.username} disabled={!!form.id} onChange={(e) => setForm({ ...form, username: e.target.value.toLowerCase() })} />}
              </Field>
              <Field label="Email">{(id) => <Input id={id} type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />}</Field>
            </div>
            <Field label={form.id ? "New password" : "Password"} hint={form.id ? "Leave blank to keep the current password." : "At least 12 characters."}>
              {(id) => <Input id={id} type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} autoComplete="new-password" />}
            </Field>
            <fieldset>
              <legend className="mb-2 text-[11.5px] font-semibold uppercase tracking-wide text-muted">Allocation (0 = unlimited)</legend>
              <div className="grid grid-cols-2 gap-3">
                {num("max_accounts", "Max accounts")}
                {num("max_disk_mb", "Disk to allocate (MB)", "Sum of package disk limits")}
              </div>
            </fieldset>
            {isRoot && (
              <div className="rounded-md border border-line bg-surface2 p-3">
                <label className="flex items-center gap-2.5 text-[13px] font-medium">
                  <Switch label="Master reseller" checked={form.is_master} onChange={(v) => setForm({ ...form, is_master: v })} />
                  Master reseller
                </label>
                <p className="mt-1 text-[12px] text-muted">Can create and manage resellers of its own.</p>
                {form.is_master && <div className="mt-3 max-w-[12rem]">{num("max_resellers", "Max sub-resellers")}</div>}
              </div>
            )}
          </div>
        )}
      </Modal>

      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        onConfirm={() => del && remove.mutate(del.id)}
        loading={remove.isPending}
        title={`Delete ${del?.username}?`}
        message="Only resellers without accounts or sub-resellers can be deleted. Their packages are removed too."
      />
    </>
  );
}
