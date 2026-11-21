import { LockKeyhole, ShieldCheck, Upload } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { Cert, Domain } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button } from "@/components/ui/Button";
import { Field, Textarea } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

const DAY = 86400;

export default function Ssl() {
  const aid = useAccountId();
  const certsKey = `/accounts/${aid}/ssl`;
  const domKey = `/accounts/${aid}/domains`;
  const domains = useGet<Domain[]>(domKey);
  const certs = useGet<Cert[]>(certsKey);
  const [custom, setCustom] = useState<Domain | null>(null);
  const [pem, setPem] = useState({ certificate: "", key: "" });
  const [removing, setRemoving] = useState<Domain | null>(null);

  const invalidate = [certsKey, domKey];
  const issue = useMutate((d: Domain) => api.post(`${domKey}/${d.id}/ssl/letsencrypt`), { invalidate, success: (_r, d) => `Certificate issued for ${d.name}` });
  const install = useMutate((d: Domain) => api.post(`${domKey}/${d.id}/ssl/custom`, pem), {
    invalidate,
    success: "Certificate installed",
    onSuccess: () => {
      setCustom(null);
      setPem({ certificate: "", key: "" });
    },
  });
  const remove = useMutate((d: Domain) => api.del(`${domKey}/${d.id}/ssl`), { invalidate, success: "Certificate removed", onSuccess: () => setRemoving(null) });

  const certFor = (id: string) => certs.data?.find((c) => c.domain_id === id);
  const loading = domains.isLoading || certs.isLoading;

  return (
    <>
      <PageHeader
        title="SSL/TLS"
        description="Free, auto-renewing Let's Encrypt certificates, or bring your own."
      />
      <Card flush>
        {loading ? (
          <Loading />
        ) : domains.error || certs.error ? (
          <div className="p-4">
            <ErrorNote error={domains.error ?? certs.error} />
          </div>
        ) : !domains.data?.length ? (
          <EmptyState icon={LockKeyhole} title="No domains" description="Add a domain before installing a certificate." />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Domain</Th>
                <Th>Certificate</Th>
                <Th>Expires</Th>
                <Th className="text-right">Actions</Th>
              </tr>
            </thead>
            <tbody>
              {domains.data.map((d) => {
                const c = certFor(d.id);
                const daysLeft = c ? Math.floor((c.not_after - Date.now() / 1000) / DAY) : 0;
                return (
                  <Tr key={d.id}>
                    <Td className="font-medium text-ink">{d.name}</Td>
                    <Td>
                      {c ? (
                        <div className="flex items-center gap-2">
                          <Badge tone={daysLeft < 0 ? "bad" : daysLeft < 14 ? "warn" : "ok"}>
                            <ShieldCheck className="size-3" aria-hidden />
                            {c.issuer === "letsencrypt" ? "Let's Encrypt" : c.issuer === "custom" ? "Custom" : c.issuer}
                          </Badge>
                          {c.auto_renew && c.issuer === "letsencrypt" && <span className="text-[12px] text-muted">auto-renews</span>}
                        </div>
                      ) : (
                        <Badge tone="warn">Not secured</Badge>
                      )}
                    </Td>
                    <Td className="text-muted">{c ? `${formatDate(c.not_after)} (${daysLeft}d)` : "—"}</Td>
                    <Td>
                      <div className="flex justify-end gap-2">
                        <Button size="sm" variant={c ? "secondary" : "primary"} loading={issue.isPending && issue.variables?.id === d.id} onClick={() => issue.mutate(d)}>
                          {c ? "Reissue" : "Get free certificate"}
                        </Button>
                        <Button size="sm" icon={Upload} onClick={() => setCustom(d)}>
                          Upload
                        </Button>
                        {c && (
                          <Button size="sm" variant="ghost" onClick={() => setRemoving(d)}>
                            Remove
                          </Button>
                        )}
                      </div>
                    </Td>
                  </Tr>
                );
              })}
            </tbody>
          </Table>
        )}
      </Card>
      <p className="mt-3 max-w-2xl text-[12.5px] text-muted">
        Free certificates use the HTTP-01 challenge, so the domain must already point at this server. Both the bare domain and www are covered.
      </p>

      <Modal
        open={!!custom}
        onClose={() => setCustom(null)}
        title={`Install certificate for ${custom?.name}`}
        width="max-w-xl"
        footer={
          <>
            <Button onClick={() => setCustom(null)}>Cancel</Button>
            <Button variant="primary" loading={install.isPending} disabled={!pem.certificate || !pem.key} onClick={() => custom && install.mutate(custom)}>
              Install certificate
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="Certificate (PEM, including any intermediate chain)">
            {(id) => (
              <Textarea id={id} rows={7} value={pem.certificate} onChange={(e) => setPem({ ...pem, certificate: e.target.value })} placeholder="-----BEGIN CERTIFICATE-----" spellCheck={false} />
            )}
          </Field>
          <Field label="Private key (PEM)">
            {(id) => <Textarea id={id} rows={6} value={pem.key} onChange={(e) => setPem({ ...pem, key: e.target.value })} placeholder="-----BEGIN PRIVATE KEY-----" spellCheck={false} />}
          </Field>
        </div>
      </Modal>

      <ConfirmDialog
        open={!!removing}
        onClose={() => setRemoving(null)}
        onConfirm={() => removing && remove.mutate(removing)}
        loading={remove.isPending}
        title={`Remove certificate for ${removing?.name}?`}
        message="The site will be served over HTTP only until a new certificate is installed."
        confirmLabel="Remove"
      />
    </>
  );
}
