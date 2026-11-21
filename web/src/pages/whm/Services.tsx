import { Play, RotateCw, Square } from "lucide-react";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import type { ServiceState } from "@/lib/types";
import { IconButton } from "@/components/ui/Button";
import { Card, ErrorNote, Loading, Mono, PageHeader, StatusDot, Table, Td, Th, Tr } from "@/components/ui/Layout";

export default function Services() {
  const q = useGet<ServiceState[]>("/server/services");
  const act = useMutate((v: { name: string; action: string }) => api.post(`/server/services/${v.name}/${v.action}`), {
    invalidate: ["/server/services"],
    success: (_r, v) => `${v.name}: ${v.action} requested`,
  });

  return (
    <>
      <PageHeader title="Services" description="Status and control of the daemons that run your hosting stack." />
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
                <Th>Service</Th>
                <Th>Unit</Th>
                <Th>State</Th>
                <Th className="text-right">Actions</Th>
              </tr>
            </thead>
            <tbody>
              {q.data?.map((s) => (
                <Tr key={s.name}>
                  <Td className="font-medium text-ink">{s.label}</Td>
                  <Td>
                    <Mono className="text-muted">{s.name}</Mono>
                  </Td>
                  <Td>
                    <span className="inline-flex items-center gap-2">
                      <StatusDot ok={s.active} />
                      <span className="capitalize">{s.state}</span>
                    </span>
                  </Td>
                  <Td>
                    <div className="flex justify-end">
                      {s.active ? (
                        <IconButton icon={Square} label={`Stop ${s.name}`} disabled={s.name === "breeferd"} onClick={() => act.mutate({ name: s.name, action: "stop" })} />
                      ) : (
                        <IconButton icon={Play} label={`Start ${s.name}`} onClick={() => act.mutate({ name: s.name, action: "start" })} />
                      )}
                      <IconButton icon={RotateCw} label={`Restart ${s.name}`} onClick={() => act.mutate({ name: s.name, action: "restart" })} />
                    </div>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
    </>
  );
}
