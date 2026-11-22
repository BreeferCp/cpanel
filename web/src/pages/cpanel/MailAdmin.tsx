import { CheckCircle2, CircleX, FileUp, HardDrive, Upload } from "lucide-react";
import { useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { formatBytes, formatMB, percent } from "@/lib/format";
import { useGet } from "@/lib/hooks";
import type { Domain } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button } from "@/components/ui/Button";
import { DataTable } from "@/components/ui/DataTable";
import { Field, Select, Textarea } from "@/components/ui/Form";
import { Card, ErrorNote, PageHeader, Stat } from "@/components/ui/Layout";

interface Usage {
  id: string;
  address: string;
  quota_mb: number;
  bytes: number;
  messages: number;
}

export function EmailDiskUsage() {
  const aid = useAccountId();
  const q = useGet<Usage[]>(`/accounts/${aid}/mail-usage`);
  const rows = q.data;
  const used = (u: Usage) => percent(u.bytes / 1048576, u.quota_mb);

  return (
    <>
      <PageHeader title="Email Disk Usage" />
      {rows && rows.length > 0 && (
        <div className="mb-4 grid gap-3 sm:grid-cols-3">
          <Stat label="Mail stored" value={formatBytes(rows.reduce((n, u) => n + u.bytes, 0))} />
          <Stat label="Messages" value={rows.reduce((n, u) => n + u.messages, 0).toLocaleString()} />
          <Stat label="Over 80% full" value={String(rows.filter((u) => used(u) >= 80).length)} />
        </div>
      )}
      <DataTable
        rows={rows}
        loading={q.isLoading}
        error={q.error}
        rowKey={(u) => u.id}
        search={(u) => u.address}
        searchPlaceholder="Search mailboxes"
        empty={{ icon: HardDrive, title: "No mailboxes", description: "Create an email account to see how much space its mail uses." }}
        initialSort={{ key: "bytes", dir: "desc" }}
        columns={[
          { key: "address", header: "Mailbox", sort: (u) => u.address, render: (u) => <span className="font-medium text-ink">{u.address}</span> },
          { key: "messages", header: "Messages", sort: (u) => u.messages, render: (u) => u.messages.toLocaleString(), hideSm: true },
          { key: "bytes", header: "Size", sort: (u) => u.bytes, render: (u) => formatBytes(u.bytes) },
          { key: "quota", header: "Of quota", sort: used, render: (u) => <span className="text-muted">{`${Math.round(used(u))}% of ${formatMB(u.quota_mb)}`}</span> },
        ]}
      />
    </>
  );
}

// parseCSV handles quoted fields, doubled quotes and line breaks inside quotes.
function parseCSV(text: string): string[][] {
  const out: string[][] = [];
  let row: string[] = [];
  let cell = "";
  let quoted = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (quoted) {
      if (c === '"' && text[i + 1] === '"') {
        cell += '"';
        i++;
      } else if (c === '"') quoted = false;
      else cell += c;
    } else if (c === '"' && cell === "") quoted = true;
    else if (c === ",") {
      row.push(cell);
      cell = "";
    } else if (c === "\n") {
      row.push(cell.replace(/\r$/, ""));
      out.push(row);
      row = [];
      cell = "";
    } else cell += c;
  }
  row.push(cell.replace(/\r$/, ""));
  out.push(row);
  return out.filter((r) => r.some((v) => v.trim()));
}

type Kind = "mailboxes" | "forwarders";
interface Entry {
  n: number;
  address: string;
  detail: string;
  problem?: string;
  path: string;
  body: object;
}

export function AddressImporter() {
  const aid = useAccountId();
  const qc = useQueryClient();
  const domains = useGet<Domain[]>(`/accounts/${aid}/domains`);
  const [kind, setKind] = useState<Kind>("mailboxes");
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState<Record<number, string>>({}); // "" means imported

  const entries = useMemo<Entry[]>(() => {
    let rows = parseCSV(text);
    // Only a first row without an address is a header.
    if (rows.length && !rows[0]?.[0]?.includes("@")) rows = rows.slice(1);
    return rows.map((r, i) => {
      const address = (r[0] ?? "").trim().toLowerCase();
      const [local, host] = address.split("@");
      const dom = domains.data?.find((d) => d.name === host && (d.kind === "primary" || d.kind === "addon"));
      const e: Entry = { n: i, address, detail: "", path: "", body: {} };
      if (!local || !dom) e.problem = "Not an address on one of your domains";
      if (kind === "mailboxes") {
        // Passwords are used exactly as written and never shown.
        const quota = Number(r[2]?.trim() || 1024);
        e.detail = `${quota} MB`;
        e.path = "mailboxes";
        e.body = { local_part: local, domain_id: dom?.id, password: r[1] ?? "", quota_mb: quota };
        if (!e.problem && !r[1]) e.problem = "Password is missing";
        if (!e.problem && !(Number.isInteger(quota) && quota >= 1 && quota <= 102400)) e.problem = "Storage must be 1 to 102400 MB";
      } else {
        e.detail = r[1]?.trim() ?? "";
        e.path = "mail-aliases";
        e.body = { local_part: local, domain_id: dom?.id, destination: e.detail };
        if (!e.problem && !e.detail.includes("@")) e.problem = "Destination is missing";
      }
      return e;
    });
  }, [text, kind, domains.data]);

  const ready = entries.filter((e) => !e.problem && done[e.n] !== "");

  const run = async () => {
    setBusy(true);
    for (const e of ready) {
      let result = "";
      try {
        await api.post(`/accounts/${aid}/${e.path}`, e.body);
      } catch (err) {
        result = err instanceof Error ? err.message : "Failed";
      }
      setDone((d) => ({ ...d, [e.n]: result }));
    }
    await qc.invalidateQueries();
    setBusy(false);
  };

  const edit = (value: string) => {
    setText(value);
    setDone({});
  };

  return (
    <>
      <PageHeader title="Address Importer" />
      <div className="space-y-5">
        <Card title="Your list">
          <div className="space-y-4">
            {domains.error ? <ErrorNote error={domains.error} /> : null}
            <Field label="Import">
              {(id) => (
                <Select
                  id={id}
                  value={kind}
                  disabled={busy}
                  onChange={(e) => {
                    setKind(e.target.value as Kind);
                    setDone({});
                  }}
                >
                  <option value="mailboxes">Email accounts</option>
                  <option value="forwarders">Forwarders</option>
                </Select>
              )}
            </Field>
            <Field label="CSV" hint={kind === "mailboxes" ? "One per line: address, password, storage in MB (optional)" : "One per line: address, forward to"}>
              {(id) => <Textarea id={id} rows={7} className="font-mono" value={text} disabled={busy} onChange={(e) => edit(e.target.value)} />}
            </Field>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <label className="inline-flex cursor-pointer items-center gap-1.5 text-[13px] text-muted hover:text-ink">
                <FileUp className="size-4" aria-hidden />
                Choose a CSV file
                <input
                  type="file"
                  accept=".csv,text/csv,text/plain"
                  className="sr-only"
                  disabled={busy}
                  onChange={async (e) => {
                    const file = e.target.files?.[0];
                    if (file) edit(await file.text());
                  }}
                />
              </label>
              <Button variant="primary" icon={Upload} loading={busy} disabled={!ready.length} onClick={run}>
                Import {ready.length}
              </Button>
            </div>
          </div>
        </Card>
        {entries.length > 0 && (
          <DataTable
            rows={entries}
            rowKey={(e) => String(e.n)}
            empty={{ icon: Upload, title: "Nothing to import" }}
            columns={[
              { key: "address", header: "Address", render: (e) => <span className="font-medium text-ink">{e.address}</span> },
              { key: "detail", header: kind === "mailboxes" ? "Storage" : "Forward to", render: (e) => <span className="text-muted">{e.detail}</span>, hideSm: true },
              {
                key: "result",
                header: "Result",
                render: (e) => {
                  const err = e.problem || done[e.n];
                  if (err)
                    return (
                      <span className="inline-flex items-center gap-1.5 text-ink">
                        <CircleX className="size-3.5" aria-hidden />
                        {err}
                      </span>
                    );
                  if (done[e.n] === "")
                    return (
                      <span className="inline-flex items-center gap-1.5 text-ink">
                        <CheckCircle2 className="size-3.5" aria-hidden />
                        Imported
                      </span>
                    );
                  return <span className="text-muted">Ready</span>;
                },
              },
            ]}
          />
        )}
      </div>
    </>
  );
}
