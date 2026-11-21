import { DownloadCloud, GitBranch, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { GitRepo } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button, IconButton } from "@/components/ui/Button";
import { CopyField } from "@/components/ui/CopyField";
import { Field, Input, Switch } from "@/components/ui/Form";
import { Badge, Card, EmptyState, Loading, Mono, PageHeader } from "@/components/ui/Layout";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

export default function Git() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/git`;
  const q = useGet<GitRepo[]>(key);
  const [open, setOpen] = useState(false);
  const [clone, setClone] = useState(true);
  const [f, setF] = useState({ name: "", path: "repositories/", clone_url: "https://" });
  const [del, setDel] = useState<GitRepo | null>(null);

  const create = useMutate(() => api.post(key, { name: f.name, path: f.path, clone_url: clone ? f.clone_url : "" }), {
    invalidate: [key],
    success: clone ? "Repository cloned" : "Repository created",
    onSuccess: () => setOpen(false),
  });
  const pull = useMutate((id: string) => api.post(`${key}/${id}/pull`), { invalidate: [key], success: "Pulled the latest changes" });
  const remove = useMutate((id: string) => api.del(`${key}/${id}`), { invalidate: [key], success: "Repository removed from the list", onSuccess: () => setDel(null) });

  return (
    <>
      <PageHeader
        title="Git Version Control"
        description="Clone a repository from GitHub, GitLab or any https remote, or create an empty one you can push to over SSH."
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setOpen(true)}>
            Create repository
          </Button>
        }
      />
      {q.isLoading ? (
        <Loading />
      ) : !q.data?.length ? (
        <Card>
          <EmptyState icon={GitBranch} title="No repositories" description="Deploy a site straight from Git: clone it into public_html or a subfolder." />
        </Card>
      ) : (
        <div className="grid gap-4 lg:grid-cols-2">
          {q.data.map((r) => (
            <Card
              key={r.id}
              title={r.name}
              description={`~/${r.path}`}
              actions={
                <>
                  {r.clone_url && (
                    <Button size="sm" icon={DownloadCloud} loading={pull.isPending && pull.variables === r.id} onClick={() => pull.mutate(r.id)}>
                      Pull
                    </Button>
                  )}
                  <IconButton icon={Trash2} label={`Remove ${r.name}`} onClick={() => setDel(r)} />
                </>
              }
            >
              <div className="space-y-3 text-[13px]">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge tone="accent">
                    <GitBranch className="size-3" aria-hidden /> {r.info.branch || "no commits"}
                  </Badge>
                  {r.info.head && <Mono className="text-muted">{r.info.head}</Mono>}
                  <span className="min-w-0 truncate text-ink">{r.info.commit}</span>
                  {r.info.when && <span className="text-muted">· {r.info.when}</span>}
                </div>
                {r.clone_url && (
                  <div>
                    <div className="mb-1 text-[12px] text-muted">Remote</div>
                    <Mono className="block truncate">{r.clone_url}</Mono>
                  </div>
                )}
                <div>
                  <div className="mb-1 text-[12px] text-muted">Push over SSH</div>
                  <CopyField value={r.ssh_url} />
                </div>
                <div className="text-[12px] text-subtle">Added {formatDate(r.created_at)}</div>
              </div>
            </Card>
          ))}
        </div>
      )}

      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title="Create repository"
        footer={
          <>
            <Button onClick={() => setOpen(false)}>Cancel</Button>
            <Button variant="primary" loading={create.isPending} onClick={() => create.mutate()}>
              {clone ? "Clone" : "Create"}
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="flex items-center justify-between rounded-md border border-line px-3 py-2">
            <span className="text-[13px]">Clone from a remote</span>
            <Switch checked={clone} onChange={setClone} label="Clone from a remote" />
          </div>
          {clone && (
            <Field label="Clone URL" hint="Public https URL. For private repositories, create an empty repository and push to it.">
              {(id) => <Input id={id} className="font-mono" value={f.clone_url} onChange={(e) => setF({ ...f, clone_url: e.target.value })} />}
            </Field>
          )}
          <Field label="Folder" hint="A new folder inside your home, e.g. public_html/app or repositories/site">
            {(id) => <Input id={id} className="font-mono" value={f.path} onChange={(e) => setF({ ...f, path: e.target.value })} />}
          </Field>
          <Field label="Name">{(id) => <Input id={id} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="Company website" />}</Field>
        </div>
      </Modal>
      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        onConfirm={() => del && remove.mutate(del.id)}
        loading={remove.isPending}
        confirmLabel="Remove"
        title="Remove repository"
        message={<>The panel stops tracking {del?.name}. Its files stay in ~/{del?.path}.</>}
      />
    </>
  );
}
