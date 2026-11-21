import { AppWindow, Play, Plus, RotateCw, Square, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import type { App, Domain } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button, IconButton } from "@/components/ui/Button";
import { Field, Input, Select } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, Mono, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

const runtimeLabel = { node: "Node.js", python: "Python", docker: "Docker" } as const;
const empty = { name: "", runtime: "node", command: "server.js", image: "", domain_id: "" };

export default function Apps() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/apps`;
  const q = useGet<App[]>(key);
  const domains = useGet<Domain[]>(`/accounts/${aid}/domains`);
  const [adding, setAdding] = useState(false);
  const [f, setF] = useState(empty);
  const [del, setDel] = useState<App | null>(null);

  const create = useMutate(() => api.post(key, { ...f, port: 0 }), {
    invalidate: [key],
    success: "Application deployed",
    onSuccess: () => {
      setAdding(false);
      setF(empty);
    },
  });
  const act = useMutate((v: { id: string; action: "start" | "stop" | "restart" }) => api.post(`${key}/${v.id}/${v.action}`), { invalidate: [key] });
  const remove = useMutate((id: string) => api.del(`${key}/${id}`), { invalidate: [key], success: "Application removed", onSuccess: () => setDel(null) });

  return (
    <>
      <PageHeader
        title="Applications"
        description="Run Node.js, Python or container workloads inside your account's resource limits."
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setAdding(true)}>
            New application
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
          <EmptyState icon={AppWindow} title="No applications" description="Deploy a long-running app and attach it to a domain through the reverse proxy." />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Name</Th>
                <Th>Runtime</Th>
                <Th>Port</Th>
                <Th>Status</Th>
                <Th className="text-right">Actions</Th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((a) => (
                <Tr key={a.id}>
                  <Td className="font-medium text-ink">{a.name}</Td>
                  <Td>
                    <Badge>{runtimeLabel[a.runtime]}</Badge>
                  </Td>
                  <Td>
                    <Mono className="text-muted">127.0.0.1:{a.port}</Mono>
                  </Td>
                  <Td>
                    <Badge tone={a.status === "running" ? "ok" : "neutral"}>{a.status}</Badge>
                  </Td>
                  <Td>
                    <div className="flex justify-end">
                      {a.status === "running" ? (
                        <IconButton icon={Square} label={`Stop ${a.name}`} onClick={() => act.mutate({ id: a.id, action: "stop" })} />
                      ) : (
                        <IconButton icon={Play} label={`Start ${a.name}`} onClick={() => act.mutate({ id: a.id, action: "start" })} />
                      )}
                      <IconButton icon={RotateCw} label={`Restart ${a.name}`} onClick={() => act.mutate({ id: a.id, action: "restart" })} />
                      <IconButton icon={Trash2} label={`Remove ${a.name}`} onClick={() => setDel(a)} />
                    </div>
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
        title="New application"
        description="Code lives in apps/<name> inside your home directory."
        footer={
          <>
            <Button onClick={() => setAdding(false)}>Cancel</Button>
            <Button variant="primary" loading={create.isPending} disabled={!f.name || (f.runtime === "docker" ? !f.image : !f.command)} onClick={() => create.mutate()}>
              Deploy
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="Name" hint="Lowercase letters, digits and dashes.">
            {(id) => <Input id={id} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} autoFocus />}
          </Field>
          <Field label="Runtime">
            {(id) => (
              <Select id={id} value={f.runtime} onChange={(e) => setF({ ...f, runtime: e.target.value })}>
                <option value="node">Node.js</option>
                <option value="python">Python 3</option>
                <option value="docker">Docker container</option>
              </Select>
            )}
          </Field>
          {f.runtime === "docker" ? (
            <Field label="Image" hint="For example nginx:1.27 or ghcr.io/you/app:latest">
              {(id) => <Input id={id} value={f.image} onChange={(e) => setF({ ...f, image: e.target.value })} className="font-mono" />}
            </Field>
          ) : (
            <Field label="Start command" hint="Run from the app directory; the PORT environment variable is set for you.">
              {(id) => <Input id={id} value={f.command} onChange={(e) => setF({ ...f, command: e.target.value })} className="font-mono" />}
            </Field>
          )}
          <Field label="Serve on domain" hint="Optional. Proxies the domain to this app.">
            {(id) => (
              <Select id={id} value={f.domain_id} onChange={(e) => setF({ ...f, domain_id: e.target.value })}>
                <option value="">None</option>
                {domains.data?.map((d) => (
                  <option key={d.id} value={d.id}>
                    {d.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
        </div>
      </Modal>

      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        onConfirm={() => del && remove.mutate(del.id)}
        loading={remove.isPending}
        title={`Remove ${del?.name}?`}
        message="The process or container is stopped and removed. Files in apps/ are kept."
        confirmLabel="Remove"
      />
    </>
  );
}
