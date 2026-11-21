import clsx from "clsx";
import { ChevronDown, ChevronLeft, ExternalLink, LayoutDashboard, Menu, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { Link, NavLink, Outlet, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useGet } from "@/lib/hooks";
import { availableTools, toolGroups, whmNav, whmVisible } from "@/lib/catalog";
import type { Account } from "@/lib/types";
import { useAuth } from "@/state/auth";
import { usePanel } from "@/state/panel";
import { IconButton } from "../ui/Button";
import { PanelMark, UserChip } from "./common";

function navClass({ isActive }: { isActive: boolean }) {
  return clsx(
    "flex items-center gap-2.5 rounded-md px-2.5 py-1.5 text-[13px] transition-colors",
    isActive ? "bg-accent-soft font-medium text-accent" : "text-muted hover:bg-surface2 hover:text-ink",
  );
}

/** Sidebar for server administration: searchable, collapsible WHM categories. */
function WhmNav({ onNavigate }: { onNavigate: () => void }) {
  const { user } = useAuth();
  const [q, setQ] = useState("");
  const [closed, setClosed] = useState<Record<string, boolean>>({});
  const cats = useMemo(() => {
    const s = q.trim().toLowerCase();
    return whmNav
      .map((c) => ({
        ...c,
        items: c.items.filter(
          (it) =>
            whmVisible(it, user?.role ?? "", !!user?.is_master) &&
            (!s || it.label.toLowerCase().includes(s) || (it.keywords ?? "").includes(s) || c.title.toLowerCase().includes(s)),
        ),
      }))
      .filter((c) => c.items.length);
  }, [q, user]);
  return (
    <>
      <div className="px-3 pb-3">
        <label className="relative block">
          <span className="sr-only">Search WHM</span>
          <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-subtle" aria-hidden />
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Search tools"
            className="h-8 w-full rounded-md border border-line bg-surface2 pl-8 pr-2 text-[13px] text-ink placeholder:text-subtle focus:border-accent focus:outline-none focus:ring-2 focus:ring-[var(--focus)]"
          />
        </label>
      </div>
      <nav className="flex-1 overflow-y-auto pb-4" aria-label="WHM">
        {cats.map((c) => {
          const isClosed = closed[c.title] && !q;
          return (
            <div key={c.title} className="mb-2">
              <button
                type="button"
                onClick={() => setClosed({ ...closed, [c.title]: !closed[c.title] })}
                aria-expanded={!isClosed}
                className="flex w-full items-center justify-between px-4 py-1 text-[11px] font-semibold uppercase tracking-wider text-subtle hover:text-muted"
              >
                {c.title}
                <ChevronDown className={clsx("size-3.5 transition-transform", isClosed && "-rotate-90")} aria-hidden />
              </button>
              {!isClosed && (
                <ul className="space-y-px px-2">
                  {c.items.map((it) => (
                    <li key={it.to}>
                      <NavLink to={it.to} end={it.to === "/"} onClick={onNavigate} className={navClass}>
                        <it.icon className="size-4 shrink-0" aria-hidden />
                        {it.label}
                      </NavLink>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          );
        })}
        {!cats.length && <p className="px-4 text-[12.5px] text-muted">No tools match “{q}”.</p>}
      </nav>
    </>
  );
}

/** Sidebar while an administrator manages one customer account. */
function ManagedNav({ aid, onNavigate }: { aid: string; onNavigate: () => void }) {
  const acct = useQuery({ queryKey: ["account", aid], queryFn: () => api.get<Account>(`/accounts/${aid}`) });
  const base = `/accounts/${aid}`;
  const list = availableTools(null, "full").filter((t) => !t.to.startsWith("/") && t.to !== "");
  return (
    <>
      <div className="mx-3 mb-3 rounded-md border border-line bg-surface2 px-3 py-2">
        <Link to="/accounts" className="mb-1 flex items-center gap-1 text-[12px] text-muted hover:text-ink">
          <ChevronLeft className="size-3.5" aria-hidden /> All accounts
        </Link>
        <div className="truncate text-[13px] font-medium text-ink">{acct.data?.primary_domain ?? "Loading…"}</div>
        <div className="truncate text-[12px] text-muted">
          {acct.data?.sys_user} · {acct.data?.package_name}
        </div>
      </div>
      <nav className="flex-1 overflow-y-auto pb-4" aria-label="Account">
        <ul className="mb-3 px-2">
          <li>
            <NavLink to={base} end onClick={onNavigate} className={navClass}>
              <LayoutDashboard className="size-4" aria-hidden /> Overview
            </NavLink>
          </li>
        </ul>
        {toolGroups.map((g) => {
          const items = list.filter((t) => t.group === g);
          if (!items.length) return null;
          return (
            <div key={g} className="mb-3">
              <div className="px-4 pb-1 text-[11px] font-semibold uppercase tracking-wider text-subtle">{g}</div>
              <ul className="space-y-px px-2">
                {items.map((t) => (
                  <li key={t.label}>
                    <NavLink to={`${base}/${t.to}`} onClick={onNavigate} className={navClass}>
                      <t.icon className="size-4 shrink-0" aria-hidden />
                      {t.label}
                    </NavLink>
                  </li>
                ))}
              </ul>
            </div>
          );
        })}
      </nav>
    </>
  );
}

export function WhmShell() {
  const { aid } = useParams();
  const panel = usePanel();
  const { user } = useAuth();
  const [open, setOpen] = useState(false);
  const close = () => setOpen(false);
  const status = useGet<{ hostname: string; load: number[] }>("/server/status", user?.role === "root");
  const hostname = status.data?.hostname || panel.hostname;

  return (
    <div className="min-h-screen lg:grid lg:grid-cols-[248px_1fr]">
      {open && <div className="fixed inset-0 z-30 bg-black/40 lg:hidden" onClick={close} aria-hidden />}
      <aside
        className={clsx(
          "fixed inset-y-0 left-0 z-40 flex w-[248px] flex-col border-r border-line bg-surface transition-transform lg:sticky lg:top-0 lg:h-screen lg:translate-x-0",
          open ? "translate-x-0" : "-translate-x-full",
        )}
      >
        <Link to="/" className="flex items-center gap-2.5 px-4 py-4">
          <img src="/favicon.svg" alt="" className="size-6" />
          <span className="text-[15px] font-semibold tracking-tight text-ink">BreeferCP</span>
          <PanelMark label="WHM" />
        </Link>
        {aid ? <ManagedNav aid={aid} onNavigate={close} /> : <WhmNav onNavigate={close} />}
        <div className="border-t border-line p-3">
          <UserChip subtitle={user?.role === "reseller" ? (user.is_master ? "Master reseller" : "Reseller") : "Administrator"} />
        </div>
      </aside>

      <div className="min-w-0">
        <header className="sticky top-0 z-20 flex h-12 items-center gap-3 border-b border-line bg-surface/95 px-4 backdrop-blur sm:px-6">
          <span className="lg:hidden">
            <IconButton icon={Menu} label="Open navigation" onClick={() => setOpen(true)} />
          </span>
          <div className="min-w-0 truncate text-[13px] text-muted">
            <span className="font-medium text-ink">{hostname || "WHM"}</span>
            {status.data?.load && (
              <span className="ml-3 hidden tabular-nums sm:inline">load {status.data.load.map((l) => l.toFixed(2)).join(" ")}</span>
            )}
          </div>
          <div className="ml-auto flex items-center gap-2">
            {panel.cpanel_url && (
              <a
                href={panel.cpanel_url}
                className="hidden items-center gap-1 rounded-md px-2 py-1 text-[12.5px] text-muted hover:bg-surface2 hover:text-ink sm:inline-flex"
              >
                cPanel <ExternalLink className="size-3" aria-hidden />
              </a>
            )}
          </div>
        </header>
        <main className="mx-auto w-full max-w-[1240px] px-4 py-6 sm:px-8 sm:py-8">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
