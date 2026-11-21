import { Ban, LogIn, MoreHorizontal, Plus, Search, Trash2, Users } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import { formatMB, timeAgo } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { Account, Pkg } from "@/lib/types";
import { Button, IconButton } from "@/components/ui/Button";
import { CopyField } from "@/components/ui/CopyField";
import { Field, Input, Select } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

const blank = { username: "", email: "", domain: "", package_id: "", password: "" };

export default function Accounts() {
  const [search, setSearch] = useState("");
  const [debounced, setDebounced] = useState("");
  useEffect(() => {
    const t = setTimeout(() => setDebounced(search.trim()), 250);
    return () => clearTimeout(t);
  }, [search]);

  const key = `/accounts${debounced ? `?q=${encodeURIComponent(debounced)}` : ""}`;
  const q = useGet<Account[]>(key);
  const pkgs = useGet<Pkg[]>("/packages");

  const [creating, setCreating] = useState(false);
  const [f, setF] = useState(blank);
  const [created, setCreated] = useState<{ username: string; password: string } | null>(null);
  const [suspend, setSuspend] = useState<Account | null>(null);
  const [reason, setReason] = useState("");
  const [del, setDel] = useState<Account | null>(null);
  const [menu, setMenu] = useState<string | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!menu) return;
    const on = (e: MouseEvent) => !menuRef.current?.contains(e.target as Node) && setMenu(null);
    document.addEventListener("mousedown", on);
    return () => document.removeEventListener("mousedown", on);
  }, [menu]);

  const create = useMutate(
    () => api.post<{ account: Account; password?: string }>("/accounts", { ...f, password: f.password || undefined }),
    {
      invalidate: ["/accounts"],
      success: "Account created",
      onSuccess: (r) => {
        setCreating(false);
        if (r.password) setCreated({ username: r.account.username, password: r.password });
        setF(blank);
      },
    },
  );
  const doSuspend = useMutate((a: Account) => api.post(`/accounts/${a.id}/suspend`, { reason }), {
    invalidate: [key],
    success: "Account suspended",
    onSuccess: () => {
      setSuspend(null);
      setReason("");
    },
  });
  const unsuspend = useMutate((a: Account) => api.post(`/accounts/${a.id}/unsuspend`), { invalidate: [key], success: "Account reactivated" });
  const terminate = useMutate((a: Account) => api.del(`/accounts/${a.id}`), { invalidate: [key], success: "Account terminated", onSuccess: () => setDel(null) });
  const sso = useMutate((a: Account) => api.post<{ path: string }>(`/accounts/${a.id}/sso`), {
    onSuccess: (r) => window.open(r.path, "_blank", "noopener"),
  });

  return (
    <>
      <PageHeader
        title="Accounts"
        description="Hosting accounts on this server. Open an account to manage its files, domains, email and databases."
        actions={
          <Button
            variant="primary"
            icon={Plus}
            onClick={() => {
              setF({ ...blank, package_id: pkgs.data?.[0]?.id ?? "" });
              setCreating(true);
            }}
          >
            Create account
          </Button>
        }
      />
      <div className="relative mb-4 max-w-sm">
        <Search className="pointer-events-none absolute left-2.5 top-2 size-4 text-subtle" aria-hidden />
        <Input aria-label="Search accounts" className="pl-8" placeholder="Search domain, username or email" value={search} onChange={(e) => setSearch(e.target.value)} />
      </div>

      <Card flush>
        {q.isLoading ? (
          <Loading />
        ) : q.error ? (
          <div className="p-4">
            <ErrorNote error={q.error} />
          </div>
        ) : !q.data?.length ? (
          <EmptyState
            icon={Users}
            title={debounced ? "No matching accounts" : "No accounts yet"}
            description={debounced ? "Try a different search." : "Create a package first, then your first hosting account."}
          />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Domain</Th>
                <Th>User</Th>
                <Th>Package</Th>
                <Th>Disk</Th>
                <Th>Status</Th>
                <Th>Created</Th>
                <Th className="w-10">
                  <span className="sr-only">Actions</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((a) => (
                <Tr key={a.id}>
                  <Td>
                    <Link to={`/accounts/${a.id}`} className="font-medium text-ink hover:text-accent">
                      {a.primary_domain}
                    </Link>
                    <div className="text-[12px] text-muted">{a.email}</div>
                  </Td>
                  <Td className="text-muted">{a.sys_user}</Td>
                  <Td>{a.package_name}</Td>
                  <Td className="tabular-nums text-muted">{formatMB(a.disk_used_mb)}</Td>
                  <Td>
                    <Badge tone={a.status === "active" ? "ok" : "bad"}>{a.status === "active" ? "Active" : "Suspended"}</Badge>
                  </Td>
                  <Td className="text-muted">{timeAgo(a.created_at)}</Td>
                  <Td>
                    <div className="relative" ref={menu === a.id ? menuRef : undefined}>
                      <IconButton icon={MoreHorizontal} label={`Actions for ${a.primary_domain}`} onClick={() => setMenu(menu === a.id ? null : a.id)} />
                      {menu === a.id && (
                        <div role="menu" className="absolute right-0 z-20 mt-1 w-48 overflow-hidden rounded-lg border border-line bg-surface py-1 text-[13px] shadow-pop">
                          <MenuItem icon={LogIn} onClick={() => (setMenu(null), sso.mutate(a))}>
                            Sign in as customer
                          </MenuItem>
                          {a.status === "active" ? (
                            <MenuItem icon={Ban} onClick={() => (setMenu(null), setSuspend(a))}>
                              Suspend
                            </MenuItem>
                          ) : (
                            <MenuItem icon={Ban} onClick={() => (setMenu(null), unsuspend.mutate(a))}>
                              Reactivate
                            </MenuItem>
                          )}
                          <MenuItem icon={Trash2} danger onClick={() => (setMenu(null), setDel(a))}>
                            Terminate
                          </MenuItem>
                        </div>
                      )}
                    </div>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <Modal
        open={creating}
        onClose={() => setCreating(false)}
        title="Create account"
        description="Provisions a Linux user, web root, PHP pool, DNS zone and resource limits."
        footer={
          <>
            <Button onClick={() => setCreating(false)}>Cancel</Button>
            <Button variant="primary" loading={create.isPending} disabled={!f.username || !f.email || !f.domain || !f.package_id} onClick={() => create.mutate()}>
              Create account
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="Primary domain">
            {(id) => <Input id={id} value={f.domain} onChange={(e) => setF({ ...f, domain: e.target.value })} placeholder="example.com" autoFocus />}
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Username" hint="3–16 lowercase letters or digits">
              {(id) => <Input id={id} value={f.username} onChange={(e) => setF({ ...f, username: e.target.value.toLowerCase() })} autoComplete="off" />}
            </Field>
            <Field label="Contact email">
              {(id) => <Input id={id} type="email" value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} />}
            </Field>
          </div>
          <Field label="Package">
            {(id) => (
              <Select id={id} value={f.package_id} onChange={(e) => setF({ ...f, package_id: e.target.value })}>
                {!pkgs.data?.length && <option value="">Create a package first</option>}
                {pkgs.data?.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Password" hint="Leave blank to generate a strong password.">
            {(id) => <Input id={id} type="password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} autoComplete="new-password" />}
          </Field>
        </div>
      </Modal>

      <Modal open={!!created} onClose={() => setCreated(null)} title="Account created" footer={<Button variant="primary" onClick={() => setCreated(null)}>Done</Button>}>
        <p className="mb-3 text-[13px] text-muted">
          Share these credentials with the customer. The password is shown only once.
        </p>
        <div className="space-y-3">
          <Field label="Username">{() => <CopyField value={created?.username ?? ""} />}</Field>
          <Field label="Password">{() => <CopyField value={created?.password ?? ""} />}</Field>
        </div>
      </Modal>

      <Modal
        open={!!suspend}
        onClose={() => setSuspend(null)}
        title={`Suspend ${suspend?.primary_domain}?`}
        description="Websites and email go offline and the customer is signed out. Data is kept."
        footer={
          <>
            <Button onClick={() => setSuspend(null)}>Cancel</Button>
            <Button variant="danger" loading={doSuspend.isPending} onClick={() => suspend && doSuspend.mutate(suspend)}>
              Suspend account
            </Button>
          </>
        }
      >
        <Field label="Reason (shown to the customer)">
          {(id) => <Input id={id} value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Overdue invoice" />}
        </Field>
      </Modal>

      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        onConfirm={() => del && terminate.mutate(del)}
        loading={terminate.isPending}
        title={`Terminate ${del?.primary_domain}?`}
        message="The Linux user, all files, databases, mailboxes and DNS zones are permanently deleted. This cannot be undone."
        confirmLabel="Terminate account"
      />
    </>
  );
}

function MenuItem({ icon: Icon, children, onClick, danger }: { icon: typeof Ban; children: string; onClick: () => void; danger?: boolean }) {
  return (
    <button
      role="menuitem"
      onClick={onClick}
      className={`flex w-full items-center gap-2.5 px-3 py-1.5 text-left hover:bg-surface2 ${danger ? "text-bad" : "text-ink"}`}
    >
      <Icon className="size-3.5" aria-hidden />
      {children}
    </button>
  );
}
