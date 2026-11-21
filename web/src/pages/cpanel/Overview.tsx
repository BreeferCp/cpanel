import { Link } from "react-router-dom";
import { ArrowUpRight } from "lucide-react";
import { useGet } from "@/lib/hooks";
import { formatMB } from "@/lib/format";
import type { Account, Usage } from "@/lib/types";
import { useAccountId, useBase } from "@/state/scope";
import { Badge, Card, ErrorNote, Loading, Meter, PageHeader } from "@/components/ui/Layout";

export default function HostingOverview() {
  const aid = useAccountId();
  const base = useBase();
  const usage = useGet<Usage>(`/accounts/${aid}/usage`);
  const acct = useGet<Account>(`/accounts/${aid}`);

  if (usage.isLoading || acct.isLoading) return <Loading />;
  if (usage.error || !usage.data || !acct.data) return <ErrorNote error={usage.error ?? acct.error} />;
  const u = usage.data;
  const a = acct.data;

  const meters: { label: string; m: { used: number; limit: number }; fmt?: (n: number) => string }[] = [
    { label: "Disk space", m: u.disk, fmt: formatMB },
    { label: "Bandwidth (month)", m: u.bandwidth, fmt: formatMB },
    { label: "Domains", m: u.domains },
    { label: "Databases", m: u.databases },
    { label: "Email accounts", m: u.mailboxes },
    { label: "Cron jobs", m: u.cron },
    { label: "Applications", m: u.apps },
  ];

  return (
    <>
      <PageHeader
        title={a.primary_domain}
        description={`Account ${a.sys_user} on the ${u.package} package.`}
        actions={<Badge tone={a.status === "active" ? "ok" : "bad"}>{a.status === "active" ? "Active" : "Suspended"}</Badge>}
      />
      {a.status === "suspended" && (
        <div className="mb-4 rounded-md bg-warn-soft px-3 py-2 text-[13px] text-warn">
          This account is suspended{a.suspend_reason ? `: ${a.suspend_reason}` : ""}. Websites and email are offline.
        </div>
      )}
      <div className="grid gap-4 lg:grid-cols-3">
        <Card title="Resource usage" className="lg:col-span-2">
          <div className="grid gap-x-8 gap-y-5 sm:grid-cols-2">
            {meters.map((x) => (
              <div key={x.label}>
                <div className="mb-1.5 text-[12.5px] font-medium text-muted">{x.label}</div>
                <Meter used={x.m.used} limit={x.m.limit} format={x.fmt} />
              </div>
            ))}
          </div>
        </Card>
        <Card title="Quick links">
          <ul className="-my-1 divide-y divide-line">
            {[
              ["File Manager", "files"],
              ["Add a domain", "domains"],
              ["Create an email account", "email"],
              ["Create a database", "databases"],
              ["Install an SSL certificate", "ssl"],
              ["Schedule a backup", "backups"],
            ].map(([label, to]) => (
              <li key={to}>
                <Link to={`${base}/${to}`} className="flex items-center justify-between py-2.5 text-[13px] text-ink hover:text-accent">
                  {label}
                  <ArrowUpRight className="size-3.5 text-subtle" aria-hidden />
                </Link>
              </li>
            ))}
          </ul>
        </Card>
      </div>
    </>
  );
}
