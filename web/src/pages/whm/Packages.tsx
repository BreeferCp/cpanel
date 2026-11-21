import { Package as PackageIcon, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { formatLimit } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { Pkg } from "@/lib/types";
import { Button, IconButton } from "@/components/ui/Button";
import { Field, Input, Switch } from "@/components/ui/Form";
import { Card, EmptyState, ErrorNote, Loading, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

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
};

type NumKey = { [K in keyof Pkg]: Pkg[K] extends number ? K : never }[keyof Pkg];

export default function Packages() {
  const q = useGet<Pkg[]>("/packages");
  const [edit, setEdit] = useState<Pkg | (Omit<Pkg, "id"> & { id?: undefined }) | null>(null);
  const [del, setDel] = useState<Pkg | null>(null);

  const save = useMutate(
    (p: Omit<Pkg, "id"> & { id?: string }) => (p.id ? api.put(`/packages/${p.id}`, p) : api.post("/packages", p)),
    { invalidate: ["/packages"], success: "Package saved", onSuccess: () => setEdit(null) },
  );
  const remove = useMutate((id: string) => api.del(`/packages/${id}`), { invalidate: ["/packages"], success: "Package deleted", onSuccess: () => setDel(null) });

  const num = (key: NumKey, label: string, hint?: string) => (
    <Field label={label} hint={hint}>
      {(id) => (
        <Input
          id={id}
          type="number"
          min={0}
          value={edit ? String(edit[key]) : ""}
          onChange={(e) => edit && setEdit({ ...edit, [key]: Number(e.target.value) })}
        />
      )}
    </Field>
  );

  return (
    <>
      <PageHeader
        title="Packages"
        description="Resource plans assigned to accounts. A limit of 0 means unlimited."
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setEdit({ ...blank })}>
            Create package
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
          <EmptyState icon={PackageIcon} title="No packages" description="Packages define disk, bandwidth, CPU and memory limits for each account." />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Name</Th>
                <Th>Disk</Th>
                <Th>Domains</Th>
                <Th>Databases</Th>
                <Th>Mailboxes</Th>
                <Th>CPU</Th>
                <Th>Memory</Th>
                <Th className="w-20">
                  <span className="sr-only">Actions</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((p) => (
                <Tr key={p.id}>
                  <Td className="font-medium text-ink">{p.name}</Td>
                  <Td>{formatLimit(p.disk_mb, "mb")}</Td>
                  <Td>{formatLimit(p.max_domains)}</Td>
                  <Td>{formatLimit(p.max_databases)}</Td>
                  <Td>{formatLimit(p.max_mailboxes)}</Td>
                  <Td>{p.cpu_percent / 100} core{p.cpu_percent === 100 ? "" : "s"}</Td>
                  <Td>{formatLimit(p.memory_mb, "mb")}</Td>
                  <Td>
                    <div className="flex justify-end">
                      <IconButton icon={Pencil} label={`Edit ${p.name}`} onClick={() => setEdit(p)} />
                      <IconButton icon={Trash2} label={`Delete ${p.name}`} onClick={() => setDel(p)} />
                    </div>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <Modal
        open={!!edit}
        onClose={() => setEdit(null)}
        width="max-w-2xl"
        title={edit?.id ? "Edit package" : "Create package"}
        footer={
          <>
            <Button onClick={() => setEdit(null)}>Cancel</Button>
            <Button variant="primary" loading={save.isPending} disabled={!edit?.name} onClick={() => edit && save.mutate(edit)}>
              Save package
            </Button>
          </>
        }
      >
        {edit && (
          <div className="space-y-5">
            <Field label="Package name">
              {(id) => <Input id={id} value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} autoFocus />}
            </Field>
            <fieldset>
              <legend className="mb-2 text-[11.5px] font-semibold uppercase tracking-wide text-muted">Quotas</legend>
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
                {num("disk_mb", "Disk (MB)")}
                {num("bandwidth_mb", "Bandwidth (MB/mo)")}
                {num("max_domains", "Domains")}
                {num("max_databases", "Databases")}
                {num("max_mailboxes", "Mailboxes")}
                {num("max_cron", "Cron jobs")}
                {num("max_apps", "Applications")}
              </div>
            </fieldset>
            <fieldset>
              <legend className="mb-2 text-[11.5px] font-semibold uppercase tracking-wide text-muted">Isolation (cgroups v2)</legend>
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                {num("cpu_percent", "CPU (%)", "100 = one core")}
                {num("memory_mb", "Memory (MB)")}
                {num("io_weight", "I/O weight", "1–10000")}
                {num("max_procs", "Max processes")}
              </div>
            </fieldset>
            <div className="grid grid-cols-[1fr_auto] items-end gap-4">
              <Field label="PHP versions" hint="Comma separated; the first is the default.">
                {(id) => <Input id={id} value={edit.php_versions} onChange={(e) => setEdit({ ...edit, php_versions: e.target.value })} className="font-mono" />}
              </Field>
              <div className="flex items-center gap-2 pb-1.5">
                <Switch label="Shell access" checked={edit.shell_access} onChange={(v) => setEdit({ ...edit, shell_access: v })} />
                <span className="text-[13px]">Shell access</span>
              </div>
            </div>
          </div>
        )}
      </Modal>

      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        onConfirm={() => del && remove.mutate(del.id)}
        loading={remove.isPending}
        title={`Delete ${del?.name}?`}
        message="Packages that are still assigned to accounts cannot be deleted."
      />
    </>
  );
}
