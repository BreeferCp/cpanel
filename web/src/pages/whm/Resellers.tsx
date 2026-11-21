import { Plus, UsersRound } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import type { Reseller } from "@/lib/types";
import { Button } from "@/components/ui/Button";
import { Field, Input } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { Modal } from "@/components/ui/Modal";

export default function Resellers() {
  const q = useGet<Reseller[]>("/resellers");
  const [open, setOpen] = useState(false);
  const [f, setF] = useState({ username: "", email: "", password: "" });
  const create = useMutate(() => api.post("/resellers", f), {
    invalidate: ["/resellers"],
    success: "Reseller created",
    onSuccess: () => {
      setOpen(false);
      setF({ username: "", email: "", password: "" });
    },
  });

  return (
    <>
      <PageHeader
        title="Resellers"
        description="Resellers create and manage their own customers and packages, isolated from other resellers."
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setOpen(true)}>
            Create reseller
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
          <EmptyState icon={UsersRound} title="No resellers" description="Create a reseller to let someone else sell hosting from this server." />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Username</Th>
                <Th>Email</Th>
                <Th>Customers</Th>
                <Th>Status</Th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((r) => (
                <Tr key={r.id}>
                  <Td className="font-medium text-ink">{r.username}</Td>
                  <Td className="text-muted">{r.email}</Td>
                  <Td className="tabular-nums">{r.customers}</Td>
                  <Td>
                    <Badge tone={r.suspended ? "bad" : "ok"}>{r.suspended ? "Suspended" : "Active"}</Badge>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Create reseller"
        footer={
          <>
            <Button onClick={() => setOpen(false)}>Cancel</Button>
            <Button variant="primary" loading={create.isPending} disabled={!f.username || !f.email || !f.password} onClick={() => create.mutate()}>
              Create reseller
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="Username">{(id) => <Input id={id} value={f.username} onChange={(e) => setF({ ...f, username: e.target.value.toLowerCase() })} autoFocus />}</Field>
          <Field label="Email">{(id) => <Input id={id} type="email" value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} />}</Field>
          <Field label="Password" hint="At least 12 characters.">
            {(id) => <Input id={id} type="password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} autoComplete="new-password" />}
          </Field>
        </div>
      </Modal>
    </>
  );
}
