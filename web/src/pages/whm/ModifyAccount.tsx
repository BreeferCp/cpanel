import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import type { Account, Pkg } from "@/lib/types";
import { Button } from "@/components/ui/Button";
import { Field, Input, Select } from "@/components/ui/Form";
import { Modal } from "@/components/ui/Modal";

const shellHelp = {
  none: "No SSH login. Files are managed through the panel only.",
  sftp: "SFTP file transfer only; no interactive shell.",
  full: "Full Bash shell over SSH and the browser terminal, inside the account's resource limits.",
};

export function ModifyAccount({ account, onClose, invalidate }: { account: Account | null; onClose: () => void; invalidate: string[] }) {
  const pkgs = useGet<Pkg[]>("/packages", !!account);
  const [f, setF] = useState({ package_id: "", email: "", password: "", shell: "none" as Account["shell"] });
  useEffect(() => {
    if (account) setF({ package_id: account.package_id, email: account.email, password: "", shell: account.shell });
  }, [account]);
  const pkg = pkgs.data?.find((p) => p.id === f.package_id);

  const save = useMutate(
    async () => {
      const a = account!;
      const body: Record<string, string> = {};
      if (f.package_id !== a.package_id) body.package_id = f.package_id;
      if (f.email !== a.email) body.email = f.email;
      if (f.password) body.password = f.password;
      if (Object.keys(body).length) await api.patch(`/accounts/${a.id}`, body);
      if (f.shell !== a.shell) await api.put(`/accounts/${a.id}/shell`, { mode: f.shell });
    },
    { invalidate, success: "Account updated", onSuccess: onClose },
  );

  return (
    <Modal
      open={!!account}
      onClose={onClose}
      title={`Modify ${account?.primary_domain}`}
      description={`Linux user ${account?.sys_user}`}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" loading={save.isPending} onClick={() => save.mutate()}>
            Save changes
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="Package" hint="Limits and cgroup quotas are re-applied immediately.">
          {(id) => (
            <Select id={id} value={f.package_id} onChange={(e) => setF({ ...f, package_id: e.target.value })}>
              {pkgs.data?.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </Select>
          )}
        </Field>
        <Field label="Contact email">{(id) => <Input id={id} type="email" value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} />}</Field>
        <Field label="New password" hint="Leave blank to keep the current password. Changing it signs the customer out.">
          {(id) => <Input id={id} type="password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} autoComplete="new-password" />}
        </Field>
        <Field label="Shell access" hint={shellHelp[f.shell]}>
          {(id) => (
            <Select id={id} value={f.shell} onChange={(e) => setF({ ...f, shell: e.target.value as Account["shell"] })}>
              <option value="none">Disabled</option>
              <option value="sftp">SFTP only</option>
              <option value="full" disabled={!pkg?.shell_access}>
                Full shell{pkg?.shell_access ? "" : " (not allowed by package)"}
              </option>
            </Select>
          )}
        </Field>
      </div>
    </Modal>
  );
}
