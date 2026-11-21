import { Plus, Shield, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import type { FirewallRule } from "@/lib/types";
import { Button, IconButton } from "@/components/ui/Button";
import { Field, Input, Select } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, Mono, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { Modal } from "@/components/ui/Modal";

const blank = { action: "allow", protocol: "tcp", port: "", source: "", comment: "" };

export default function Firewall() {
  const q = useGet<FirewallRule[]>("/server/firewall");
  const [open, setOpen] = useState(false);
  const [f, setF] = useState(blank);

  const add = useMutate(() => api.post("/server/firewall", f), {
    invalidate: ["/server/firewall"],
    success: "Rule applied",
    onSuccess: () => {
      setOpen(false);
      setF(blank);
    },
  });
  const remove = useMutate((id: string) => api.del(`/server/firewall/${id}`), { invalidate: ["/server/firewall"], success: "Rule removed" });

  return (
    <>
      <PageHeader
        title="Firewall"
        description="nftables rules, default-deny inbound. SSH, web, mail, DNS and the panel port always stay open so you cannot lock yourself out."
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setOpen(true)}>
            Add rule
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
          <EmptyState icon={Shield} title="No custom rules" description="Only the built-in service ports are open. Add a rule to open another port or block an address." />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Action</Th>
                <Th>Protocol</Th>
                <Th>Port</Th>
                <Th>Source</Th>
                <Th>Comment</Th>
                <Th className="w-10">
                  <span className="sr-only">Actions</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((r) => (
                <Tr key={r.id}>
                  <Td>
                    <Badge tone={r.action === "allow" ? "ok" : "bad"}>{r.action === "allow" ? "Allow" : "Deny"}</Badge>
                  </Td>
                  <Td className="uppercase">{r.protocol}</Td>
                  <Td>
                    <Mono>{r.port || "any"}</Mono>
                  </Td>
                  <Td>
                    <Mono>{r.source || "anywhere"}</Mono>
                  </Td>
                  <Td className="text-muted">{r.comment}</Td>
                  <Td>
                    <IconButton icon={Trash2} label="Delete rule" onClick={() => remove.mutate(r.id)} />
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
        title="Add firewall rule"
        description="Rules are validated with nft before they replace the live ruleset."
        footer={
          <>
            <Button onClick={() => setOpen(false)}>Cancel</Button>
            <Button variant="primary" loading={add.isPending} onClick={() => add.mutate()}>
              Apply rule
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-3">
            <Field label="Action">
              {(id) => (
                <Select id={id} value={f.action} onChange={(e) => setF({ ...f, action: e.target.value })}>
                  <option value="allow">Allow</option>
                  <option value="deny">Deny</option>
                </Select>
              )}
            </Field>
            <Field label="Protocol">
              {(id) => (
                <Select id={id} value={f.protocol} onChange={(e) => setF({ ...f, protocol: e.target.value })}>
                  <option value="tcp">TCP</option>
                  <option value="udp">UDP</option>
                  <option value="any">Any</option>
                </Select>
              )}
            </Field>
          </div>
          <Field label="Port or range" hint="For example 3306 or 8000:8100. Leave blank for all ports.">
            {(id) => <Input id={id} value={f.port} onChange={(e) => setF({ ...f, port: e.target.value })} className="font-mono" />}
          </Field>
          <Field label="Source address" hint="An IP or CIDR such as 203.0.113.0/24. Leave blank for anywhere.">
            {(id) => <Input id={id} value={f.source} onChange={(e) => setF({ ...f, source: e.target.value })} className="font-mono" />}
          </Field>
          <Field label="Comment">{(id) => <Input id={id} value={f.comment} onChange={(e) => setF({ ...f, comment: e.target.value })} />}</Field>
        </div>
      </Modal>
    </>
  );
}
