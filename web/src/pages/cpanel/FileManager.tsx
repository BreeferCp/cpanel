import clsx from "clsx";
import {
  Archive,
  ArrowUp,
  ChevronRight,
  Copy,
  Download,
  File as FileIcon,
  FilePlus,
  FileText,
  Folder,
  FolderPlus,
  Home,
  Pencil,
  PackageOpen,
  Shield,
  Trash2,
  Upload,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState, type DragEvent } from "react";
import { api, downloadUrl } from "@/lib/api";
import { formatBytes, timeAgo } from "@/lib/format";
import { useGet, useMutate } from "@/lib/hooks";
import type { FileEntry } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button, IconButton } from "@/components/ui/Button";
import { Checkbox, Field, Input, Textarea } from "@/components/ui/Form";
import { Card, EmptyState, ErrorNote, Loading, Mono, PageHeader } from "@/components/ui/Layout";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/Toast";

type Dialog =
  | { kind: "mkdir" }
  | { kind: "newfile" }
  | { kind: "rename"; entry: FileEntry }
  | { kind: "copy"; entry: FileEntry }
  | { kind: "chmod"; entry: FileEntry }
  | { kind: "edit"; entry: FileEntry }
  | { kind: "delete"; paths: string[] }
  | { kind: "compress" }
  | null;

const join = (dir: string, name: string) => (dir === "/" ? `/${name}` : `${dir}/${name}`);
const parentOf = (p: string) => (p === "/" ? "/" : p.slice(0, p.lastIndexOf("/")) || "/");
const isZip = (n: string) => n.toLowerCase().endsWith(".zip");

export default function FileManager() {
  const aid = useAccountId();
  const toast = useToast();
  const [dir, setDir] = useState("/");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [dialog, setDialog] = useState<Dialog>(null);
  const [dragging, setDragging] = useState(false);
  const [uploading, setUploading] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);

  const base = `/accounts/${aid}/files`;
  const listKey = `${base}?path=${encodeURIComponent(dir)}`;
  const list = useGet<FileEntry[]>(listKey);
  const entries = list.data ?? [];

  const refresh = [listKey];
  const done = (msg: string) => ({
    invalidate: refresh,
    success: msg,
    onSuccess: () => {
      setDialog(null);
      setSelected(new Set());
    },
  });

  const mkdir = useMutate((name: string) => api.post(`${base}/mkdir`, { path: join(dir, name) }), done("Folder created"));
  const newFile = useMutate((name: string) => api.put(`${base}/content`, { path: join(dir, name), content: "" }), done("File created"));
  const rename = useMutate((v: { from: string; to: string }) => api.post(`${base}/rename`, v), done("Renamed"));
  const copy = useMutate((v: { from: string; to: string }) => api.post(`${base}/copy`, v), done("Copied"));
  const chmod = useMutate((v: { path: string; mode: string }) => api.post(`${base}/chmod`, v), done("Permissions updated"));
  const remove = useMutate((paths: string[]) => api.post(`${base}/delete`, { paths }), done("Deleted"));
  const compress = useMutate((v: { paths: string[]; dest: string }) => api.post(`${base}/compress`, v), done("Archive created"));
  const extract = useMutate((e: FileEntry) => api.post(`${base}/extract`, { path: e.path, dest: dir }), done("Archive extracted"));

  const upload = useCallback(
    async (files: FileList | File[]) => {
      const arr = Array.from(files);
      if (!arr.length) return;
      setUploading(true);
      try {
        const form = new FormData();
        arr.forEach((f) => form.append("file", f, f.name));
        await api.upload(`${base}/upload?path=${encodeURIComponent(dir)}`, form);
        toast.success(arr.length === 1 ? `Uploaded ${arr[0]!.name}` : `Uploaded ${arr.length} files`);
        await list.refetch();
      } catch (e) {
        toast.error(e);
      } finally {
        setUploading(false);
      }
    },
    [base, dir, list, toast],
  );

  const onDrop = (e: DragEvent) => {
    e.preventDefault();
    setDragging(false);
    if (e.dataTransfer.files.length) void upload(e.dataTransfer.files);
  };

  const crumbs = useMemo(() => {
    const parts = dir.split("/").filter(Boolean);
    return parts.map((name, i) => ({ name, path: "/" + parts.slice(0, i + 1).join("/") }));
  }, [dir]);

  const open = (e: FileEntry) => {
    if (e.dir) {
      setDir(e.path);
      setSelected(new Set());
    } else if (isZip(e.name)) {
      extract.mutate(e);
    } else {
      setDialog({ kind: "edit", entry: e });
    }
  };

  const allSelected = entries.length > 0 && selected.size === entries.length;
  const toggle = (p: string) =>
    setSelected((s) => {
      const n = new Set(s);
      if (n.has(p)) n.delete(p);
      else n.add(p);
      return n;
    });

  return (
    <>
      <PageHeader
        title="File Manager"
        description="Browse and edit the files in your home directory."
        actions={
          <>
            <Button icon={FolderPlus} onClick={() => setDialog({ kind: "mkdir" })}>
              New folder
            </Button>
            <Button icon={FilePlus} onClick={() => setDialog({ kind: "newfile" })}>
              New file
            </Button>
            <Button variant="primary" icon={Upload} loading={uploading} onClick={() => fileInput.current?.click()}>
              Upload
            </Button>
            <input ref={fileInput} type="file" multiple hidden onChange={(e) => e.target.files && void upload(e.target.files)} />
          </>
        }
      />

      <Card flush>
        {/* Path bar + bulk actions */}
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-line px-3 py-2">
          <nav aria-label="Current folder" className="flex min-w-0 items-center gap-0.5 text-[13px]">
            <IconButton icon={ArrowUp} label="Up one level" disabled={dir === "/"} onClick={() => setDir(parentOf(dir))} />
            <button onClick={() => setDir("/")} className="flex items-center gap-1 rounded px-1.5 py-1 text-muted hover:bg-surface2 hover:text-ink">
              <Home className="size-3.5" aria-hidden />
              <span>home</span>
            </button>
            {crumbs.map((c) => (
              <span key={c.path} className="flex items-center">
                <ChevronRight className="size-3.5 text-subtle" aria-hidden />
                <button onClick={() => setDir(c.path)} className="rounded px-1.5 py-1 text-ink hover:bg-surface2">
                  {c.name}
                </button>
              </span>
            ))}
          </nav>
          {selected.size > 0 && (
            <div className="flex items-center gap-2">
              <span className="text-[12.5px] text-muted">{selected.size} selected</span>
              <Button size="sm" icon={Archive} onClick={() => setDialog({ kind: "compress" })}>
                Compress
              </Button>
              <Button size="sm" variant="danger" icon={Trash2} onClick={() => setDialog({ kind: "delete", paths: [...selected] })}>
                Delete
              </Button>
            </div>
          )}
        </div>

        {/* Listing, also a drop target */}
        <div
          onDragOver={(e) => {
            e.preventDefault();
            setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={onDrop}
          className={clsx("relative min-h-[280px]", dragging && "bg-accent-soft")}
        >
          {dragging && (
            <div className="pointer-events-none absolute inset-2 z-10 flex items-center justify-center rounded-lg border-2 border-dashed border-accent text-[13px] font-medium text-accent">
              Drop files to upload to {dir}
            </div>
          )}
          {list.isLoading ? (
            <Loading />
          ) : list.error ? (
            <div className="p-4">
              <ErrorNote error={list.error} />
            </div>
          ) : entries.length === 0 ? (
            <EmptyState icon={Folder} title="This folder is empty" description="Drag files here, or use Upload." />
          ) : (
            <table className="w-full border-collapse text-left text-[13px]">
              <thead>
                <tr className="border-b border-line text-[11.5px] uppercase tracking-wide text-muted">
                  <th className="w-10 px-4 py-2">
                    <Checkbox
                      aria-label="Select all"
                      checked={allSelected}
                      onChange={() => setSelected(allSelected ? new Set() : new Set(entries.map((e) => e.path)))}
                    />
                  </th>
                  <th className="px-2 py-2 font-semibold">Name</th>
                  <th className="hidden px-4 py-2 text-right font-semibold sm:table-cell">Size</th>
                  <th className="hidden px-4 py-2 font-semibold md:table-cell">Modified</th>
                  <th className="hidden px-4 py-2 font-semibold md:table-cell">Perms</th>
                  <th className="w-44 px-4 py-2">
                    <span className="sr-only">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {entries.map((e) => (
                  <tr key={e.path} className="group border-b border-line last:border-b-0 hover:bg-surface2/60">
                    <td className="px-4 py-1.5">
                      <Checkbox aria-label={`Select ${e.name}`} checked={selected.has(e.path)} onChange={() => toggle(e.path)} />
                    </td>
                    <td className="px-2 py-1.5">
                      <button onClick={() => open(e)} className="flex max-w-full items-center gap-2 text-left">
                        {e.dir ? (
                          <Folder className="size-4 shrink-0 text-accent" aria-hidden />
                        ) : isZip(e.name) ? (
                          <Archive className="size-4 shrink-0 text-subtle" aria-hidden />
                        ) : (
                          <FileIcon className="size-4 shrink-0 text-subtle" aria-hidden />
                        )}
                        <span className={clsx("truncate", e.dir ? "font-medium text-ink" : "text-ink hover:text-accent")}>{e.name}</span>
                        {e.symlink && <span className="text-[11px] text-subtle">link</span>}
                      </button>
                    </td>
                    <td className="hidden px-4 py-1.5 text-right tabular-nums text-muted sm:table-cell">{e.dir ? "—" : formatBytes(e.size)}</td>
                    <td className="hidden px-4 py-1.5 text-muted md:table-cell">{timeAgo(new Date(e.mod_time).getTime() / 1000)}</td>
                    <td className="hidden px-4 py-1.5 md:table-cell">
                      <Mono className="text-muted">{e.mode}</Mono>
                    </td>
                    <td className="px-2 py-1.5">
                      <div className="flex justify-end gap-0.5 opacity-100 sm:opacity-0 sm:transition-opacity sm:group-focus-within:opacity-100 sm:group-hover:opacity-100">
                        {!e.dir && !isZip(e.name) && <IconButton icon={FileText} label={`Edit ${e.name}`} onClick={() => setDialog({ kind: "edit", entry: e })} />}
                        {!e.dir && isZip(e.name) && <IconButton icon={PackageOpen} label={`Extract ${e.name}`} onClick={() => extract.mutate(e)} />}
                        {!e.dir && (
                          <a
                            href={downloadUrl(aid, e.path)}
                            aria-label={`Download ${e.name}`}
                            title="Download"
                            className="inline-flex size-7 items-center justify-center rounded-md text-muted hover:bg-surface2 hover:text-ink"
                          >
                            <Download className="size-4" aria-hidden />
                          </a>
                        )}
                        <IconButton icon={Pencil} label={`Rename ${e.name}`} onClick={() => setDialog({ kind: "rename", entry: e })} />
                        <IconButton icon={Copy} label={`Copy ${e.name}`} onClick={() => setDialog({ kind: "copy", entry: e })} />
                        <IconButton icon={Shield} label={`Permissions for ${e.name}`} onClick={() => setDialog({ kind: "chmod", entry: e })} />
                        <IconButton icon={Trash2} label={`Delete ${e.name}`} onClick={() => setDialog({ kind: "delete", paths: [e.path] })} />
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </Card>

      <NameDialog
        open={dialog?.kind === "mkdir"}
        title="New folder"
        label="Folder name"
        action="Create"
        loading={mkdir.isPending}
        onClose={() => setDialog(null)}
        onSubmit={(v) => mkdir.mutate(v)}
      />
      <NameDialog
        open={dialog?.kind === "newfile"}
        title="New file"
        label="File name"
        action="Create"
        loading={newFile.isPending}
        onClose={() => setDialog(null)}
        onSubmit={(v) => newFile.mutate(v)}
      />
      {dialog?.kind === "rename" && (
        <NameDialog
          open
          title="Rename"
          label="New name"
          action="Rename"
          initial={dialog.entry.name}
          loading={rename.isPending}
          onClose={() => setDialog(null)}
          onSubmit={(v) => rename.mutate({ from: dialog.entry.path, to: join(dir, v) })}
        />
      )}
      {dialog?.kind === "copy" && (
        <NameDialog
          open
          title="Copy"
          label="Copy to (path from home)"
          action="Copy"
          initial={join(dir, `${dialog.entry.name}-copy`)}
          raw
          loading={copy.isPending}
          onClose={() => setDialog(null)}
          onSubmit={(v) => copy.mutate({ from: dialog.entry.path, to: v })}
        />
      )}
      {dialog?.kind === "chmod" && (
        <NameDialog
          open
          title={`Permissions for ${dialog.entry.name}`}
          label="Mode (octal)"
          action="Apply"
          initial={dialog.entry.mode.slice(-3)}
          raw
          loading={chmod.isPending}
          onClose={() => setDialog(null)}
          onSubmit={(v) => chmod.mutate({ path: dialog.entry.path, mode: v })}
        />
      )}
      {dialog?.kind === "compress" && (
        <NameDialog
          open
          title="Compress to zip"
          label="Archive name"
          action="Compress"
          initial="archive.zip"
          loading={compress.isPending}
          onClose={() => setDialog(null)}
          onSubmit={(v) => compress.mutate({ paths: [...selected], dest: join(dir, v.endsWith(".zip") ? v : `${v}.zip`) })}
        />
      )}
      {dialog?.kind === "edit" && <EditorDialog base={base} entry={dialog.entry} onClose={() => setDialog(null)} />}
      <ConfirmDialog
        open={dialog?.kind === "delete"}
        onClose={() => setDialog(null)}
        onConfirm={() => dialog?.kind === "delete" && remove.mutate(dialog.paths)}
        loading={remove.isPending}
        title="Delete permanently?"
        message={
          dialog?.kind === "delete" ? (
            <>
              {dialog.paths.length === 1 ? <Mono>{dialog.paths[0]}</Mono> : `${dialog.paths.length} items`} will be deleted. This cannot be
              undone.
            </>
          ) : null
        }
      />
    </>
  );
}

function NameDialog({
  open,
  title,
  label,
  action,
  initial = "",
  raw,
  loading,
  onClose,
  onSubmit,
}: {
  open: boolean;
  title: string;
  label: string;
  action: string;
  initial?: string;
  raw?: boolean;
  loading?: boolean;
  onClose: () => void;
  onSubmit: (v: string) => void;
}) {
  const [v, setV] = useState(initial);
  // The dialog stays mounted between uses, so reset the field on every open.
  useEffect(() => {
    if (open) setV(initial);
  }, [open, initial]);
  const bad =!raw && (v.includes("/") || v === "." || v === "..");
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={title}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" loading={loading} disabled={!v.trim() || bad} onClick={() => onSubmit(v.trim())}>
            {action}
          </Button>
        </>
      }
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (v.trim() && !bad) onSubmit(v.trim());
        }}
      >
        <Field label={label} error={bad ? "Names cannot contain slashes." : undefined}>
          {(id) => <Input id={id} value={v} onChange={(e) => setV(e.target.value)} autoFocus onFocus={(e) => e.currentTarget.select()} />}
        </Field>
      </form>
    </Modal>
  );
}

function EditorDialog({ base, entry, onClose }: { base: string; entry: FileEntry; onClose: () => void }) {
  const key = `${base}/content?path=${encodeURIComponent(entry.path)}`;
  const q = useGet<{ content: string }>(key);
  const [text, setText] = useState<string | null>(null);
  const value = text ?? q.data?.content ?? "";
  const save = useMutate(() => api.put(`${base}/content`, { path: entry.path, content: value }), {
    invalidate: [key],
    success: "Saved",
    onSuccess: onClose,
  });
  return (
    <Modal
      open
      onClose={onClose}
      width="max-w-4xl"
      title={entry.name}
      description={entry.path}
      footer={
        <>
          <Button onClick={onClose}>Close</Button>
          <Button variant="primary" loading={save.isPending} disabled={text === null} onClick={() => save.mutate()}>
            Save changes
          </Button>
        </>
      }
    >
      {q.isLoading ? (
        <Loading />
      ) : q.error ? (
        <ErrorNote error={q.error} />
      ) : (
        <Textarea
          aria-label={`Contents of ${entry.name}`}
          className="h-[55vh] w-full resize-none whitespace-pre text-[12.5px] leading-5"
          spellCheck={false}
          value={value}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if ((e.ctrlKey || e.metaKey) && e.key === "s") {
              e.preventDefault();
              if (text !== null) save.mutate();
            }
          }}
        />
      )}
    </Modal>
  );
}
