import { Ban, CheckCircle2, Plus, Shield, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { formatDate, timeAgo } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { LoginFailure, LoginRule } from "@/lib/types";
import { useAuth } from "@/state/auth";
import { Button, IconButton } from "@/components/ui/Button";
import { Field, Input, Select, Textarea } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, Mono, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";

// ---- Host Access Control (brute-force protection) ----------------------------------------

export function HostAccess() {
  const rules = useGet<LoginRule[]>("/server/login-rules");
  const fails = useGet<LoginFailure[]>("/server/login-failures");
  const [f, setF] = useState({ cidr: "", kind: "deny" as "allow" | "deny", note: "" });
  const add = useMutate((v: typeof f) => api.post("/server/login-rules", v), {
    invalidate: ["/server/login-rules", "/server/login-failures"],
    success: (_r, v) => (v.kind === "deny" ? "Address blocked" : "Address trusted"),
    onSuccess: () => setF({ cidr: "", kind: "deny", note: "" }),
  });
  const remove = useMutate((id: string) => api.del(`/server/login-rules/${id}`), { invalidate: ["/server/login-rules"], success: "Rule removed" });
  const ruled = new Set(rules.data?.map((r) => r.cidr));

  return (
    <>
      <PageHeader
        title="Host Access Control"
        description="Brute-force protection for panel sign-in. Repeated failures are rate limited automatically; here you can block addresses permanently or trust your office so it is never limited."
      />
      <div className="grid gap-4 xl:grid-cols-2">
        <Card title="Rules" flush>
          <form
            className="flex flex-wrap items-end gap-2 border-b border-line p-4"
            onSubmit={(e) => {
              e.preventDefault();
              add.mutate(f);
            }}
          >
            <Field label="IP or CIDR" className="min-w-44 flex-1">
              {(id) => <Input id={id} className="font-mono" value={f.cidr} onChange={(e) => setF({ ...f, cidr: e.target.value })} placeholder="198.51.100.0/24" />}
            </Field>
            <Field label="Action" className="w-32">
              {(id) => (
                <Select id={id} value={f.kind} onChange={(e) => setF({ ...f, kind: e.target.value as "allow" | "deny" })}>
                  <option value="deny">Block</option>
                  <option value="allow">Trust</option>
                </Select>
              )}
            </Field>
            <Field label="Note" className="min-w-32 flex-1">
              {(id) => <Input id={id} value={f.note} onChange={(e) => setF({ ...f, note: e.target.value })} />}
            </Field>
            <Button type="submit" icon={Plus} loading={add.isPending} disabled={!f.cidr.trim()}>
              Add
            </Button>
          </form>
          {rules.isLoading ? (
            <Loading />
          ) : !rules.data?.length ? (
            <EmptyState icon={Shield} title="No rules" description="Every address may try to sign in, subject to rate limiting." />
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Address</Th>
                  <Th>Action</Th>
                  <Th>Note</Th>
                  <Th className="w-10">
                    <span className="sr-only">Actions</span>
                  </Th>
                </tr>
              </thead>
              <tbody>
                {rules.data.map((r) => (
                  <Tr key={r.id}>
                    <Td>
                      <Mono className="text-ink">{r.cidr}</Mono>
                    </Td>
                    <Td>
                      <Badge tone={r.kind === "deny" ? "bad" : "ok"}>{r.kind === "deny" ? "Blocked" : "Trusted"}</Badge>
                    </Td>
                    <Td className="text-muted">
                      {r.note || "—"}
                      <div className="text-[11.5px] text-subtle">{formatDate(r.created_at)}</div>
                    </Td>
                    <Td>
                      <IconButton icon={Trash2} label={`Remove rule ${r.cidr}`} onClick={() => remove.mutate(r.id)} />
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>
        <Card title="Failed sign-ins, last 24 hours" flush>
          {fails.isLoading ? (
            <Loading />
          ) : !fails.data?.length ? (
            <EmptyState icon={CheckCircle2} title="No failed sign-ins" />
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Address</Th>
                  <Th className="text-right">Failures</Th>
                  <Th>Last</Th>
                  <Th className="w-20">
                    <span className="sr-only">Actions</span>
                  </Th>
                </tr>
              </thead>
              <tbody>
                {fails.data.map((x) => (
                  <Tr key={x.ip}>
                    <Td>
                      <Mono className="text-ink">{x.ip || "unknown"}</Mono>
                      {x.username && <div className="text-[11.5px] text-subtle">as {x.username}</div>}
                    </Td>
                    <Td className="text-right tabular-nums">
                      {x.failures}
                      {x.blocked && (
                        <span className="ml-2">
                          <Badge tone="warn">rate limited</Badge>
                        </span>
                      )}
                    </Td>
                    <Td className="text-muted">{timeAgo(x.last)}</Td>
                    <Td>
                      {x.ip && !ruled.has(x.ip) && (
                        <Button size="sm" icon={Ban} onClick={() => add.mutate({ cidr: x.ip, kind: "deny", note: `${x.failures} failures` })}>
                          Block
                        </Button>
                      )}
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>
      </div>
    </>
  );
}

// ---- Security policy ---------------------------------------------------------------------

export function SecurityPolicy() {
  const { user } = useAuth();
  const q = useGet<{ require_2fa: string; whm_allowed_ips: string }>("/server/security");
  const [f, setF] = useState({ require_2fa: "off", whm_allowed_ips: "" });
  useEffect(() => {
    if (q.data) setF({ require_2fa: q.data.require_2fa, whm_allowed_ips: q.data.whm_allowed_ips.split(", ").filter(Boolean).join("\n") });
  }, [q.data]);
  const save = useMutate(() => api.put("/server/security", f), { invalidate: ["/server/security"], success: "Security policy saved" });
  if (q.isLoading) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  return (
    <>
      <PageHeader title="Security Policy" description="Server-wide rules for signing in to WHM and cPanel." />
      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="Two-factor authentication">
          <Field label="Require two-factor authentication for" hint="Users without 2FA can still sign in, but can only open their profile to turn it on.">
            {(id) => (
              <Select id={id} value={f.require_2fa} onChange={(e) => setF({ ...f, require_2fa: e.target.value })}>
                <option value="off">Nobody (optional)</option>
                <option value="admins">Administrators and resellers</option>
                <option value="all">Everyone, including cPanel users</option>
              </Select>
            )}
          </Field>
          {!user?.totp_enabled && f.require_2fa !== "off" && (
            <p className="mt-2 text-[12.5px] text-warn">Turn on 2FA for your own login under My Account first.</p>
          )}
        </Card>
        <Card title="WHM access by IP address">
          <Field label="Allowed addresses" hint="One IP or CIDR per line. Empty allows every address. Your current address must be included.">
            {(id) => <Textarea id={id} value={f.whm_allowed_ips} onChange={(e) => setF({ ...f, whm_allowed_ips: e.target.value })} placeholder={"203.0.113.10\n198.51.100.0/24"} />}
          </Field>
          <p className="mt-2 text-[12.5px] text-muted">Only affects WHM. Customers can always reach cPanel.</p>
        </Card>
      </div>
      <div className="mt-4 flex justify-end">
        <Button variant="primary" loading={save.isPending} onClick={() => save.mutate()}>
          Save policy
        </Button>
      </div>
    </>
  );
}
