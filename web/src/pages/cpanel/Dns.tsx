import { Network, Plus, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import type { DnsRecord, Domain } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button, IconButton } from "@/components/ui/Button";
import { Field, Input, Select } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, Mono, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

const TYPES = ["A", "AAAA", "CNAME", "MX", "TXT", "NS", "SRV", "CAA"];

export default function Dns() {
  const aid = useAccountId();
  const domains = useGet<Domain[]>(`/accounts/${aid}/domains`);
  const zones = domains.data?.filter((d) => d.kind === "primary" || d.kind === "addon") ?? [];
  const [zone, setZone] = useState("");
  useEffect(() => {
    if (!zone && zones[0]) setZone(zones[0].name);
  }, [zone, zones]);

  const key = `/accounts/${aid}/dns/${zone}`;
  const recs = useGet<DnsRecord[]>(key, !!zone);
  const [adding, setAdding] = useState(false);
  const [f, setF] = useState({ name: "", type: "A", ttl: "3600", content: "", priority: "10" });
  const [del, setDel] = useState<DnsRecord | null>(null);

  const reset = () => {
    setAdding(false);
    setF({ name: "", type: "A", ttl: "3600", content: "", priority: "10" });
  };
  const add = useMutate(
    () =>
      api.post(key, {
        name: f.name || "@",
        type: f.type,
        ttl: Number(f.ttl),
        content: f.content,
        ...(f.type === "MX" ? { priority: Number(f.priority) } : {}),
      }),
    { invalidate: [key], success: "Record saved", onSuccess: reset },
  );
  const remove = useMutate((r: DnsRecord) => api.del(key, { name: r.name, type: r.type }), {
    invalidate: [key],
    success: "Record deleted",
    onSuccess: () => setDel(null),
  });

  const rel = (name: string) => (name === zone ? "@" : name.replace(new RegExp(`\\.${zone.replace(/\./g, "\\.")}$`), ""));
  const readOnly = (r: DnsRecord) => r.type === "SOA";

  return (
    <>
      <PageHeader
        title="DNS Zone Editor"
        description="Edit the DNS records served by this server's authoritative nameservers."
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setAdding(true)} disabled={!zone}>
            Add record
          </Button>
        }
      />
      {zones.length > 1 && (
        <div className="mb-4 max-w-xs">
          <Field label="Zone">
            {(id) => (
              <Select id={id} value={zone} onChange={(e) => setZone(e.target.value)}>
                {zones.map((z) => (
                  <option key={z.id}>{z.name}</option>
                ))}
              </Select>
            )}
          </Field>
        </div>
      )}
      <Card flush>
        {domains.isLoading || recs.isLoading ? (
          <Loading />
        ) : recs.error ? (
          <div className="p-4">
            <ErrorNote error={recs.error} />
          </div>
        ) : !recs.data?.length ? (
          <EmptyState icon={Network} title="No records" description="Add an A record to point your domain at a server." />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Name</Th>
                <Th>Type</Th>
                <Th>TTL</Th>
                <Th>Value</Th>
                <Th className="w-10">
                  <span className="sr-only">Actions</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {recs.data.map((r, i) => (
                <Tr key={`${r.name}-${r.type}-${i}`}>
                  <Td>
                    <Mono className="text-ink">{rel(r.name)}</Mono>
                  </Td>
                  <Td>
                    <Badge tone={r.type === "A" || r.type === "AAAA" ? "accent" : "neutral"}>{r.type}</Badge>
                  </Td>
                  <Td className="tabular-nums text-muted">{r.ttl}</Td>
                  <Td className="max-w-md">
                    <Mono className="block truncate text-muted" >{r.content}</Mono>
                  </Td>
                  <Td>{!readOnly(r) && <IconButton icon={Trash2} label={`Delete ${r.type} record ${rel(r.name)}`} onClick={() => setDel(r)} />}</Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <Modal
        open={adding}
        onClose={reset}
        title="Add DNS record"
        description="Adding a record with the same name and type replaces the existing one."
        footer={
          <>
            <Button onClick={reset}>Cancel</Button>
            <Button variant="primary" loading={add.isPending} disabled={!f.content} onClick={() => add.mutate()}>
              Save record
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="grid grid-cols-[1fr_7rem] gap-3">
            <Field label="Name" hint={`Use @ for ${zone}`}>
              {(id) => <Input id={id} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="@" autoFocus />}
            </Field>
            <Field label="Type">
              {(id) => (
                <Select id={id} value={f.type} onChange={(e) => setF({ ...f, type: e.target.value })}>
                  {TYPES.map((t) => (
                    <option key={t}>{t}</option>
                  ))}
                </Select>
              )}
            </Field>
          </div>
          <Field label="Value" hint={f.type === "TXT" ? "Wrap the value in double quotes." : undefined}>
            {(id) => <Input id={id} value={f.content} onChange={(e) => setF({ ...f, content: e.target.value })} className="font-mono" />}
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="TTL (seconds)">
              {(id) => <Input id={id} type="number" min={60} max={86400} value={f.ttl} onChange={(e) => setF({ ...f, ttl: e.target.value })} />}
            </Field>
            {f.type === "MX" && (
              <Field label="Priority">
                {(id) => <Input id={id} type="number" min={0} value={f.priority} onChange={(e) => setF({ ...f, priority: e.target.value })} />}
              </Field>
            )}
          </div>
        </div>
      </Modal>

      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        onConfirm={() => del && remove.mutate(del)}
        loading={remove.isPending}
        title="Delete DNS record?"
        message={del ? <>All <Mono>{del.type}</Mono> records named <Mono>{rel(del.name)}</Mono> will be removed.</> : null}
      />
    </>
  );
}
