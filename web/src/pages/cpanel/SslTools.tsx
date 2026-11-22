import { Download, FileKey2 } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import type { Domain } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button } from "@/components/ui/Button";
import { Field, Input, Select, Switch, Textarea } from "@/components/ui/Form";
import { Card, ErrorNote } from "@/components/ui/Layout";
import { Modal } from "@/components/ui/Modal";

export function AutoSslExclusions({ domains }: { domains: Domain[] }) {
  const aid = useAccountId();
  const key = `/accounts/${aid}/ssl/autossl`;
  const q = useGet<Record<string, boolean>>(key);
  const set = useMutate((v: { id: string; enabled: boolean }) => api.put(`${key}/${v.id}`, { enabled: v.enabled }), { invalidate: [key], success: "AutoSSL updated" });
  return (
    <Card title="AutoSSL" description="Switch a domain off if you install its certificate yourself or its DNS is elsewhere." className="mt-4" flush>
      {q.error ? (
        <div className="p-4">
          <ErrorNote error={q.error} />
        </div>
      ) : (
        <ul className="divide-y divide-[var(--line)]">
          {domains.map((d) => (
            <li key={d.id} className="flex items-center justify-between gap-3 px-4 py-2.5 text-[13px]">
              <span className="min-w-0 break-all text-ink">{d.name}</span>
              <Switch label={`AutoSSL for ${d.name}`} checked={q.data?.[d.id] ?? true} disabled={!q.data} onChange={(enabled) => set.mutate({ id: d.id, enabled })} />
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

function save(name: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: "application/x-pem-file" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export function CsrGenerator({ domains }: { domains: Domain[] }) {
  const aid = useAccountId();
  const [open, setOpen] = useState(false);
  const [f, setF] = useState({ domain_id: "", key_type: "ecdsa-p256", organization: "", country: "" });
  const [made, setMade] = useState<{ key: string; csr: string } | null>(null);
  const domain = domains.find((d) => d.id === f.domain_id) ?? domains[0];
  const make = useMutate(() => api.post<{ key: string; csr: string }>(`/accounts/${aid}/ssl/csr`, { ...f, domain_id: domain?.id }), { onSuccess: setMade });
  const close = () => {
    setOpen(false);
    setMade(null);
  };
  return (
    <>
      <Button icon={FileKey2} disabled={!domain} onClick={() => setOpen(true)}>
        Generate key and CSR
      </Button>
      <Modal
        open={open}
        onClose={close}
        title="Private key and signing request"
        description={made ? "The key is not stored on the server. Save it before closing." : "For buying a certificate from a certificate authority."}
        width="max-w-xl"
        footer={
          made && domain ? (
            <>
              <Button icon={Download} onClick={() => save(`${domain.name}.key`, made.key)}>
                Save key
              </Button>
              <Button icon={Download} onClick={() => save(`${domain.name}.csr`, made.csr)}>
                Save CSR
              </Button>
              <Button variant="primary" onClick={close}>
                Done
              </Button>
            </>
          ) : (
            <>
              <Button onClick={close}>Cancel</Button>
              <Button variant="primary" loading={make.isPending} onClick={() => make.mutate()}>
                Generate
              </Button>
            </>
          )
        }
      >
        {made ? (
          <div className="space-y-4">
            <Field label="Signing request">{(id) => <Textarea id={id} readOnly rows={6} value={made.csr} />}</Field>
            <Field label="Private key">{(id) => <Textarea id={id} readOnly rows={6} value={made.key} />}</Field>
          </div>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Domain" className="sm:col-span-2">
              {(id) => (
                <Select id={id} value={domain?.id} onChange={(e) => setF({ ...f, domain_id: e.target.value })}>
                  {domains.map((d) => (
                    <option key={d.id} value={d.id}>
                      {d.name}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            <Field label="Key type" className="sm:col-span-2">
              {(id) => (
                <Select id={id} value={f.key_type} onChange={(e) => setF({ ...f, key_type: e.target.value })}>
                  <option value="ecdsa-p256">ECDSA P-256 (recommended)</option>
                  <option value="ecdsa-p384">ECDSA P-384</option>
                  <option value="rsa-2048">RSA 2048-bit</option>
                  <option value="rsa-4096">RSA 4096-bit</option>
                </Select>
              )}
            </Field>
            <Field label="Organization (optional)">{(id) => <Input id={id} value={f.organization} onChange={(e) => setF({ ...f, organization: e.target.value })} />}</Field>
            <Field label="Country code (optional)">{(id) => <Input id={id} maxLength={2} value={f.country} onChange={(e) => setF({ ...f, country: e.target.value })} />}</Field>
          </div>
        )}
      </Modal>
    </>
  );
}
