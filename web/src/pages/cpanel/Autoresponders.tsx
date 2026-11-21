import { BellRing, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import type { Autoresponder, Mailbox } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button, IconButton } from "@/components/ui/Button";
import { Field, Input, Select, Textarea } from "@/components/ui/Form";
import { Badge, Card, EmptyState, Loading, Mono, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { Modal } from "@/components/ui/Modal";

const blank = { mailbox_id: "", subject: "Out of office", body: "Thank you for your email. I am away and will reply when I am back.", interval_days: 1, starts_on: "", ends_on: "" };

function activeWindow(a: Autoresponder): string {
  if (a.starts_on && a.ends_on) return `${a.starts_on} → ${a.ends_on}`;
  if (a.starts_on) return `From ${a.starts_on}`;
  if (a.ends_on) return `Until ${a.ends_on}`;
  return "Always on";
}

export default function Autoresponders() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/autoresponders`;
  const q = useGet<Autoresponder[]>(key);
  const boxes = useGet<Mailbox[]>(`/accounts/${aid}/mailboxes`);
  const [edit, setEdit] = useState<typeof blank | null>(null);
  const save = useMutate(() => api.put(`${key}/${edit?.mailbox_id}`, edit), { invalidate: [key], success: "Autoresponder saved", onSuccess: () => setEdit(null) });
  const remove = useMutate((id: string) => api.del(`${key}/${id}`), { invalidate: [key], success: "Autoresponder removed" });
  const today = new Date().toISOString().slice(0, 10);

  return (
    <>
      <PageHeader
        title="Autoresponders"
        description="Send an automatic reply to incoming mail, for example while you are on holiday. Each sender gets at most one reply per interval, and spam never gets a reply."
        actions={
          <Button variant="primary" icon={Plus} disabled={!boxes.data?.length} onClick={() => setEdit({ ...blank, mailbox_id: boxes.data?.[0]?.id ?? "" })}>
            Add autoresponder
          </Button>
        }
      />
      <Card flush>
        {q.isLoading ? (
          <Loading />
        ) : !q.data?.length ? (
          <EmptyState icon={BellRing} title="No autoresponders" description={boxes.data?.length ? "None of your mailboxes reply automatically." : "Create a mailbox first under Email Accounts."} />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Mailbox</Th>
                <Th>Subject</Th>
                <Th>Active</Th>
                <Th>Interval</Th>
                <Th className="w-20">
                  <span className="sr-only">Actions</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((a) => {
                const live = (!a.starts_on || a.starts_on <= today) && (!a.ends_on || a.ends_on >= today);
                return (
                  <Tr key={a.mailbox_id}>
                    <Td>
                      <Mono className="text-ink">{a.address}</Mono>
                    </Td>
                    <Td className="max-w-xs truncate">{a.subject}</Td>
                    <Td>
                      <Badge tone={live ? "ok" : "neutral"}>{live ? activeWindow(a) : `Scheduled: ${activeWindow(a)}`}</Badge>
                    </Td>
                    <Td>{a.interval_days === 1 ? "Daily" : `Every ${a.interval_days} days`}</Td>
                    <Td>
                      <div className="flex justify-end gap-1">
                        <IconButton icon={Pencil} label={`Edit ${a.address}`} onClick={() => setEdit({ ...a })} />
                        <IconButton icon={Trash2} label={`Remove ${a.address}`} onClick={() => remove.mutate(a.mailbox_id)} />
                      </div>
                    </Td>
                  </Tr>
                );
              })}
            </tbody>
          </Table>
        )}
      </Card>

      <Modal
        open={!!edit}
        onClose={() => setEdit(null)}
        title="Autoresponder"
        width="max-w-lg"
        footer={
          <>
            <Button onClick={() => setEdit(null)}>Cancel</Button>
            <Button variant="primary" loading={save.isPending} onClick={() => save.mutate()}>
              Save
            </Button>
          </>
        }
      >
        {edit && (
          <div className="space-y-4">
            <Field label="Mailbox">
              {(id) => (
                <Select id={id} value={edit.mailbox_id} onChange={(e) => setEdit({ ...edit, mailbox_id: e.target.value })}>
                  {boxes.data?.map((b) => (
                    <option key={b.id} value={b.id}>
                      {b.address}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            <Field label="Subject">{(id) => <Input id={id} value={edit.subject} onChange={(e) => setEdit({ ...edit, subject: e.target.value })} />}</Field>
            <Field label="Message">
              {(id) => <Textarea id={id} className="min-h-32 font-sans text-[13px]" value={edit.body} onChange={(e) => setEdit({ ...edit, body: e.target.value })} />}
            </Field>
            <div className="grid grid-cols-3 gap-3">
              <Field label="Starts">{(id) => <Input id={id} type="date" value={edit.starts_on} onChange={(e) => setEdit({ ...edit, starts_on: e.target.value })} />}</Field>
              <Field label="Ends">{(id) => <Input id={id} type="date" value={edit.ends_on} onChange={(e) => setEdit({ ...edit, ends_on: e.target.value })} />}</Field>
              <Field label="Reply every (days)">
                {(id) => <Input id={id} type="number" min={1} max={30} value={edit.interval_days} onChange={(e) => setEdit({ ...edit, interval_days: Number(e.target.value) })} />}
              </Field>
            </div>
          </div>
        )}
      </Modal>
    </>
  );
}
