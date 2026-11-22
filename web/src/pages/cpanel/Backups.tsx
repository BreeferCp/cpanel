import { Archive, DatabaseBackup, Download, FolderArchive, RotateCcw, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { formatBytes, formatDate } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { Backup } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button, IconButton } from "@/components/ui/Button";
import { Badge, Card, EmptyState, ErrorNote, Loading, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { Field, Select } from "@/components/ui/Form";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

const kindLabel: Record<string, string> = { full: "Full account", files: "Home directory", databases: "Databases", mail: "Mail" };

export default function Backups() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/backups`;
  // Poll every 3s while any backup is still running.
  const q = useGet<Backup[]>(key, true, (d) => (d?.some((b) => b.status === "running") ? 3000 : false));
  const [restore, setRestore] = useState<Backup | null>(null);
  const [del, setDel] = useState<Backup | null>(null);

  const create = useMutate((kind: string) => api.post(key, { kind }), { invalidate: [key], success: "Backup started" });
  const [part, setPart] = useState("files");
  const doRestore = useMutate((id: string) => api.post(`${key}/${id}/restore?part=${part}`), { success: "Backup restored", onSuccess: () => setRestore(null) });
  const remove = useMutate((id: string) => api.del(`${key}/${id}`), { invalidate: [key], success: "Backup deleted", onSuccess: () => setDel(null) });

  return (
    <>
      <PageHeader
        title="Backups"
        description="Point-in-time archives of your files and databases."
        actions={
          <>
            <Button icon={DatabaseBackup} loading={create.isPending && create.variables === "databases"} onClick={() => create.mutate("databases")}>
              Databases only
            </Button>
            <Button icon={FolderArchive} loading={create.isPending && create.variables === "files"} onClick={() => create.mutate("files")}>
              Files only
            </Button>
            <Button variant="primary" icon={Archive} loading={create.isPending && create.variables === "full"} onClick={() => create.mutate("full")}>
              Create full backup
            </Button>
          </>
        }
      />
      <Card flush>
        {q.isLoading ? (
          <Loading />
        ) : q.error ? (
          <div className="p-4">
            <ErrorNote error={q.error} />
          </div>
        ) : !q.data?.length ? (
          <EmptyState icon={Archive} title="No backups yet" description="Create a full backup before making large changes to your site." />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Created</Th>
                <Th>Contents</Th>
                <Th>Size</Th>
                <Th>Status</Th>
                <Th className="text-right">Actions</Th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((b) => (
                <Tr key={b.id}>
                  <Td className="text-ink">{formatDate(b.created_at)}</Td>
                  <Td>{kindLabel[b.kind] ?? b.kind}</Td>
                  <Td className="tabular-nums text-muted">{b.status === "complete" ? formatBytes(b.size_bytes) : "—"}</Td>
                  <Td>
                    <Badge tone={b.status === "complete" ? "ok" : b.status === "failed" ? "bad" : "warn"}>
                      {b.status === "running" ? "In progress" : b.status === "complete" ? "Complete" : "Failed"}
                    </Badge>
                  </Td>
                  <Td>
                    <div className="flex justify-end">
                      {b.status === "complete" && (
                        <a href={`/api/v1${key}/${b.id}/download`} download aria-label="Download backup" title="Download backup" className="inline-flex size-7 items-center justify-center rounded-md text-muted hover:bg-surface2 hover:text-ink">
                          <Download className="size-4" aria-hidden />
                        </a>
                      )}
                      <IconButton
                        icon={RotateCcw}
                        label="Restore from this backup"
                        disabled={b.status !== "complete"}
                        onClick={() => {
                          setPart(b.kind === "databases" ? "databases" : "files");
                          setRestore(b);
                        }}
                      />
                      <IconButton icon={Trash2} label="Delete backup" onClick={() => setDel(b)} />
                    </div>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <Modal
        open={!!restore}
        onClose={() => setRestore(null)}
        title="Restore backup"
        description={restore ? `Taken ${formatDate(restore.created_at)}` : undefined}
        footer={
          <>
            <Button onClick={() => setRestore(null)}>Cancel</Button>
            <Button variant="danger" loading={doRestore.isPending} onClick={() => restore && doRestore.mutate(restore.id)}>
              Restore
            </Button>
          </>
        }
      >
        <div className="space-y-3">
          <Field label="What to restore">
            {(id) => (
              <Select id={id} value={part} onChange={(e) => setPart(e.target.value)}>
                {restore?.kind !== "databases" && <option value="files">Files only</option>}
                {restore?.kind !== "files" && <option value="databases">Databases only</option>}
                {restore?.kind === "full" && <option value="all">Files and databases</option>}
              </Select>
            )}
          </Field>
          <p className="text-[13px] text-muted">Files with the same name and tables with the same name are replaced. Anything newer in them is lost.</p>
        </div>
      </Modal>
      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        onConfirm={() => del && remove.mutate(del.id)}
        loading={remove.isPending}
        title="Delete this backup?"
        message="The archive is permanently deleted from the server."
      />
    </>
  );
}
