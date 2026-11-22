import { KeyRound, Trash2, UserPlus, UsersRound } from "lucide-react";
import { useMemo, useState } from "react";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import type { Domain, FtpAccount, Mailbox } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button, IconButton } from "@/components/ui/Button";
import { DataTable } from "@/components/ui/DataTable";
import { Checkbox, Field, Input, Select } from "@/components/ui/Form";
import { Badge, PageHeader } from "@/components/ui/Layout";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

interface Person {
  login: string;
  mail?: Mailbox;
  ftp?: FtpAccount;
}

const blank = { name: "", domain: "", password: "", mail: true, ftp: false };

/**
 * One person, one login: the mailbox and the FTP account with the same
 * name@domain are managed together.
 */
export function UserManager() {
  const aid = useAccountId();
  const base = `/accounts/${aid}`;
  const keys = [`${base}/mailboxes`, `${base}/ftp`];
  const mail = useGet<Mailbox[]>(`${base}/mailboxes`);
  const ftp = useGet<FtpAccount[]>(`${base}/ftp`);
  const domains = useGet<Domain[]>(`${base}/domains`);
  const hosts = domains.data?.filter((d) => d.kind === "primary" || d.kind === "addon") ?? [];
  const [adding, setAdding] = useState(false);
  const [f, setF] = useState(blank);
  const [changing, setChanging] = useState<Person | null>(null);
  const [password, setPassword] = useState("");
  const [removing, setRemoving] = useState<Person | null>(null);

  const people = useMemo<Person[]>(() => {
    const by = new Map<string, Person>();
    const of = (login: string) => {
      if (!by.has(login)) by.set(login, { login });
      return by.get(login)!;
    };
    mail.data?.forEach((m) => (of(m.address).mail = m));
    ftp.data?.forEach((x) => (of(x.login).ftp = x));
    return [...by.values()];
  }, [mail.data, ftp.data]);

  const create = useMutate(
    async () => {
      const d = hosts.find((h) => h.id === f.domain) ?? hosts[0];
      if (!d) throw new Error("Add a domain first");
      let box: { id: string } | undefined;
      if (f.mail) box = await api.post<{ id: string }>(`${base}/mailboxes`, { local_part: f.name, domain_id: d.id, password: f.password, quota_mb: 1024 });
      if (f.ftp) {
        try {
          await api.post(`${base}/ftp`, { name: f.name, domain: d.name, password: f.password, directory: "public_html", quota_mb: 0 });
        } catch (e) {
          // Both or neither. The mailbox was created a moment ago and is empty.
          if (box) await api.del(`${base}/mailboxes/${box.id}`);
          throw e;
        }
      }
    },
    {
      invalidate: keys,
      success: "User created",
      onSuccess: () => {
        setAdding(false);
        setF(blank);
      },
    },
  );
  const change = useMutate(
    async () => {
      if (changing?.mail) await api.put(`${base}/mailboxes/${changing.mail.id}/password`, { password });
      if (changing?.ftp) await api.patch(`${base}/ftp/${changing.ftp.id}`, { password });
    },
    {
      invalidate: keys,
      success: "Password changed",
      onSuccess: () => {
        setChanging(null);
        setPassword("");
      },
    },
  );
  const remove = useMutate(
    async () => {
      if (removing?.ftp) await api.del(`${base}/ftp/${removing.ftp.id}`);
      if (removing?.mail) await api.del(`${base}/mailboxes/${removing.mail.id}`);
    },
    { invalidate: keys, success: "User deleted", onSuccess: () => setRemoving(null) },
  );

  return (
    <>
      <PageHeader
        title="User Manager"
        actions={
          <Button variant="primary" icon={UserPlus} onClick={() => setAdding(true)}>
            Add user
          </Button>
        }
      />
      <DataTable
        rows={mail.data || ftp.data ? people : undefined}
        loading={mail.isLoading || ftp.isLoading}
        error={mail.error ?? ftp.error}
        rowKey={(p) => p.login}
        search={(p) => p.login}
        searchPlaceholder="Search users"
        empty={{ icon: UsersRound, title: "No users yet", description: "Give a colleague email, FTP or both under one name and password." }}
        initialSort={{ key: "login", dir: "asc" }}
        columns={[
          { key: "login", header: "User", sort: (p) => p.login, render: (p) => <span className="font-medium text-ink">{p.login}</span> },
          {
            key: "services",
            header: "Services",
            render: (p) => (
              <span className="inline-flex flex-wrap gap-1.5">
                {p.mail && <Badge tone="accent">Email</Badge>}
                {p.ftp && <Badge tone="ok">FTP</Badge>}
              </span>
            ),
          },
        ]}
        actions={(p) => (
          <>
            <IconButton icon={KeyRound} label={`Change password of ${p.login}`} onClick={() => setChanging(p)} />
            <IconButton icon={Trash2} label={`Delete ${p.login}`} onClick={() => setRemoving(p)} />
          </>
        )}
      />

      <Modal
        open={adding}
        onClose={() => setAdding(false)}
        title="Add user"
        footer={
          <>
            <Button onClick={() => setAdding(false)}>Cancel</Button>
            <Button variant="primary" loading={create.isPending} disabled={!f.name || !f.password || (!f.mail && !f.ftp)} onClick={() => create.mutate()}>
              Create user
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="Name">{(id) => <Input id={id} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} autoFocus autoComplete="off" />}</Field>
          <Field label="Domain">
            {(id) => (
              <Select id={id} value={f.domain || hosts[0]?.id} onChange={(e) => setF({ ...f, domain: e.target.value })}>
                {hosts.map((d) => (
                  <option key={d.id} value={d.id}>
                    @{d.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Password" hint="At least 12 characters, mixing letters with digits or symbols.">
            {(id) => <Input id={id} type="password" autoComplete="new-password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} />}
          </Field>
          <div className="flex flex-wrap gap-4 text-[13px] text-ink">
            <label className="inline-flex items-center gap-2">
              <Checkbox checked={f.mail} onChange={(e) => setF({ ...f, mail: e.target.checked })} /> Email account
            </label>
            <label className="inline-flex items-center gap-2">
              <Checkbox checked={f.ftp} onChange={(e) => setF({ ...f, ftp: e.target.checked })} /> FTP (public_html)
            </label>
          </div>
        </div>
      </Modal>

      <Modal
        open={!!changing}
        onClose={() => setChanging(null)}
        title={`Password of ${changing?.login}`}
        description="Changes every service this user has."
        footer={
          <>
            <Button onClick={() => setChanging(null)}>Cancel</Button>
            <Button variant="primary" loading={change.isPending} disabled={!password} onClick={() => change.mutate()}>
              Change password
            </Button>
          </>
        }
      >
        <Field label="New password">
          {(id) => <Input id={id} type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} autoFocus />}
        </Field>
      </Modal>

      <ConfirmDialog
        open={!!removing}
        onClose={() => setRemoving(null)}
        onConfirm={() => remove.mutate()}
        loading={remove.isPending}
        title={`Delete ${removing?.login}?`}
        message={removing?.mail ? "The mailbox and all mail in it are permanently deleted, with any FTP login of the same name." : "The FTP login is deleted. Its files are kept."}
      />
    </>
  );
}
