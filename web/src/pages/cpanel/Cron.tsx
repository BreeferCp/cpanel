import { Clock, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import type { CronJob } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button, IconButton } from "@/components/ui/Button";
import { Field, Input, Select, Switch } from "@/components/ui/Form";
import { Card, EmptyState, ErrorNote, Loading, Mono, PageHeader, Table, Td, Th, Tr } from "@/components/ui/Layout";
import { ConfirmDialog, Modal } from "@/components/ui/Modal";

const PRESETS: [string, string][] = [
  ["Every minute", "* * * * *"],
  ["Every 5 minutes", "*/5 * * * *"],
  ["Every hour", "0 * * * *"],
  ["Every day at 02:00", "0 2 * * *"],
  ["Every week (Sunday 03:00)", "0 3 * * 0"],
  ["Every month (1st, 04:00)", "0 4 1 * *"],
];

function CronEmail() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/cron-email`;
  const q = useGet<{ email: string }>(key);
  const [edited, setEdited] = useState<string | null>(null);
  const save = useMutate(() => api.put(key, { email: edited }), { invalidate: [key], success: "Cron email saved", onSuccess: () => setEdited(null) });
  return (
    <Card title="Cron email" className="mb-5">
      {q.error ? (
        <ErrorNote error={q.error} />
      ) : (
        <form
          className="flex flex-wrap items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          <div className="min-w-0 flex-1">
            <Field label="Email job output to" hint="Leave empty to discard output.">
              {(id) => <Input id={id} type="email" value={edited ?? q.data?.email ?? ""} onChange={(e) => setEdited(e.target.value)} />}
            </Field>
          </div>
          <Button type="submit" loading={save.isPending} disabled={edited === null}>
            Save
          </Button>
        </form>
      )}
    </Card>
  );
}

export default function Cron() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/cron`;
  const q = useGet<CronJob[]>(key);
  const [adding, setAdding] = useState(false);
  const [f, setF] = useState({ schedule: "0 2 * * *", command: "" });
  const [del, setDel] = useState<CronJob | null>(null);

  const add = useMutate(() => api.post(key, f), {
    invalidate: [key],
    success: "Cron job added",
    onSuccess: () => {
      setAdding(false);
      setF({ schedule: "0 2 * * *", command: "" });
    },
  });
  const toggle = useMutate((v: { id: string; enabled: boolean }) => api.patch(`${key}/${v.id}`, { enabled: v.enabled }), { invalidate: [key] });
  const remove = useMutate((id: string) => api.del(`${key}/${id}`), { invalidate: [key], success: "Cron job deleted", onSuccess: () => setDel(null) });

  return (
    <>
      <PageHeader
        title="Cron Jobs"
        description="Run commands on a schedule as your account user."
        actions={
          <Button variant="primary" icon={Plus} onClick={() => setAdding(true)}>
            Add cron job
          </Button>
        }
      />
      <CronEmail />
      <Card flush>
        {q.isLoading ? (
          <Loading />
        ) : q.error ? (
          <div className="p-4">
            <ErrorNote error={q.error} />
          </div>
        ) : !q.data?.length ? (
          <EmptyState icon={Clock} title="No cron jobs" description="Schedule maintenance scripts, queue workers or backups." />
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Schedule</Th>
                <Th>Command</Th>
                <Th>Enabled</Th>
                <Th className="w-10">
                  <span className="sr-only">Actions</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {q.data.map((j) => (
                <Tr key={j.id}>
                  <Td>
                    <Mono className="text-ink">{j.schedule}</Mono>
                  </Td>
                  <Td className="max-w-lg">
                    <Mono className="block truncate text-muted">{j.command}</Mono>
                  </Td>
                  <Td>
                    <Switch label={`Enable job ${j.command}`} checked={j.enabled} onChange={(v) => toggle.mutate({ id: j.id, enabled: v })} />
                  </Td>
                  <Td>
                    <IconButton icon={Trash2} label="Delete cron job" onClick={() => setDel(j)} />
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>

      <Modal
        open={adding}
        onClose={() => setAdding(false)}
        title="Add cron job"
        footer={
          <>
            <Button onClick={() => setAdding(false)}>Cancel</Button>
            <Button variant="primary" loading={add.isPending} disabled={!f.command} onClick={() => add.mutate()}>
              Add job
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="Common schedules">
            {(id) => (
              <Select id={id} value={PRESETS.some((p) => p[1] === f.schedule) ? f.schedule : ""} onChange={(e) => e.target.value && setF({ ...f, schedule: e.target.value })}>
                <option value="">Custom</option>
                {PRESETS.map(([label, v]) => (
                  <option key={v} value={v}>
                    {label}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Schedule" hint="minute hour day-of-month month day-of-week">
            {(id) => <Input id={id} value={f.schedule} onChange={(e) => setF({ ...f, schedule: e.target.value })} className="font-mono" />}
          </Field>
          <Field label="Command">
            {(id) => (
              <Input id={id} value={f.command} onChange={(e) => setF({ ...f, command: e.target.value })} className="font-mono" placeholder="/usr/bin/php /home/you/public_html/cron.php" />
            )}
          </Field>
        </div>
      </Modal>

      <ConfirmDialog
        open={!!del}
        onClose={() => setDel(null)}
        onConfirm={() => del && remove.mutate(del.id)}
        loading={remove.isPending}
        title="Delete cron job?"
        message={del ? <Mono>{del.command}</Mono> : null}
      />
    </>
  );
}
