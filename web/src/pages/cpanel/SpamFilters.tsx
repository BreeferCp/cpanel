import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import type { SpamPolicy } from "@/lib/types";
import { useAccountId } from "@/state/scope";
import { Button } from "@/components/ui/Button";
import { Field, Select, Switch, Textarea } from "@/components/ui/Form";
import { Card, ErrorNote, Loading, PageHeader } from "@/components/ui/Layout";

const lines = (s: string) => s.split(/[\s,]+/).map((x) => x.trim()).filter(Boolean);

export default function SpamFilters() {
  const aid = useAccountId();
  const key = `/accounts/${aid}/spam`;
  const q = useGet<SpamPolicy>(key);
  const [f, setF] = useState({ enabled: false, threshold: 5, action: "junk" as SpamPolicy["action"], allow: "", deny: "" });
  useEffect(() => {
    if (q.data) setF({ ...q.data, allow: q.data.allow.join("\n"), deny: q.data.deny.join("\n") });
  }, [q.data]);
  const save = useMutate(() => api.put(key, { ...f, allow: lines(f.allow), deny: lines(f.deny) }), { invalidate: [key], success: "Spam filter saved" });

  if (q.isLoading) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  return (
    <>
      <PageHeader
        title="Spam Filters"
        description="Every incoming message gets a spam score. Choose how strict to be and what happens to mail above that score. Applies to all mailboxes in this account."
      />
      <div className="grid gap-4 lg:grid-cols-2">
        <Card title="Filtering">
          <div className="space-y-5">
            <div className="flex items-center justify-between">
              <div>
                <div className="text-[13px] font-medium">Filter spam</div>
                <div className="text-[12.5px] text-muted">Off: messages are delivered with their score in a header only.</div>
              </div>
              <Switch checked={f.enabled} onChange={(v) => setF({ ...f, enabled: v })} label="Filter spam" />
            </div>
            <Field label={`Score threshold: ${f.threshold}`} hint="Lower catches more spam but risks real mail. 5 suits most accounts.">
              {(id) => (
                <input
                  id={id}
                  type="range"
                  min={1}
                  max={15}
                  value={f.threshold}
                  onChange={(e) => setF({ ...f, threshold: Number(e.target.value) })}
                  className="w-full accent-[var(--accent)]"
                  disabled={!f.enabled}
                />
              )}
            </Field>
            <div className="flex justify-between text-[11.5px] text-subtle">
              <span>Aggressive</span>
              <span>Relaxed</span>
            </div>
            <Field label="Mail above the threshold">
              {(id) => (
                <Select id={id} value={f.action} disabled={!f.enabled} onChange={(e) => setF({ ...f, action: e.target.value as SpamPolicy["action"] })}>
                  <option value="junk">Move to the Junk folder</option>
                  <option value="discard">Delete immediately</option>
                </Select>
              )}
            </Field>
          </div>
        </Card>
        <Card title="Sender lists" description="One address or @domain per line.">
          <div className="space-y-4">
            <Field label="Always allow" hint="Never treated as spam, e.g. your bank or @yourcompany.com">
              {(id) => <Textarea id={id} value={f.allow} onChange={(e) => setF({ ...f, allow: e.target.value })} placeholder="billing@example.com" />}
            </Field>
            <Field label="Always block" hint="Deleted on arrival.">
              {(id) => <Textarea id={id} value={f.deny} onChange={(e) => setF({ ...f, deny: e.target.value })} placeholder="@spammy.example" />}
            </Field>
          </div>
        </Card>
      </div>
      <div className="mt-4 flex justify-end">
        <Button variant="primary" loading={save.isPending} onClick={() => save.mutate()}>
          Save changes
        </Button>
      </div>
    </>
  );
}
