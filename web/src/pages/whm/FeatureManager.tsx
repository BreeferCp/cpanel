import { Pencil, Plus, ToggleRight, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import type { FeatureDef, FeatureList } from "@/lib/types";
import { Button, IconButton } from "@/components/ui/Button";
import { Checkbox, Field, Input } from "@/components/ui/Form";
import { Badge, Card, EmptyState, Loading, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { Modal } from "@/components/ui/Modal";

export default function FeatureManager() {
  const lists = useGet<FeatureList[]>("/feature-lists");
  const catalog = useGet<FeatureDef[]>("/features");
  const [edit, setEdit] = useState<{ id?: string; name: string; disabled: Set<string> } | null>(null);
  const groups = useMemo(() => {
    const m = new Map<string, FeatureDef[]>();
    for (const f of catalog.data ?? []) m.set(f.group, [...(m.get(f.group) ?? []), f]);
    return [...m.entries()];
  }, [catalog.data]);

  const save = useMutate(
    () => {
      const body = { name: edit!.name, disabled: [...edit!.disabled] };
      return edit!.id ? api.put(`/feature-lists/${edit!.id}`, body) : api.post("/feature-lists", body);
    },
    { invalidate: ["/feature-lists"], success: "Feature list saved", onSuccess: () => setEdit(null) },
  );
  const remove = useMutate((id: string) => api.del(`/feature-lists/${id}`), { invalidate: ["/feature-lists"], success: "Feature list deleted" });
  const toggle = (k: string, enabled: boolean) => {
    if (!edit) return;
    const d = new Set(edit.disabled);
    if (enabled) d.delete(k);
    else d.add(k);
    setEdit({ ...edit, disabled: d });
  };

  return (
    <>
      <PageHeader
        title="Feature Manager"
        description="A feature list chooses which cPanel tools a package includes. Assign it to packages under Packages. Disabled tools are hidden in cPanel and refused by the API."
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setEdit({ name: "", disabled: new Set() })}>
            Add feature list
          </Button>
        }
      />
      <Card flush>
        {lists.isLoading ? (
          <Loading />
        ) : !lists.data?.length ? (
          <EmptyState icon={ToggleRight} title="No feature lists" description="Packages without a feature list get every tool." />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Name</Th>
                <Th>Disabled tools</Th>
                <Th>Packages</Th>
                <Th className="w-20">
                  <span className="sr-only">Actions</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {lists.data.map((l) => (
                <Tr key={l.id}>
                  <Td className="font-medium text-ink">{l.name}</Td>
                  <Td>
                    {l.disabled.length ? (
                      <div className="flex flex-wrap gap-1">
                        {l.disabled.map((k) => (
                          <Badge key={k}>{catalog.data?.find((f) => f.key === k)?.label ?? k}</Badge>
                        ))}
                      </div>
                    ) : (
                      <span className="text-muted">None, all tools enabled</span>
                    )}
                  </Td>
                  <Td className="tabular-nums">{l.packages}</Td>
                  <Td>
                    <div className="flex justify-end gap-1">
                      <IconButton icon={Pencil} label={`Edit ${l.name}`} onClick={() => setEdit({ id: l.id, name: l.name, disabled: new Set(l.disabled) })} />
                      <IconButton icon={Trash2} label={`Delete ${l.name}`} disabled={l.packages > 0} onClick={() => remove.mutate(l.id)} />
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
        title={edit?.id ? `Edit ${edit.name}` : "New feature list"}
        footer={
          <>
            <span className="mr-auto text-[12.5px] text-muted">
              {catalog.data && edit ? `${catalog.data.length - edit.disabled.size} of ${catalog.data.length} tools enabled` : ""}
            </span>
            <Button onClick={() => setEdit(null)}>Cancel</Button>
            <Button variant="primary" loading={save.isPending} onClick={() => save.mutate()}>
              Save
            </Button>
          </>
        }
      >
        {edit && (
          <div className="space-y-4">
            <Field label="Name">{(id) => <Input id={id} value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} placeholder="Starter tools" autoFocus />}</Field>
            <div className="grid gap-4 sm:grid-cols-2">
              {groups.map(([g, fs]) => (
                <fieldset key={g} className="rounded-md border border-line p-3">
                  <legend className="px-1 text-[11.5px] font-semibold uppercase tracking-wider text-subtle">{g}</legend>
                  <div className="space-y-1.5">
                    {fs.map((f) => (
                      <label key={f.key} className="flex items-center gap-2 text-[13px]">
                        <Checkbox checked={!edit.disabled.has(f.key)} onChange={(e) => toggle(f.key, e.target.checked)} />
                        {f.label}
                      </label>
                    ))}
                  </div>
                </fieldset>
              ))}
            </div>
          </div>
        )}
      </Modal>
    </>
  );
}
