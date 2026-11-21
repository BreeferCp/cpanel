import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useGet, useMutate } from "@/lib/hooks";
import { Button } from "@/components/ui/Button";
import { Field, Input } from "@/components/ui/Form";
import { Card, ErrorNote, Loading, PageHeader } from "@/components/ui/Layout";

type S = Record<string, string>;

export default function Settings() {
  const q = useGet<S>("/server/settings");
  const [f, setF] = useState<S>({});
  useEffect(() => {
    if (q.data) setF(q.data);
  }, [q.data]);

  const save = useMutate(() => api.put<S>("/server/settings", f), { invalidate: ["/server/settings"], success: "Settings saved" });
  const set = (k: string) => (e: React.ChangeEvent<HTMLInputElement>) => setF({ ...f, [k]: e.target.value });

  if (q.isLoading) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;

  return (
    <>
      <PageHeader title="Settings" description="Server identity and defaults used when creating accounts and DNS zones." />
      <form
        className="max-w-2xl space-y-5"
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <Card title="Server">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Server IP address" hint="Used for A records in new DNS zones.">
              {(id) => <Input id={id} value={f.server_ip ?? ""} onChange={set("server_ip")} className="font-mono" placeholder="203.0.113.10" />}
            </Field>
            <Field label="Hostname">{(id) => <Input id={id} value={f.hostname ?? ""} onChange={set("hostname")} placeholder="server.example.com" />}</Field>
            <Field label="Nameserver 1">{(id) => <Input id={id} value={f.nameserver1 ?? ""} onChange={set("nameserver1")} placeholder="ns1.example.com" />}</Field>
            <Field label="Nameserver 2">{(id) => <Input id={id} value={f.nameserver2 ?? ""} onChange={set("nameserver2")} placeholder="ns2.example.com" />}</Field>
          </div>
        </Card>
        <Card title="Contact">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Company name">{(id) => <Input id={id} value={f.company_name ?? ""} onChange={set("company_name")} />}</Field>
            <Field label="Administrator email">{(id) => <Input id={id} type="email" value={f.contact_email ?? ""} onChange={set("contact_email")} />}</Field>
            <Field label="Let's Encrypt account email" className="sm:col-span-2">
              {(id) => <Input id={id} type="email" value={f.acme_email ?? ""} onChange={set("acme_email")} />}
            </Field>
          </div>
        </Card>
        <Button type="submit" variant="primary" loading={save.isPending}>
          Save settings
        </Button>
      </form>
    </>
  );
}
