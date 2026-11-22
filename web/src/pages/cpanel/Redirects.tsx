import { ArrowRight, Plus, Repeat, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import type { Domain, RedirectRule } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button, IconButton } from "@/components/ui/Button";
import { DomainPicker } from "@/components/ui/DomainPicker";
import { Checkbox, Field, Input, Select } from "@/components/ui/Form";
import { Badge, Card, EmptyState, Loading, Mono, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { Modal } from "@/components/ui/Modal";

export default function Redirects() {
  const aid = useAccountId();
  const domains = useGet<Domain[]>(`/accounts/${aid}/domains`);
  const [domainId, setDomainId] = useState("");
  const key = `/accounts/${aid}/domains/${domainId}/redirects`;
  const q = useGet<RedirectRule[]>(key, !!domainId);
  const [open, setOpen] = useState(false);
  const [f, setF] = useState({ source: "/", target: "https://", code: "301", wildcard: false });
  const domain = domains.data?.find((d) => d.id === domainId);

  const add = useMutate(() => api.post(key, { ...f, code: Number(f.code) }), {
    invalidate: [key],
    success: "Redirect added",
    onSuccess: () => {
      setOpen(false);
      setF({ source: "/", target: "https://", code: "301", wildcard: false });
    },
  });
  const del = useMutate((id: string) => api.del(`${key}/${id}`), { invalidate: [key], success: "Redirect removed" });

  return (
    <>
      <PageHeader
        title="Redirects"
        description="Send visitors from one path to another URL. Permanent (301/308) redirects also move search rankings."
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setOpen(true)} disabled={!domainId}>
            Add redirect
          </Button>
        }
      />
      <div className="mb-4">
        <DomainPicker domains={domains.data} value={domainId} onChange={setDomainId} />
      </div>
      <Card flush>
        {q.isLoading || domains.isLoading ? (
          <Loading />
        ) : !q.data?.length ? (
          <EmptyState icon={Repeat} title="No redirects" description={`Nothing is redirected on ${domain?.name ?? "this domain"}.`} />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>From</Th>
                <Th className="w-8" />
                <Th>To</Th>
                <Th>Type</Th>
                <Th className="w-10">
                  <span className="sr-only">Actions</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((r) => (
                <Tr key={r.id}>
                  <Td>
                    <Mono className="text-ink">
                      {domain?.name}
                      {r.source}
                      {r.wildcard && (r.source.endsWith("/") ? "*" : "/*")}
                    </Mono>
                  </Td>
                  <Td>
                    <ArrowRight className="size-3.5 text-subtle" aria-hidden />
                  </Td>
                  <Td className="max-w-md">
                    <Mono className="block truncate text-muted">{r.target}</Mono>
                  </Td>
                  <Td>
                    <Badge tone={r.code === 301 || r.code === 308 ? "accent" : "neutral"}>{r.code === 301 || r.code === 308 ? `Permanent ${r.code}` : `Temporary ${r.code}`}</Badge>
                  </Td>
                  <Td>
                    <IconButton icon={Trash2} label={`Remove redirect ${r.source}`} onClick={() => del.mutate(r.id)} />
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
        title={`Add redirect on ${domain?.name}`}
        footer={
          <>
            <Button onClick={() => setOpen(false)}>Cancel</Button>
            <Button variant="primary" loading={add.isPending} onClick={() => add.mutate()}>
              Add redirect
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="From path" hint={f.wildcard ? "Everything below this path too; / means the whole site" : "An exact path, for example /old-page"}>
            {(id) => <Input id={id} className="font-mono" value={f.source} onChange={(e) => setF({ ...f, source: e.target.value })} autoFocus />}
          </Field>
          <label className="flex items-center gap-2 text-[13px] text-ink">
            <Checkbox checked={f.wildcard} onChange={(e) => setF({ ...f, wildcard: e.target.checked })} />
            Wildcard: keep the rest of the path and the query
          </label>
          <Field label="To URL">{(id) => <Input id={id} className="font-mono" value={f.target} onChange={(e) => setF({ ...f, target: e.target.value })} />}</Field>
          <Field label="Type">
            {(id) => (
              <Select id={id} value={f.code} onChange={(e) => setF({ ...f, code: e.target.value })}>
                <option value="301">301 Moved permanently</option>
                <option value="308">308 Permanent (keeps method)</option>
                <option value="302">302 Found (temporary)</option>
                <option value="307">307 Temporary (keeps method)</option>
              </Select>
            )}
          </Field>
        </div>
      </Modal>
    </>
  );
}
