import { Globe, Lock, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import type { Domain } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button, IconButton } from "@/components/ui/Button";
import { Field, Input, Select, Switch } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, Mono, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

const kindLabel: Record<Domain["kind"], string> = { primary: "Primary", addon: "Addon", subdomain: "Subdomain", alias: "Alias" };

export default function Domains() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/domains`;
  const q = useGet<Domain[]>(key);
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<Domain | null>(null);
  const [form, setForm] = useState({ name: "", kind: "addon", doc_root: "" });

  const add = useMutate(() => api.post(key, form), {
    invalidate: [key],
    success: "Domain added",
    onSuccess: () => {
      setAdding(false);
      setForm({ name: "", kind: "addon", doc_root: "" });
    },
  });
  const update = useMutate((v: { id: string; body: object }) => api.patch(`${key}/${v.id}`, v.body), { invalidate: [key], success: "Domain updated" });
  const remove = useMutate((id: string) => api.del(`${key}/${id}`), { invalidate: [key], success: "Domain removed", onSuccess: () => setRemoving(null) });

  return (
    <>
      <PageHeader
        title="Domains"
        description="Manage the domains, subdomains and aliases served from this account."
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setAdding(true)}>
            Add domain
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
          <EmptyState icon={Globe} title="No domains yet" description="Add a domain to start hosting a website or email." />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Domain</Th>
                <Th>Type</Th>
                <Th>Document root</Th>
                <Th>PHP</Th>
                <Th>Force HTTPS</Th>
                <Th className="w-10">
                  <span className="sr-only">Actions</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((d) => (
                <Tr key={d.id}>
                  <Td>
                    <div className="flex items-center gap-2">
                      <span className="font-medium text-ink">{d.name}</span>
                      {d.has_ssl && (
                        <span title="SSL certificate installed" className="text-ok">
                          <Lock className="size-3.5" aria-label="SSL active" />
                        </span>
                      )}
                    </div>
                  </Td>
                  <Td>
                    <Badge tone={d.kind === "primary" ? "accent" : "neutral"}>{kindLabel[d.kind]}</Badge>
                  </Td>
                  <Td>
                    <Mono className="text-muted">{d.doc_root}</Mono>
                  </Td>
                  <Td>
                    <Select
                      aria-label={`PHP version for ${d.name}`}
                      className="h-7 w-20"
                      value={d.php_version}
                      onChange={(e) => update.mutate({ id: d.id, body: { php_version: e.target.value } })}
                    >
                      {["8.3", "8.2", "8.1"].map((v) => (
                        <option key={v}>{v}</option>
                      ))}
                    </Select>
                  </Td>
                  <Td>
                    <Switch
                      label={`Force HTTPS for ${d.name}`}
                      checked={d.force_https}
                      disabled={!d.has_ssl}
                      onChange={(v) => update.mutate({ id: d.id, body: { force_https: v } })}
                    />
                  </Td>
                  <Td>
                    {d.kind !== "primary" && <IconButton icon={Trash2} label={`Remove ${d.name}`} onClick={() => setRemoving(d)} />}
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <Modal
        open={adding}
        onClose={() => setAdding(false)}
        title="Add domain"
        footer={
          <>
            <Button onClick={() => setAdding(false)}>Cancel</Button>
            <Button variant="primary" loading={add.isPending} onClick={() => add.mutate()} disabled={!form.name}>
              Add domain
            </Button>
          </>
        }
      >
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            add.mutate();
          }}
        >
          <Field label="Domain name" hint="For example example.com or blog.example.com">
            {(id) => <Input id={id} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="example.com" autoFocus />}
          </Field>
          <Field label="Type">
            {(id) => (
              <Select id={id} value={form.kind} onChange={(e) => setForm({ ...form, kind: e.target.value })}>
                <option value="addon">Addon domain (separate site)</option>
                <option value="subdomain">Subdomain of an existing domain</option>
                <option value="alias">Alias (serves the same content)</option>
              </Select>
            )}
          </Field>
          <Field label="Document root" hint="Relative to your home directory. Leave blank to use public_html/<domain>.">
            {(id) => <Input id={id} value={form.doc_root} onChange={(e) => setForm({ ...form, doc_root: e.target.value })} placeholder="public_html/example.com" />}
          </Field>
        </form>
      </Modal>

      <ConfirmDialog
        open={!!removing}
        onClose={() => setRemoving(null)}
        onConfirm={() => removing && remove.mutate(removing.id)}
        loading={remove.isPending}
        title={`Remove ${removing?.name}?`}
        message="The site will stop being served and its DNS zone will be deleted. Files in the document root are kept."
        confirmLabel="Remove domain"
      />
    </>
  );
}
