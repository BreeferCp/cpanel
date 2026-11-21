import { ArrowRight, KeyRound, Mail, Plus, ShieldCheck, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { formatMB } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { Domain, MailAlias, Mailbox } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button, IconButton } from "@/components/ui/Button";
import { Field, Input, Select } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, Meter, PageHeader, Table, Tabs, Td, Th, Tr } from "@/components/ui/Layout";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

type Tab = "mailboxes" | "aliases" | "deliverability";

export default function Email() {
  const aid = useAccountId();
  const mbKey = `/accounts/${aid}/mailboxes`;
  const alKey = `/accounts/${aid}/mail-aliases`;
  const boxes = useGet<Mailbox[]>(mbKey);
  const aliases = useGet<MailAlias[]>(alKey);
  const domains = useGet<Domain[]>(`/accounts/${aid}/domains`);
  const [tab, setTab] = useState<Tab>("mailboxes");

  const [modal, setModal] = useState<"box" | "alias" | null>(null);
  const [f, setF] = useState({ local: "", domain: "", password: "", quota: "1024", dest: "" });
  const [pw, setPw] = useState<Mailbox | null>(null);
  const [del, setDel] = useState<{ kind: "box" | "alias"; id: string; label: string } | null>(null);

  const close = () => {
    setModal(null);
    setPw(null);
    setF({ local: "", domain: "", password: "", quota: "1024", dest: "" });
  };

  const createBox = useMutate(
    () => api.post(mbKey, { local_part: f.local, domain_id: f.domain, password: f.password, quota_mb: Number(f.quota) }),
    { invalidate: [mbKey], success: "Mailbox created", onSuccess: close },
  );
  const changePw = useMutate((id: string) => api.put(`${mbKey}/${id}/password`, { password: f.password }), { success: "Password changed", onSuccess: close });
  const createAlias = useMutate(() => api.post(alKey, { local_part: f.local, domain_id: f.domain, destination: f.dest }), {
    invalidate: [alKey],
    success: "Forwarder created",
    onSuccess: close,
  });
  const doDelete = useMutate(() => api.del(`${del!.kind === "box" ? mbKey : alKey}/${del!.id}`), {
    invalidate: [mbKey, alKey],
    success: "Deleted",
    onSuccess: () => setDel(null),
  });
  const dkim = useMutate((domain: string) => api.post(`/accounts/${aid}/mail/dkim`, { domain }), {
    success: "DKIM and DMARC records published",
  });

  const mailDomains = domains.data?.filter((d) => d.kind === "primary" || d.kind === "addon") ?? [];

  return (
    <>
      <PageHeader
        title="Email"
        description="Mailboxes, forwarders and sender authentication for your domains."
        actions={
          tab === "aliases" ? (
            <Button variant="primary" icon={Plus} onClick={() => setModal("alias")}>
              New forwarder
            </Button>
          ) : tab === "mailboxes" ? (
            <Button variant="primary" icon={Plus} onClick={() => setModal("box")}>
              New mailbox
            </Button>
          ) : undefined
        }
      />
      <Tabs
        tabs={[
          { id: "mailboxes", label: "Mailboxes" },
          { id: "aliases", label: "Forwarders" },
          { id: "deliverability", label: "Deliverability" },
        ]}
        value={tab}
        onChange={setTab}
      />

      {tab === "mailboxes" && (
        <Card flush>
          {boxes.isLoading ? (
            <Loading />
          ) : boxes.error ? (
            <div className="p-4">
              <ErrorNote error={boxes.error} />
            </div>
          ) : !boxes.data?.length ? (
            <EmptyState icon={Mail} title="No mailboxes" description="Create a mailbox to send and receive email on your domain." />
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Address</Th>
                  <Th className="w-64">Storage</Th>
                  <Th>Status</Th>
                  <Th className="w-24">
                    <span className="sr-only">Actions</span>
                  </Th>
                </tr>
              </thead>
              <tbody>
                {boxes.data.map((m) => (
                  <Tr key={m.id}>
                    <Td className="font-medium text-ink">{m.address}</Td>
                    <Td>
                      <Meter used={m.used_mb} limit={m.quota_mb} format={formatMB} />
                    </Td>
                    <Td>
                      <Badge tone={m.active ? "ok" : "neutral"}>{m.active ? "Active" : "Disabled"}</Badge>
                    </Td>
                    <Td>
                      <div className="flex justify-end">
                        <IconButton icon={KeyRound} label={`Change password for ${m.address}`} onClick={() => setPw(m)} />
                        <IconButton icon={Trash2} label={`Delete ${m.address}`} onClick={() => setDel({ kind: "box", id: m.id, label: m.address })} />
                      </div>
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>
      )}

      {tab === "aliases" && (
        <Card flush>
          {aliases.isLoading ? (
            <Loading />
          ) : !aliases.data?.length ? (
            <EmptyState icon={ArrowRight} title="No forwarders" description="Forward mail for one address to another. Use * as the name for a catch-all." />
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>From</Th>
                  <Th>To</Th>
                  <Th className="w-10">
                    <span className="sr-only">Actions</span>
                  </Th>
                </tr>
              </thead>
              <tbody>
                {aliases.data.map((a) => (
                  <Tr key={a.id}>
                    <Td className="font-medium text-ink">{a.source}</Td>
                    <Td className="text-muted">{a.destination}</Td>
                    <Td>
                      <IconButton icon={Trash2} label={`Delete forwarder ${a.source}`} onClick={() => setDel({ kind: "alias", id: a.id, label: a.source })} />
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>
      )}

      {tab === "deliverability" && (
        <Card title="Sender authentication" description="DKIM signs outgoing mail and DMARC tells receivers how to treat failures. Both records are published to your DNS zone automatically.">
          {mailDomains.length === 0 ? (
            <p className="text-[13px] text-muted">Add a domain first.</p>
          ) : (
            <ul className="divide-y divide-line">
              {mailDomains.map((d) => (
                <li key={d.id} className="flex items-center justify-between py-3">
                  <div className="flex items-center gap-2.5">
                    <ShieldCheck className="size-4 text-subtle" aria-hidden />
                    <span className="text-[13px] font-medium text-ink">{d.name}</span>
                  </div>
                  <Button size="sm" loading={dkim.isPending && dkim.variables === d.name} onClick={() => dkim.mutate(d.name)}>
                    Enable / rotate DKIM
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </Card>
      )}

      <Modal
        open={modal === "box"}
        onClose={close}
        title="New mailbox"
        footer={
          <>
            <Button onClick={close}>Cancel</Button>
            <Button variant="primary" loading={createBox.isPending} disabled={!f.local || !f.domain || !f.password} onClick={() => createBox.mutate()}>
              Create mailbox
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="grid grid-cols-[1fr_auto_1fr] items-end gap-2">
            <Field label="Username">{(id) => <Input id={id} value={f.local} onChange={(e) => setF({ ...f, local: e.target.value })} autoFocus />}</Field>
            <span className="pb-1.5 text-muted">@</span>
            <Field label="Domain">
              {(id) => (
                <Select id={id} value={f.domain} onChange={(e) => setF({ ...f, domain: e.target.value })}>
                  <option value="">Select…</option>
                  {mailDomains.map((d) => (
                    <option key={d.id} value={d.id}>
                      {d.name}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
          </div>
          <Field label="Password" hint="At least 12 characters.">
            {(id) => <Input id={id} type="password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} autoComplete="new-password" />}
          </Field>
          <Field label="Storage quota (MB)">
            {(id) => <Input id={id} type="number" min={1} value={f.quota} onChange={(e) => setF({ ...f, quota: e.target.value })} />}
          </Field>
        </div>
      </Modal>

      <Modal
        open={modal === "alias"}
        onClose={close}
        title="New forwarder"
        footer={
          <>
            <Button onClick={close}>Cancel</Button>
            <Button variant="primary" loading={createAlias.isPending} disabled={!f.local || !f.domain || !f.dest} onClick={() => createAlias.mutate()}>
              Create forwarder
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="grid grid-cols-[1fr_auto_1fr] items-end gap-2">
            <Field label="Forward from">{(id) => <Input id={id} value={f.local} onChange={(e) => setF({ ...f, local: e.target.value })} placeholder="info or *" autoFocus />}</Field>
            <span className="pb-1.5 text-muted">@</span>
            <Field label="Domain">
              {(id) => (
                <Select id={id} value={f.domain} onChange={(e) => setF({ ...f, domain: e.target.value })}>
                  <option value="">Select…</option>
                  {mailDomains.map((d) => (
                    <option key={d.id} value={d.id}>
                      {d.name}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
          </div>
          <Field label="Forward to">
            {(id) => <Input id={id} type="email" value={f.dest} onChange={(e) => setF({ ...f, dest: e.target.value })} placeholder="you@example.com" />}
          </Field>
        </div>
      </Modal>

      <Modal
        open={!!pw}
        onClose={close}
        title={`Change password for ${pw?.address}`}
        footer={
          <>
            <Button onClick={close}>Cancel</Button>
            <Button variant="primary" loading={changePw.isPending} disabled={!f.password} onClick={() => pw && changePw.mutate(pw.id)}>
              Change password
            </Button>
          </>
        }
      >
        <Field label="New password">
          {(id) => <Input id={id} type="password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} autoComplete="new-password" autoFocus />}
        </Field>
      </Modal>

      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        onConfirm={() => doDelete.mutate()}
        loading={doDelete.isPending}
        title={`Delete ${del?.label}?`}
        message={del?.kind === "box" ? "All mail stored in this mailbox will be permanently deleted." : "Mail will no longer be forwarded."}
      />
    </>
  );
}
