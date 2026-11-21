import { useState } from "react";
import { formatDate } from "@/lib/format";
import { useGet } from "@/lib/hooks";
import type { AuditEntry } from "@/lib/types";
import { Button } from "@/components/ui/Button";
import { Badge, Card, ErrorNote, Loading, Mono, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";

const PAGE = 50;

export default function Audit() {
  const [page, setPage] = useState(0);
  const q = useGet<AuditEntry[]>(`/server/audit?limit=${PAGE}&offset=${page * PAGE}`);

  return (
    <>
      <PageHeader title="Audit log" description="Every sign-in, change and administrative action, kept for one year." />
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
                <Th>Time</Th>
                <Th>Actor</Th>
                <Th>Action</Th>
                <Th>Target</Th>
                <Th>IP</Th>
                <Th>Result</Th>
              </tr>
            </thead>
            <tbody>
              {q.data?.map((e) => (
                <Tr key={e.id}>
                  <Td className="whitespace-nowrap text-muted">{formatDate(e.at)}</Td>
                  <Td className="font-medium text-ink">{e.actor || "—"}</Td>
                  <Td>
                    <Mono>{e.action}</Mono>
                  </Td>
                  <Td className="max-w-xs">
                    <span className="block truncate text-muted" title={`${e.target} ${e.detail}`}>
                      {e.target}
                      {e.detail && <span className="text-subtle"> · {e.detail}</span>}
                    </span>
                  </Td>
                  <Td>
                    <Mono className="text-muted">{e.ip}</Mono>
                  </Td>
                  <Td>
                    <Badge tone={e.ok ? "ok" : "bad"}>{e.ok ? "OK" : "Failed"}</Badge>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
        <div className="flex items-center justify-between border-t border-line px-4 py-2.5">
          <span className="text-[12.5px] text-muted">Page {page + 1}</span>
          <div className="flex gap-2">
            <Button size="sm" disabled={page === 0} onClick={() => setPage(page - 1)}>
              Newer
            </Button>
            <Button size="sm" disabled={(q.data?.length ?? 0) < PAGE} onClick={() => setPage(page + 1)}>
              Older
            </Button>
          </div>
        </div>
      </Card>
    </>
  );
}
