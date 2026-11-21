import { FolderUp, KeyRound, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { formatMB } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { Domain, FtpAccount } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { usePanel } from "@/state/panel";
import { Button, IconButton } from "@/components/ui/Button";
import { CopyField } from "@/components/ui/CopyField";
import { Field, Input, Select } from "@/components/ui/Form";
import { Card, EmptyState, Loading, Mono, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

export default function Ftp() {
  const aid = useAccountId();
  const panel = usePanel();
  const key = `/accounts/${aid}/ftp`;
  const q = useGet<FtpAccount[]>(key);
  const domains = useGet<Domain[]>(`/accounts/${aid}/domains`);
  const [open, setOpen] = useState(false);
  const [f, setF] = useState({ name: "", domain: "", password: "", directory: "public_html", quota_mb: "0" });
  const [pw, setPw] = useState<FtpAccount | null>(null);
  const [newPw, setNewPw] = useState("");
  const [del, setDel] = useState<FtpAccount | null>(null);

  const create = useMutate(
    () => api.post(key, { ...f, domain: f.domain || domains.data?.[0]?.name, quota_mb: Number(f.quota_mb) }),
    { invalidate: [key], success: "FTP account created", onSuccess: () => (setOpen(false), setF({ ...f, name: "", password: "" })) },
  );
  const setPassword = useMutate(() => api.patch(`${key}/${pw?.id}`, { password: newPw }), {
    success: "Password changed",
    onSuccess: () => (setPw(null), setNewPw("")),
  });
  const remove = useMutate((id: string) => api.del(`${key}/${id}`), { invalidate: [key], success: "FTP account deleted", onSuccess: () => setDel(null) });
  const host = panel.hostname || domains.data?.[0]?.name || "your server";

  return (
    <>
      <PageHeader
        title="FTP Accounts"
        description="Give designers and tools file access to one folder of your account. Connections must use FTPS (explicit TLS)."
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setOpen(true)}>
            Add FTP account
          </Button>
        }
      />
      <div className="grid gap-4 lg:grid-cols-[1fr_300px]">
        <Card flush>
          {q.isLoading ? (
            <Loading />
          ) : !q.data?.length ? (
            <EmptyState icon={FolderUp} title="No FTP accounts" description="Your main login also works over SFTP if your plan includes it." />
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Login</Th>
                  <Th>Directory</Th>
                  <Th>Quota</Th>
                  <Th className="w-20">
                    <span className="sr-only">Actions</span>
                  </Th>
                </tr>
              </thead>
              <tbody>
                {q.data.map((a) => (
                  <Tr key={a.id}>
                    <Td>
                      <Mono className="text-ink">{a.login}</Mono>
                    </Td>
                    <Td>
                      <Mono className="text-muted">~/{a.directory}</Mono>
                    </Td>
                    <Td>{a.quota_mb ? formatMB(a.quota_mb) : "Unlimited"}</Td>
                    <Td>
                      <div className="flex justify-end gap-1">
                        <IconButton icon={KeyRound} label={`Change password for ${a.login}`} onClick={() => setPw(a)} />
                        <IconButton icon={Trash2} label={`Delete ${a.login}`} onClick={() => setDel(a)} />
                      </div>
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>
        <Card title="Client settings">
          <dl className="space-y-3 text-[13px]">
            <div>
              <dt className="mb-1 text-muted">Server</dt>
              <dd>
                <CopyField value={host} />
              </dd>
            </div>
            <div>
              <dt className="text-muted">Port</dt>
              <dd className="font-medium">21 (FTP with explicit TLS)</dd>
            </div>
            <div>
              <dt className="text-muted">Passive ports</dt>
              <dd className="font-medium">30000–30999</dd>
            </div>
            <div>
              <dt className="text-muted">Username</dt>
              <dd className="font-medium">The full login, e.g. upload@yourdomain.com</dd>
            </div>
          </dl>
        </Card>
      </div>

      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Add FTP account"
        footer={
          <>
            <Button onClick={() => setOpen(false)}>Cancel</Button>
            <Button variant="primary" loading={create.isPending} onClick={() => create.mutate()}>
              Create
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="grid grid-cols-[1fr_auto_1fr] items-end gap-2">
            <Field label="User name">{(id) => <Input id={id} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} autoFocus />}</Field>
            <span className="pb-2 text-muted">@</span>
            <Field label="Domain">
              {(id) => (
                <Select id={id} value={f.domain || domains.data?.[0]?.name} onChange={(e) => setF({ ...f, domain: e.target.value })}>
                  {domains.data?.map((d) => (
                    <option key={d.id}>{d.name}</option>
                  ))}
                </Select>
              )}
            </Field>
          </div>
          <Field label="Password" hint="At least 12 characters with mixed character types.">
            {(id) => <Input id={id} type="password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} autoComplete="new-password" />}
          </Field>
          <Field label="Directory" hint="Relative to your home. The account cannot see anything above it.">
            {(id) => <Input id={id} className="font-mono" value={f.directory} onChange={(e) => setF({ ...f, directory: e.target.value })} />}
          </Field>
          <Field label="Quota (MB)" hint="0 means no FTP quota (your account quota still applies).">
            {(id) => <Input id={id} type="number" min={0} value={f.quota_mb} onChange={(e) => setF({ ...f, quota_mb: e.target.value })} />}
          </Field>
        </div>
      </Modal>

      <Modal
        open={!!pw}
        onClose={() => setPw(null)}
        title={`Change password for ${pw?.login}`}
        footer={
          <>
            <Button onClick={() => setPw(null)}>Cancel</Button>
            <Button variant="primary" loading={setPassword.isPending} onClick={() => setPassword.mutate()}>
              Change password
            </Button>
          </>
        }
      >
        <Field label="New password">
          {(id) => <Input id={id} type="password" value={newPw} onChange={(e) => setNewPw(e.target.value)} autoComplete="new-password" autoFocus />}
        </Field>
      </Modal>

      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        onConfirm={() => del && remove.mutate(del.id)}
        loading={remove.isPending}
        title="Delete FTP account"
        message={
          <>
            <Mono>{del?.login}</Mono> will no longer be able to sign in. Files in its directory are kept.
          </>
        }
      />
    </>
  );
}
