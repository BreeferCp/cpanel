import { Network, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { RemoteHost } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { usePanel } from "@/state/panel";
import { Button, IconButton } from "@/components/ui/Button";
import { CopyField } from "@/components/ui/CopyField";
import { Field, Input } from "@/components/ui/Form";
import { Card, EmptyState, Loading, Mono, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";

export default function RemoteMysql() {
  const aid = useAccountId();
  const panel = usePanel();
  const key = `/accounts/${aid}/remote-mysql`;
  const q = useGet<RemoteHost[]>(key);
  const [f, setF] = useState({ host: "", note: "" });
  const add = useMutate(() => api.post(key, f), { invalidate: [key], success: "Remote host added", onSuccess: () => setF({ host: "", note: "" }) });
  const remove = useMutate((id: string) => api.del(`${key}/${id}`), { invalidate: [key], success: "Remote host removed" });
  return (
    <>
      <PageHeader
        title="Remote MySQL"
        description="Allow programs on other computers to connect to your databases. Every database user gets access from these hosts, with its usual password and grants."
      />
      <div className="grid gap-4 lg:grid-cols-[1fr_300px]">
        <div className="space-y-4">
          <Card>
            <form
              className="flex flex-wrap items-end gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                add.mutate();
              }}
            >
              <Field label="Host" className="min-w-56 flex-1">
                {(id) => <Input id={id} className="font-mono" placeholder="203.0.113.10 or 203.0.113.%" value={f.host} onChange={(e) => setF({ ...f, host: e.target.value })} />}
              </Field>
              <Field label="Note" className="min-w-40 flex-1">
                {(id) => <Input id={id} placeholder="Office" value={f.note} onChange={(e) => setF({ ...f, note: e.target.value })} />}
              </Field>
              <Button type="submit" variant="primary" icon={Plus} loading={add.isPending} disabled={!f.host.trim()}>
                Add host
              </Button>
            </form>
          </Card>
          <Card flush>
            {q.isLoading ? (
              <Loading />
            ) : !q.data?.length ? (
              <EmptyState icon={Network} title="Only local connections" description="Your databases accept connections from this server only." />
            ) : (
              <Table>
                <thead>
                  <tr>
                    <Th>Host</Th>
                    <Th>Note</Th>
                    <Th>Added</Th>
                    <Th className="w-10">
                      <span className="sr-only">Actions</span>
                    </Th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.map((h) => (
                    <Tr key={h.id}>
                      <Td>
                        <Mono className="text-ink">{h.host}</Mono>
                      </Td>
                      <Td className="text-muted">{h.note || "—"}</Td>
                      <Td className="text-muted">{formatDate(h.created_at)}</Td>
                      <Td>
                        <IconButton icon={Trash2} label={`Remove ${h.host}`} onClick={() => remove.mutate(h.id)} />
                      </Td>
                    </Tr>
                  ))}
                </tbody>
              </Table>
            )}
          </Card>
        </div>
        <Card title="Connecting">
          <div className="space-y-3 text-[13px]">
            <div>
              <div className="mb-1 text-muted">Server</div>
              <CopyField value={panel.hostname || "your server's host name"} />
            </div>
            <div>
              <span className="text-muted">Port</span> <span className="font-medium">3306</span>
            </div>
            <p className="text-[12.5px] text-muted">Your provider must also open port 3306 in the server firewall for these addresses.</p>
          </div>
        </Card>
      </div>
    </>
  );
}
