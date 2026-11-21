import { ArrowLeft, LifeBuoy, Plus, Send } from "lucide-react";
import { useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api } from "@/lib/api";
import { formatDate, timeAgo } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { Ticket, TicketMessage } from "@/lib/types";
import { useAuth } from "@/state/auth";
import { Button } from "@/components/ui/Button";
import { Field, Input, Select, Textarea } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, PageHeader, Table, Tabs, Td, Th, Tr } from "@/components/ui/Layout";
import { Modal } from "@/components/ui/Modal";

const statusLabel: Record<Ticket["status"], string> = { open: "Open", answered: "Answered", customer_reply: "Customer reply", closed: "Closed" };
const statusTone = { open: "accent", answered: "ok", customer_reply: "warn", closed: "neutral" } as const;
const prioTone = { low: "neutral", normal: "neutral", high: "warn", urgent: "bad" } as const;

export default function Tickets() {
  const [params, setParams] = useSearchParams();
  const id = params.get("id");
  return id ? <TicketView id={id} back={() => setParams({})} /> : <TicketList open={(t) => setParams({ id: t })} />;
}

function TicketList({ open }: { open: (id: string) => void }) {
  const { user } = useAuth();
  const staff = user?.role !== "user";
  const [filter, setFilter] = useState<"open" | "all">("open");
  const key = `/tickets${filter === "open" ? "?status=open" : ""}`;
  const q = useGet<Ticket[]>(key);
  const [creating, setCreating] = useState(false);
  const [f, setF] = useState({ subject: "", department: "support", priority: "normal", body: "" });
  const create = useMutate(() => api.post<Ticket>("/tickets", f), {
    invalidate: [key],
    success: "Ticket opened",
    onSuccess: (t) => {
      setCreating(false);
      setF({ subject: "", department: "support", priority: "normal", body: "" });
      open(t.id);
    },
  });

  return (
    <>
      <PageHeader
        title="Support"
        description={staff ? "Tickets from your customers, and any you opened with your provider." : "Ask for help from your hosting provider."}
        actions={
          user?.role !== "root" && (
            <Button variant="primary" icon={Plus} onClick={() => setCreating(true)}>
              New ticket
            </Button>
          )
        }
      />
      <Tabs
        tabs={[
          { id: "open", label: "Active" },
          { id: "all", label: "All tickets" },
        ]}
        value={filter}
        onChange={setFilter}
      />
      <Card flush>
        {q.isLoading ? (
          <Loading />
        ) : q.error ? (
          <div className="p-4">
            <ErrorNote error={q.error} />
          </div>
        ) : !q.data?.length ? (
          <EmptyState icon={LifeBuoy} title="No tickets" description={filter === "open" ? "Nothing needs attention." : "No tickets yet."} />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th className="w-20">#</Th>
                <Th>Subject</Th>
                {staff && <Th>Customer</Th>}
                <Th>Priority</Th>
                <Th>Status</Th>
                <Th>Updated</Th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((t) => (
                <Tr key={t.id} className="cursor-pointer" onClick={() => open(t.id)}>
                  <Td className="tabular-nums text-muted">{t.number}</Td>
                  <Td>
                    <button className="text-left font-medium text-ink hover:text-accent" onClick={() => open(t.id)}>
                      {t.subject}
                    </button>
                    <div className="text-[12px] capitalize text-subtle">{t.department}</div>
                  </Td>
                  {staff && <Td className="text-muted">{t.mine ? "You" : t.customer}</Td>}
                  <Td>
                    <Badge tone={prioTone[t.priority]}>{t.priority}</Badge>
                  </Td>
                  <Td>
                    <Badge tone={statusTone[t.status]}>{statusLabel[t.status]}</Badge>
                  </Td>
                  <Td className="text-muted">{timeAgo(t.updated_at)}</Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <Modal
        open={creating}
        onClose={() => setCreating(false)}
        title="New support ticket"
        width="max-w-xl"
        footer={
          <>
            <Button onClick={() => setCreating(false)}>Cancel</Button>
            <Button variant="primary" loading={create.isPending} disabled={!f.subject.trim() || !f.body.trim()} onClick={() => create.mutate()}>
              Open ticket
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="Subject">{(id) => <Input id={id} value={f.subject} onChange={(e) => setF({ ...f, subject: e.target.value })} autoFocus />}</Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Department">
              {(id) => (
                <Select id={id} value={f.department} onChange={(e) => setF({ ...f, department: e.target.value })}>
                  <option value="support">Technical support</option>
                  <option value="billing">Billing</option>
                  <option value="sales">Sales</option>
                  <option value="abuse">Abuse</option>
                </Select>
              )}
            </Field>
            <Field label="Priority">
              {(id) => (
                <Select id={id} value={f.priority} onChange={(e) => setF({ ...f, priority: e.target.value })}>
                  <option value="low">Low</option>
                  <option value="normal">Normal</option>
                  <option value="high">High</option>
                  <option value="urgent">Urgent</option>
                </Select>
              )}
            </Field>
          </div>
          <Field label="Message">
            {(id) => <Textarea id={id} rows={8} className="font-sans text-[13px]" value={f.body} onChange={(e) => setF({ ...f, body: e.target.value })} />}
          </Field>
        </div>
      </Modal>
    </>
  );
}

function TicketView({ id, back }: { id: string; back: () => void }) {
  const key = `/tickets/${id}`;
  const q = useGet<{ ticket: Ticket; messages: TicketMessage[]; can_answer: boolean }>(key);
  const [reply, setReply] = useState("");
  const send = useMutate(() => api.post(`${key}/replies`, { body: reply }), { invalidate: [key, "/tickets", "/tickets?status=open"], success: "Reply sent", onSuccess: () => setReply("") });
  const setStatus = useMutate((status: string) => api.patch(key, { status }), { invalidate: [key, "/tickets", "/tickets?status=open"] });

  if (q.isLoading) return <Loading />;
  if (q.error || !q.data) return <ErrorNote error={q.error} />;
  const { ticket: t, messages } = q.data;

  return (
    <>
      <button onClick={back} className="mb-3 inline-flex items-center gap-1 text-[13px] text-muted hover:text-ink">
        <ArrowLeft className="size-3.5" aria-hidden /> All tickets
      </button>
      <PageHeader
        title={`#${t.number} ${t.subject}`}
        description={`${t.department} · opened ${formatDate(t.created_at)} by ${t.customer}`}
        actions={
          <>
            <Badge tone={prioTone[t.priority]}>{t.priority}</Badge>
            <Badge tone={statusTone[t.status]}>{statusLabel[t.status]}</Badge>
            {t.status === "closed" ? (
              <Button size="sm" onClick={() => setStatus.mutate("open")}>
                Reopen
              </Button>
            ) : (
              <Button size="sm" onClick={() => setStatus.mutate("closed")}>
                Close ticket
              </Button>
            )}
          </>
        }
      />
      <div className="max-w-3xl space-y-3">
        {messages.map((m) => (
          <article key={m.id} className={`rounded-lg border p-4 ${m.staff ? "border-accent/30 bg-accent-soft/40" : "border-line bg-surface"}`}>
            <header className="mb-2 flex items-center justify-between text-[12.5px]">
              <span className="font-medium text-ink">
                {m.author}
                {m.staff && (
                  <span className="ml-2">
                    <Badge tone="accent">Staff</Badge>
                  </span>
                )}
              </span>
              <time className="text-muted">{formatDate(m.created_at)}</time>
            </header>
            <div className="whitespace-pre-wrap break-words text-[13.5px] leading-6 text-ink">{m.body}</div>
          </article>
        ))}
        {t.status !== "closed" && (
          <Card>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                if (reply.trim()) send.mutate();
              }}
            >
              <Textarea aria-label="Reply" rows={5} className="font-sans text-[13px]" placeholder="Write a reply…" value={reply} onChange={(e) => setReply(e.target.value)} />
              <div className="mt-3 flex justify-end">
                <Button type="submit" variant="primary" icon={Send} loading={send.isPending} disabled={!reply.trim()}>
                  Send reply
                </Button>
              </div>
            </form>
          </Card>
        )}
      </div>
    </>
  );
}
