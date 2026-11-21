import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import type { Domain, PHPInfo } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button } from "@/components/ui/Button";
import { Input, Select } from "@/components/ui/Form";
import { Badge, Card, ErrorNote, Loading, Mono, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";

const labels: Record<string, string> = {
  memory_limit: "Memory limit per script",
  max_execution_time: "Max execution time (s)",
  max_input_time: "Max input time (s)",
  max_input_vars: "Max input variables",
  post_max_size: "Max POST size",
  upload_max_filesize: "Max upload size",
  display_errors: "Display errors",
  error_reporting: "Error reporting",
  "date.timezone": "Timezone",
  "session.gc_maxlifetime": "Session lifetime (s)",
  output_buffering: "Output buffering",
  short_open_tag: "Short open tags",
  "zlib.output_compression": "Zlib compression",
};

const choices: Record<string, string[]> = {
  display_errors: ["Off", "On"],
  short_open_tag: ["Off", "On"],
  "zlib.output_compression": ["Off", "On"],
  error_reporting: ["E_ALL & ~E_DEPRECATED & ~E_STRICT", "E_ALL", "E_ALL & ~E_NOTICE", "0"],
};

export default function Php() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/php`;
  const domKey = `/accounts/${aid}/domains`;
  const q = useGet<PHPInfo>(key);
  const domains = useGet<Domain[]>(domKey);
  const [ini, setIni] = useState<Record<string, string>>({});
  useEffect(() => {
    if (q.data) setIni({ ...q.data.defaults, ...(q.data.ini ?? {}) });
  }, [q.data]);

  const save = useMutate(() => api.put(key, { ini }), { invalidate: [key], success: "PHP settings applied" });
  const setVersion = useMutate((v: { id: string; php: string }) => api.patch(`${domKey}/${v.id}`, { php_version: v.php }), {
    invalidate: [domKey],
    success: "PHP version changed",
  });

  if (q.isLoading) return <Loading />;
  if (q.error || !q.data) return <ErrorNote error={q.error} />;
  const allowed = q.data.versions.filter((v) => v.allowed && v.installed).map((v) => v.version);
  const changed = Object.keys(ini).filter((k) => ini[k] !== (q.data!.ini?.[k] ?? q.data!.defaults[k]));

  return (
    <>
      <PageHeader title="PHP Manager" description="Choose the PHP version per domain and tune php.ini settings for your account." />
      <div className="grid gap-5 xl:grid-cols-[1fr_1.2fr]">
        <div className="space-y-5">
          <Card title="Version per domain" flush>
            <Table>
              <thead>
                <tr>
                  <Th>Domain</Th>
                  <Th className="w-28">PHP</Th>
                </tr>
              </thead>
              <tbody>
                {domains.data?.map((d) => (
                  <Tr key={d.id}>
                    <Td className="font-medium text-ink">{d.name}</Td>
                    <Td>
                      <Select aria-label={`PHP version for ${d.name}`} className="h-7" value={d.php_version} onChange={(e) => setVersion.mutate({ id: d.id, php: e.target.value })}>
                        {allowed.includes(d.php_version) ? null : <option>{d.php_version}</option>}
                        {allowed.map((v) => (
                          <option key={v}>{v}</option>
                        ))}
                      </Select>
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          </Card>
          <Card title="Versions on this server" flush>
            <Table>
              <tbody>
                {q.data.versions.map((v) => (
                  <Tr key={v.version}>
                    <Td className="font-medium">PHP {v.version}</Td>
                    <Td className="text-right">
                      {!v.installed ? <Badge>Not installed</Badge> : v.allowed ? <Badge tone="ok">Available</Badge> : <Badge>Not in your plan</Badge>}
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          </Card>
        </div>

        <Card
          title="php.ini settings"
          description="Applied to every PHP version your domains use. Security-sensitive settings stay under the administrator's control."
          actions={
            <Button size="sm" variant="primary" loading={save.isPending} disabled={changed.length === 0} onClick={() => save.mutate()}>
              Apply{changed.length ? ` (${changed.length})` : ""}
            </Button>
          }
          flush
        >
          <Table>
            <tbody>
              {q.data.keys.map((k) => (
                <Tr key={k}>
                  <Td>
                    <div className="text-ink">{labels[k] ?? k}</div>
                    <Mono className="text-subtle">{k}</Mono>
                  </Td>
                  <Td className="w-64">
                    {choices[k] ? (
                      <Select aria-label={k} value={ini[k] ?? ""} onChange={(e) => setIni({ ...ini, [k]: e.target.value })}>
                        {choices[k]!.map((c) => (
                          <option key={c}>{c}</option>
                        ))}
                      </Select>
                    ) : (
                      <Input aria-label={k} className="font-mono" value={ini[k] ?? ""} onChange={(e) => setIni({ ...ini, [k]: e.target.value })} />
                    )}
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        </Card>
      </div>
    </>
  );
}
