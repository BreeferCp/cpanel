import { RefreshCw } from "lucide-react";
import { useState } from "react";
import { useGet } from "@/lib/hooks";
import type { Domain } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button } from "@/components/ui/Button";
import { Select } from "@/components/ui/Form";
import { Card, ErrorNote, Loading, PageHeader, Tabs } from "@/components/ui/Layout";

type Kind = "access" | "error" | "php";

export default function Logs() {
  const aid = useAccountId();
  const domains = useGet<Domain[]>(`/accounts/${aid}/domains`);
  const [kind, setKind] = useState<Kind>("access");
  const [domain, setDomain] = useState("");
  const [lines, setLines] = useState("200");
  const name = domain || domains.data?.[0]?.name || "";
  const path = `/accounts/${aid}/logs?kind=${kind}&lines=${lines}${kind === "php" ? "" : `&domain=${encodeURIComponent(name)}`}`;
  const q = useGet<{ lines: string[] }>(path, kind === "php" || !!name);

  return (
    <>
      <PageHeader title="Logs" description="The latest requests and errors for your sites, newest at the bottom." />
      <Tabs
        tabs={[
          { id: "access", label: "Access log" },
          { id: "error", label: "Error log" },
          { id: "php", label: "PHP errors" },
        ]}
        value={kind}
        onChange={setKind}
      />
      <div className="mb-3 flex flex-wrap items-center gap-2">
        {kind !== "php" && (
          <Select aria-label="Domain" className="w-64" value={name} onChange={(e) => setDomain(e.target.value)}>
            {domains.data?.map((d) => (
              <option key={d.id}>{d.name}</option>
            ))}
          </Select>
        )}
        <Select aria-label="Number of lines" className="w-32" value={lines} onChange={(e) => setLines(e.target.value)}>
          {["100", "200", "500", "1000"].map((n) => (
            <option key={n} value={n}>
              Last {n}
            </option>
          ))}
        </Select>
        <Button icon={RefreshCw} onClick={() => void q.refetch()} loading={q.isFetching}>
          Refresh
        </Button>
      </div>
      <Card flush>
        {q.isLoading ? (
          <Loading />
        ) : q.error ? (
          <div className="p-4">
            <ErrorNote error={q.error} />
          </div>
        ) : !q.data?.lines.length ? (
          <p className="px-4 py-10 text-center text-[13px] text-muted">This log is empty.</p>
        ) : (
          <pre className="max-h-[65vh] overflow-auto px-4 py-3 font-mono text-[12px] leading-5 text-ink">{q.data.lines.join("\n")}</pre>
        )}
      </Card>
    </>
  );
}
