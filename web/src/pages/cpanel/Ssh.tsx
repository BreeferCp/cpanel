import { KeyRound, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { Account, SshKey } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button, IconButton } from "@/components/ui/Button";
import { Field, Input, Textarea } from "@/components/ui/Form";
import { Card, EmptyState, ErrorNote, Loading, Mono, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { CopyField } from "@/components/ui/CopyField";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

export default function Ssh() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/ssh-keys`;
  const q = useGet<SshKey[]>(key);
  const acct = useGet<Account>(`/accounts/${aid}`);
  const [adding, setAdding] = useState(false);
  const [f, setF] = useState({ name: "", public_key: "" });
  const [del, setDel] = useState<SshKey | null>(null);

  const add = useMutate(() => api.post(key, f), {
    invalidate: [key],
    success: "Key authorized",
    onSuccess: () => {
      setAdding(false);
      setF({ name: "", public_key: "" });
    },
  });
  const remove = useMutate((id: string) => api.del(`${key}/${id}`), { invalidate: [key], success: "Key removed", onSuccess: () => setDel(null) });

  return (
    <>
      <PageHeader
        title="SSH Access"
        description="Authorize public keys for SSH and SFTP access to your account."
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setAdding(true)}>
            Authorize key
          </Button>
        }
      />
      {acct.data && (
        <Card title="Connect" className="mb-5">
          <p className="mb-2 text-[13px] text-muted">Use any authorized key to connect:</p>
          <CopyField value={`ssh ${acct.data.sys_user}@${acct.data.primary_domain}`} />
        </Card>
      )}
      <Card title="Authorized keys" flush>
        {q.isLoading ? (
          <Loading />
        ) : q.error ? (
          <div className="p-4">
            <ErrorNote error={q.error} />
          </div>
        ) : !q.data?.length ? (
          <EmptyState icon={KeyRound} title="No keys authorized" description="Add the public half of an Ed25519 or RSA key pair." />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Name</Th>
                <Th>Fingerprint</Th>
                <Th>Added</Th>
                <Th className="w-10">
                  <span className="sr-only">Actions</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((k) => (
                <Tr key={k.id}>
                  <Td className="font-medium text-ink">{k.name}</Td>
                  <Td>
                    <Mono className="text-muted">{k.fingerprint}</Mono>
                  </Td>
                  <Td className="text-muted">{formatDate(k.created_at)}</Td>
                  <Td>
                    <IconButton icon={Trash2} label={`Remove ${k.name}`} onClick={() => setDel(k)} />
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
        title="Authorize SSH key"
        width="max-w-xl"
        footer={
          <>
            <Button onClick={() => setAdding(false)}>Cancel</Button>
            <Button variant="primary" loading={add.isPending} disabled={!f.name || !f.public_key} onClick={() => add.mutate()}>
              Authorize key
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="Name" hint="For example: work laptop">
            {(id) => <Input id={id} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} autoFocus />}
          </Field>
          <Field label="Public key" hint="Starts with ssh-ed25519, ssh-rsa or ecdsa-sha2-…">
            {(id) => <Textarea id={id} rows={4} value={f.public_key} onChange={(e) => setF({ ...f, public_key: e.target.value })} spellCheck={false} />}
          </Field>
        </div>
      </Modal>

      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        onConfirm={() => del && remove.mutate(del.id)}
        loading={remove.isPending}
        title={`Remove ${del?.name}?`}
        message="Anyone holding the matching private key will lose access."
        confirmLabel="Remove key"
      />
    </>
  );
}
