import { KeyRound, Plus, Smartphone, Trash2 } from "lucide-react";
import QRCode from "qrcode";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { formatDate, timeAgo } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { ApiToken, SessionInfo } from "@/lib/types";
import { useAuth } from "@/state/auth";
import { Button, IconButton } from "@/components/ui/Button";
import { CopyField } from "@/components/ui/CopyField";
import { Field, Input } from "@/components/ui/Form";
import { Badge, Card, ErrorNote, Loading, Mono, PageHeader, Table, Tabs, Td, Th, Tr } from "@/components/ui/Layout";
import { Modal } from "@/components/ui/Modal";

type Tab = "security" | "sessions" | "tokens";

export default function Profile() {
  const [tab, setTab] = useState<Tab>("security");
  const { user } = useAuth();
  return (
    <>
      <PageHeader title="Profile & Security" description={`Signed in as ${user?.username} (${user?.email}).`} />
      <Tabs
        tabs={[
          { id: "security", label: "Password & 2FA" },
          { id: "sessions", label: "Sessions" },
          { id: "tokens", label: "API tokens" },
        ]}
        value={tab}
        onChange={setTab}
      />
      {tab === "security" && (
        <div className="max-w-2xl space-y-5">
          <PasswordCard />
          <TwoFactorCard />
        </div>
      )}
      {tab === "sessions" && <Sessions />}
      {tab === "tokens" && <Tokens />}
    </>
  );
}

function PasswordCard() {
  const [f, setF] = useState({ current: "", next: "", confirm: "" });
  const mismatch = f.confirm !== "" && f.next !== f.confirm;
  const save = useMutate(() => api.post("/me/password", { current: f.current, new: f.next }), {
    success: "Password changed. Other sessions were signed out.",
    onSuccess: () => setF({ current: "", next: "", confirm: "" }),
  });
  return (
    <Card title="Change password">
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (!mismatch) save.mutate();
        }}
      >
        <Field label="Current password">
          {(id) => <Input id={id} type="password" value={f.current} onChange={(e) => setF({ ...f, current: e.target.value })} autoComplete="current-password" required />}
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="New password" hint="At least 12 characters, mixing letters with digits or symbols.">
            {(id) => <Input id={id} type="password" value={f.next} onChange={(e) => setF({ ...f, next: e.target.value })} autoComplete="new-password" required />}
          </Field>
          <Field label="Confirm new password" error={mismatch ? "Passwords do not match." : undefined}>
            {(id) => <Input id={id} type="password" value={f.confirm} onChange={(e) => setF({ ...f, confirm: e.target.value })} autoComplete="new-password" required />}
          </Field>
        </div>
        <Button type="submit" variant="primary" loading={save.isPending} disabled={!f.current || !f.next || mismatch}>
          Update password
        </Button>
      </form>
    </Card>
  );
}

function TwoFactorCard() {
  const { user, refresh } = useAuth();
  const [setup, setSetup] = useState<{ secret: string; uri: string } | null>(null);
  const [qr, setQr] = useState("");
  const [code, setCode] = useState("");
  const [recovery, setRecovery] = useState<string[] | null>(null);
  const [disabling, setDisabling] = useState(false);
  const [password, setPassword] = useState("");

  useEffect(() => {
    if (setup) void QRCode.toDataURL(setup.uri, { margin: 1, width: 180 }).then(setQr);
  }, [setup]);

  const start = useMutate(() => api.post<{ secret: string; uri: string }>("/me/2fa/setup"), { onSuccess: setSetup });
  const enable = useMutate(() => api.post<{ recovery_codes: string[] }>("/me/2fa/enable", { code }), {
    onSuccess: async (r) => {
      setRecovery(r.recovery_codes);
      setSetup(null);
      setCode("");
      await refresh();
    },
  });
  const disable = useMutate(() => api.post("/me/2fa/disable", { password }), {
    success: "Two-factor authentication disabled",
    onSuccess: async () => {
      setDisabling(false);
      setPassword("");
      await refresh();
    },
  });

  return (
    <>
      <Card
        title="Two-factor authentication"
        description="Require a code from an authenticator app in addition to your password."
        actions={user?.totp_enabled ? <Badge tone="ok">Enabled</Badge> : <Badge>Off</Badge>}
      >
        {user?.totp_enabled ? (
          <Button variant="danger" onClick={() => setDisabling(true)}>
            Disable two-factor
          </Button>
        ) : setup ? (
          <div className="flex flex-col gap-5 sm:flex-row">
            {qr && <img src={qr} alt="QR code for your authenticator app" className="size-[180px] rounded-md border border-line bg-white p-1" />}
            <div className="min-w-0 flex-1 space-y-3">
              <p className="text-[13px] text-muted">Scan the code with an authenticator app, or enter this key manually:</p>
              <CopyField value={setup.secret} />
              <Field label="Enter the 6-digit code to confirm">
                {(id) => <Input id={id} value={code} onChange={(e) => setCode(e.target.value)} inputMode="numeric" autoComplete="one-time-code" className="max-w-[10rem]" />}
              </Field>
              <div className="flex gap-2">
                <Button variant="primary" loading={enable.isPending} disabled={code.length < 6} onClick={() => enable.mutate()}>
                  Enable
                </Button>
                <Button onClick={() => setSetup(null)}>Cancel</Button>
              </div>
            </div>
          </div>
        ) : (
          <Button icon={Smartphone} loading={start.isPending} onClick={() => start.mutate()}>
            Set up authenticator app
          </Button>
        )}
      </Card>

      <Modal
        open={!!recovery}
        onClose={() => setRecovery(null)}
        title="Save your recovery codes"
        description="Each code works once if you lose access to your authenticator. They will not be shown again."
        footer={
          <Button variant="primary" onClick={() => setRecovery(null)}>
            I have saved them
          </Button>
        }
      >
        <div className="grid grid-cols-2 gap-2 rounded-md border border-line bg-surface2 p-3">
          {recovery?.map((c) => (
            <Mono key={c} className="text-ink">
              {c}
            </Mono>
          ))}
        </div>
      </Modal>

      <Modal
        open={disabling}
        onClose={() => setDisabling(false)}
        title="Disable two-factor?"
        footer={
          <>
            <Button onClick={() => setDisabling(false)}>Cancel</Button>
            <Button variant="danger" loading={disable.isPending} disabled={!password} onClick={() => disable.mutate()}>
              Disable
            </Button>
          </>
        }
      >
        <Field label="Confirm your password">
          {(id) => <Input id={id} type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" autoFocus />}
        </Field>
      </Modal>
    </>
  );
}

function Sessions() {
  const q = useGet<SessionInfo[]>("/me/sessions");
  const revoke = useMutate((id: string) => api.del(`/me/sessions/${id}`), { invalidate: ["/me/sessions"], success: "Session signed out" });
  return (
    <Card flush>
      {q.isLoading ? (
        <Loading />
      ) : q.error ? (
        <div className="p-4">
          <ErrorNote error={q.error} />
        </div>
      ) : (
        <Table>
          <thead>
            <tr>
              <Th>Device</Th>
              <Th>IP address</Th>
              <Th>Last active</Th>
              <Th className="w-10">
                <span className="sr-only">Actions</span>
              </Th>
            </tr>
          </thead>
          <tbody>
            {q.data?.map((s) => (
              <Tr key={s.id}>
                <Td className="max-w-xs">
                  <span className="block truncate text-ink" title={s.user_agent}>
                    {s.user_agent || "Unknown device"}
                  </span>
                  {s.current && <Badge tone="accent">This session</Badge>}
                </Td>
                <Td>
                  <Mono className="text-muted">{s.ip}</Mono>
                </Td>
                <Td className="text-muted">{timeAgo(s.last_seen)}</Td>
                <Td>{!s.current && <IconButton icon={Trash2} label="Sign out this session" onClick={() => revoke.mutate(s.id)} />}</Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      )}
    </Card>
  );
}

const SCOPES: [string, string][] = [
  ["billing", "Billing & provisioning (WHMCS, Paymenter)"],
  ["accounts", "Accounts"],
  ["packages", "Packages"],
  ["hosting", "Hosting features"],
  ["files", "File manager"],
];

function Tokens() {
  const q = useGet<ApiToken[]>("/me/tokens");
  const [open, setOpen] = useState(false);
  const [f, setF] = useState({ name: "", scopes: ["billing"], allowed_ips: "", days: "365" });
  const [secret, setSecret] = useState<string | null>(null);

  const create = useMutate(
    () => api.post<{ token: string }>("/me/tokens", { name: f.name, scopes: f.scopes, allowed_ips: f.allowed_ips, expires_in_days: Number(f.days) || 0 }),
    {
      invalidate: ["/me/tokens"],
      onSuccess: (r) => {
        setOpen(false);
        setSecret(r.token);
        setF({ name: "", scopes: ["billing"], allowed_ips: "", days: "365" });
      },
    },
  );
  const remove = useMutate((id: string) => api.del(`/me/tokens/${id}`), { invalidate: ["/me/tokens"], success: "Token revoked" });

  return (
    <>
      <div className="mb-4 flex justify-end">
        <Button variant="primary" icon={Plus} onClick={() => setOpen(true)}>
          Create token
        </Button>
      </div>
      <Card flush>
        {q.isLoading ? (
          <Loading />
        ) : !q.data?.length ? (
          <div className="flex flex-col items-center px-6 py-12 text-center">
            <KeyRound className="mb-3 size-6 text-subtle" aria-hidden />
            <h3 className="text-[13.5px] font-semibold">No API tokens</h3>
            <p className="mt-1 max-w-sm text-[13px] text-muted">Create a token to connect WHMCS, Paymenter or your own automation.</p>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Name</Th>
                <Th>Scopes</Th>
                <Th>Last used</Th>
                <Th>Expires</Th>
                <Th className="w-10">
                  <span className="sr-only">Actions</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((t) => (
                <Tr key={t.id}>
                  <Td className="font-medium text-ink">{t.name}</Td>
                  <Td>
                    <div className="flex flex-wrap gap-1">
                      {t.scopes.split(" ").map((s) => (
                        <Badge key={s}>{s}</Badge>
                      ))}
                    </div>
                  </Td>
                  <Td className="text-muted">{t.last_used ? timeAgo(t.last_used) : "never"}</Td>
                  <Td className="text-muted">{t.expires_at ? formatDate(t.expires_at) : "never"}</Td>
                  <Td>
                    <IconButton icon={Trash2} label={`Revoke ${t.name}`} onClick={() => remove.mutate(t.id)} />
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
        title="Create API token"
        footer={
          <>
            <Button onClick={() => setOpen(false)}>Cancel</Button>
            <Button variant="primary" loading={create.isPending} disabled={!f.name || !f.scopes.length} onClick={() => create.mutate()}>
              Create token
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="Name">{(id) => <Input id={id} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="WHMCS production" autoFocus />}</Field>
          <fieldset>
            <legend className="mb-1.5 text-[12.5px] font-medium">Scopes</legend>
            <div className="space-y-1.5">
              {SCOPES.map(([id, label]) => (
                <label key={id} className="flex items-center gap-2 text-[13px]">
                  <input
                    type="checkbox"
                    className="accent-[var(--accent)]"
                    checked={f.scopes.includes(id)}
                    onChange={(e) => setF({ ...f, scopes: e.target.checked ? [...f.scopes, id] : f.scopes.filter((s) => s !== id) })}
                  />
                  {label}
                </label>
              ))}
            </div>
          </fieldset>
          <Field label="Allowed IP addresses" hint="Optional. Comma separated IPs or CIDRs. Strongly recommended for billing tokens.">
            {(id) => <Input id={id} value={f.allowed_ips} onChange={(e) => setF({ ...f, allowed_ips: e.target.value })} className="font-mono" placeholder="203.0.113.7" />}
          </Field>
          <Field label="Expires after (days)" hint="0 = never expires.">
            {(id) => <Input id={id} type="number" min={0} value={f.days} onChange={(e) => setF({ ...f, days: e.target.value })} className="max-w-[8rem]" />}
          </Field>
        </div>
      </Modal>

      <Modal open={!!secret} onClose={() => setSecret(null)} title="Copy your token now" description="For security it is shown only once." footer={<Button variant="primary" onClick={() => setSecret(null)}>Done</Button>}>
        <CopyField value={secret ?? ""} />
      </Modal>
    </>
  );
}
