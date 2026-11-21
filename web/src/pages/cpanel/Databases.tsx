import { Database as DbIcon, ExternalLink, KeyRound, Plus, Trash2, UserPlus } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { formatMB } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { Account, Database, DbUser } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button, IconButton } from "@/components/ui/Button";
import { Field, Input, Select } from "@/components/ui/Form";
import { Badge, Card, EmptyState, ErrorNote, Loading, Mono, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

export default function Databases() {
  const aid = useAccountId();
  const dbKey = `/accounts/${aid}/databases`;
  const userKey = `/accounts/${aid}/db-users`;
  const dbs = useGet<Database[]>(dbKey);
  const users = useGet<DbUser[]>(userKey);
  const acct = useGet<Account>(`/accounts/${aid}`);
  const prefix = acct.data ? `${acct.data.sys_user}_` : "";

  const [modal, setModal] = useState<"db" | "user" | "grant" | null>(null);
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [grant, setGrant] = useState({ db: "", user: "" });
  const [pwUser, setPwUser] = useState<DbUser | null>(null);
  const [del, setDel] = useState<{ kind: "db" | "user"; id: string; name: string } | null>(null);

  const close = () => {
    setModal(null);
    setName("");
    setPassword("");
  };

  const createDb = useMutate(() => api.post(dbKey, { name }), { invalidate: [dbKey], success: "Database created", onSuccess: close });
  const createUser = useMutate(() => api.post(userKey, { name, password }), { invalidate: [userKey], success: "User created", onSuccess: close });
  const setPw = useMutate((id: string) => api.put(`${userKey}/${id}/password`, { password }), {
    success: "Password changed",
    onSuccess: () => {
      setPwUser(null);
      setPassword("");
    },
  });
  const doGrant = useMutate(() => api.put(`${dbKey}/${grant.db}/grants/${grant.user}`, { privileges: [] }), {
    invalidate: [dbKey],
    success: "User added to database",
    onSuccess: () => setModal(null),
  });
  const revoke = useMutate((v: { db: string; user: string }) => api.del(`${dbKey}/${v.db}/grants/${v.user}`), {
    invalidate: [dbKey],
    success: "Access removed",
  });
  const doDelete = useMutate(
    () => (del!.kind === "db" ? api.del(`${dbKey}/${del!.id}`) : api.del(`${userKey}/${del!.id}`)),
    { invalidate: [dbKey, userKey], success: "Deleted", onSuccess: () => setDel(null) },
  );

  // Single sign-on: the panel mints a short-lived database user for this session.
  const pma = useMutate(() => api.post<{ url: string }>(`/accounts/${aid}/phpmyadmin`), {
    onSuccess: (r) => window.open(r.url, "_blank", "noopener"),
  });

  const userName = (id: string) => users.data?.find((u) => u.id === id)?.name ?? id;

  return (
    <>
      <PageHeader
        title="Databases"
        description="MariaDB databases and users. Every name is prefixed with your account name."
        actions={
          <>
            <Button icon={ExternalLink} loading={pma.isPending} disabled={!dbs.data?.length} onClick={() => pma.mutate()}>
              phpMyAdmin
            </Button>
            <Button icon={UserPlus} onClick={() => setModal("user")}>
              New user
            </Button>
            <Button variant="primary" icon={Plus} onClick={() => setModal("db")}>
              New database
            </Button>
          </>
        }
      />

      <div className="space-y-5">
        <Card
          title="Databases"
          flush
          actions={
            <Button size="sm" onClick={() => setModal("grant")} disabled={!dbs.data?.length || !users.data?.length}>
              Add user to database
            </Button>
          }
        >
          {dbs.isLoading ? (
            <Loading />
          ) : dbs.error ? (
            <div className="p-4">
              <ErrorNote error={dbs.error} />
            </div>
          ) : !dbs.data?.length ? (
            <EmptyState icon={DbIcon} title="No databases" description="Create a database, then a user, then give the user access to it." />
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Name</Th>
                  <Th>Size</Th>
                  <Th>Users with access</Th>
                  <Th className="w-10">
                    <span className="sr-only">Actions</span>
                  </Th>
                </tr>
              </thead>
              <tbody>
                {dbs.data.map((d) => (
                  <Tr key={d.id}>
                    <Td>
                      <Mono className="font-medium text-ink">{d.name}</Mono>
                    </Td>
                    <Td className="text-muted">{formatMB(d.size_mb)}</Td>
                    <Td>
                      <div className="flex flex-wrap gap-1.5">
                        {d.users.length === 0 && <span className="text-muted">None</span>}
                        {d.users.map((uid) => (
                          <span key={uid} className="inline-flex items-center gap-1 rounded border border-line bg-surface2 py-px pl-1.5 pr-0.5 text-[12px]">
                            <Mono>{userName(uid)}</Mono>
                            <button
                              aria-label={`Remove ${userName(uid)} from ${d.name}`}
                              onClick={() => revoke.mutate({ db: d.id, user: uid })}
                              className="rounded px-1 text-subtle hover:bg-line hover:text-ink"
                            >
                              ×
                            </button>
                          </span>
                        ))}
                      </div>
                    </Td>
                    <Td>
                      <IconButton icon={Trash2} label={`Delete ${d.name}`} onClick={() => setDel({ kind: "db", id: d.id, name: d.name })} />
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>

        <Card title="Database users" flush>
          {users.isLoading ? (
            <Loading />
          ) : !users.data?.length ? (
            <EmptyState icon={UserPlus} title="No database users" description="Applications connect to a database with a user and password." />
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Username</Th>
                  <Th>Host</Th>
                  <Th className="w-24">
                    <span className="sr-only">Actions</span>
                  </Th>
                </tr>
              </thead>
              <tbody>
                {users.data.map((u) => (
                  <Tr key={u.id}>
                    <Td>
                      <Mono className="font-medium text-ink">{u.name}</Mono>
                    </Td>
                    <Td>
                      <Badge>localhost</Badge>
                    </Td>
                    <Td>
                      <div className="flex justify-end">
                        <IconButton icon={KeyRound} label={`Change password for ${u.name}`} onClick={() => setPwUser(u)} />
                        <IconButton icon={Trash2} label={`Delete ${u.name}`} onClick={() => setDel({ kind: "user", id: u.id, name: u.name })} />
                      </div>
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>
      </div>

      <Modal
        open={modal === "db"}
        onClose={close}
        title="New database"
        footer={
          <>
            <Button onClick={close}>Cancel</Button>
            <Button variant="primary" loading={createDb.isPending} disabled={!name} onClick={() => createDb.mutate()}>
              Create database
            </Button>
          </>
        }
      >
        <Field label="Database name" hint={`Created as ${prefix}${name || "name"}`}>
          {(id) => <Input id={id} value={name} onChange={(e) => setName(e.target.value)} placeholder="wordpress" autoFocus />}
        </Field>
      </Modal>

      <Modal
        open={modal === "user"}
        onClose={close}
        title="New database user"
        footer={
          <>
            <Button onClick={close}>Cancel</Button>
            <Button variant="primary" loading={createUser.isPending} disabled={!name || !password} onClick={() => createUser.mutate()}>
              Create user
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="Username" hint={`Created as ${prefix}${name || "name"}`}>
            {(id) => <Input id={id} value={name} onChange={(e) => setName(e.target.value)} autoFocus />}
          </Field>
          <Field label="Password" hint="At least 12 characters, mixing letters with digits or symbols.">
            {(id) => <Input id={id} type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />}
          </Field>
        </div>
      </Modal>

      <Modal
        open={modal === "grant"}
        onClose={() => setModal(null)}
        title="Add user to database"
        description="The user receives all privileges on the selected database."
        footer={
          <>
            <Button onClick={() => setModal(null)}>Cancel</Button>
            <Button variant="primary" loading={doGrant.isPending} disabled={!grant.db || !grant.user} onClick={() => doGrant.mutate()}>
              Grant access
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="User">
            {(id) => (
              <Select id={id} value={grant.user} onChange={(e) => setGrant({ ...grant, user: e.target.value })}>
                <option value="">Select a user…</option>
                {users.data?.map((u) => (
                  <option key={u.id} value={u.id}>
                    {u.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Database">
            {(id) => (
              <Select id={id} value={grant.db} onChange={(e) => setGrant({ ...grant, db: e.target.value })}>
                <option value="">Select a database…</option>
                {dbs.data?.map((d) => (
                  <option key={d.id} value={d.id}>
                    {d.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
        </div>
      </Modal>

      <Modal
        open={!!pwUser}
        onClose={() => setPwUser(null)}
        title={`Change password for ${pwUser?.name}`}
        footer={
          <>
            <Button onClick={() => setPwUser(null)}>Cancel</Button>
            <Button variant="primary" loading={setPw.isPending} disabled={!password} onClick={() => pwUser && setPw.mutate(pwUser.id)}>
              Change password
            </Button>
          </>
        }
      >
        <Field label="New password">
          {(id) => <Input id={id} type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" autoFocus />}
        </Field>
      </Modal>

      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        onConfirm={() => doDelete.mutate()}
        loading={doDelete.isPending}
        title={`Delete ${del?.name}?`}
        message={del?.kind === "db" ? "All tables and data in this database will be permanently destroyed." : "Applications using this user will lose database access."}
      />
    </>
  );
}
