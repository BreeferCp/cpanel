import clsx from "clsx";
import { Home, Info, LifeBuoy, Mail, Menu, Search, UserCog, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link, NavLink, Outlet, useNavigate } from "react-router-dom";
import { useGet } from "@/lib/hooks";
import { availableTools, matchTool } from "@/lib/catalog";
import type { Account } from "@/lib/types";
import { usePanel } from "@/state/panel";
import { IconButton } from "../ui/Button";
import { PanelMark, UserChip } from "./common";

const rail = [
  { to: "/hosting", label: "Home", icon: Home, end: true },
  { to: "/hosting/email", label: "Email", icon: Mail },
  { to: "/tickets", label: "Support", icon: LifeBuoy },
  { to: "/profile", label: "Account", icon: UserCog },
];

/** Global tool search in the top bar (like cPanel's "Search Tools"). */
function ToolSearch({ account }: { account?: Account }) {
  const [q, setQ] = useState("");
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const nav = useNavigate();
  const box = useRef<HTMLDivElement>(null);
  const results = useMemo(
    () => (q.trim() ? availableTools(account?.disabled_features, account?.shell).filter((t) => matchTool(t, q)).slice(0, 8) : []),
    [q, account],
  );
  useEffect(() => {
    const onDoc = (e: MouseEvent) => {
      if (box.current && !box.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, []);
  const go = (to: string) => {
    nav(to.startsWith("/") ? to : `/hosting${to ? "/" + to : ""}`);
    setQ("");
    setOpen(false);
  };
  return (
    <div ref={box} className="relative w-full max-w-md">
      <label className="relative block">
        <span className="sr-only">Search tools</span>
        <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-subtle" aria-hidden />
        <input
          value={q}
          role="combobox"
          aria-expanded={open && results.length > 0}
          aria-controls="tool-results"
          onChange={(e) => {
            setQ(e.target.value);
            setOpen(true);
            setActive(0);
          }}
          onFocus={() => setOpen(true)}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") setActive((a) => Math.min(a + 1, results.length - 1));
            if (e.key === "ArrowUp") setActive((a) => Math.max(a - 1, 0));
            if (e.key === "Enter" && results[active]) go(results[active].to);
            if (e.key === "Escape") setOpen(false);
          }}
          placeholder="Search tools (/ to focus)"
          className="h-8 w-full rounded-md border border-line bg-surface2 pl-8 pr-2 text-[13px] text-ink placeholder:text-subtle focus:border-accent focus:outline-none focus:ring-2 focus:ring-[var(--focus)]"
        />
      </label>
      {open && results.length > 0 && (
        <ul id="tool-results" role="listbox" className="absolute left-0 right-0 top-9 z-30 overflow-hidden rounded-md border border-line bg-surface py-1 shadow-pop">
          {results.map((t, i) => (
            <li key={t.label} role="option" aria-selected={i === active}>
              <button
                type="button"
                onMouseEnter={() => setActive(i)}
                onClick={() => go(t.to)}
                className={clsx("flex w-full items-center gap-2.5 px-3 py-1.5 text-left text-[13px]", i === active ? "bg-surface2 text-ink" : "text-muted")}
              >
                <t.icon className="size-4 text-accent" aria-hidden />
                <span className="flex-1">{t.label}</span>
                <span className="text-[11.5px] text-subtle">{t.group}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export function CpanelShell() {
  const panel = usePanel();
  const acct = useGet<Account>("/accounts/me");
  const [open, setOpen] = useState(false);
  const [hideNotice, setHideNotice] = useState(false);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (e.key === "/" && t.tagName !== "INPUT" && t.tagName !== "TEXTAREA") {
        e.preventDefault();
        document.querySelector<HTMLInputElement>('input[role="combobox"]')?.focus();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  const close = () => setOpen(false);

  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-30 flex h-14 items-center gap-3 border-b border-line bg-surface/95 px-4 backdrop-blur sm:px-6">
        <span className="md:hidden">
          <IconButton icon={Menu} label="Open navigation" onClick={() => setOpen(true)} />
        </span>
        <Link to="/hosting" className="flex shrink-0 items-center gap-2">
          <img src="/favicon.svg" alt="" className="size-6" />
          <span className="hidden text-[15px] font-semibold tracking-tight text-ink sm:inline">BreeferCP</span>
          <PanelMark label="cPanel" />
        </Link>
        <div className="mx-auto flex min-w-0 flex-1 justify-center px-2">
          <ToolSearch account={acct.data} />
        </div>
        <div className="hidden w-56 shrink-0 lg:block">
          <UserChip subtitle={acct.data?.primary_domain} />
        </div>
      </header>

      {open && <div className="fixed inset-0 z-40 bg-black/40 md:hidden" onClick={close} aria-hidden />}
      <div className="md:grid md:grid-cols-[200px_1fr]">
        <aside
          className={clsx(
            "fixed inset-y-0 left-0 z-50 flex w-[220px] flex-col border-r border-line bg-surface transition-transform md:sticky md:top-14 md:z-10 md:h-[calc(100vh-3.5rem)] md:w-auto md:translate-x-0",
            open ? "translate-x-0" : "-translate-x-full",
          )}
        >
          <div className="flex items-center justify-between px-3 py-3 md:hidden">
            <span className="text-[14px] font-semibold">Menu</span>
            <IconButton icon={X} label="Close navigation" onClick={close} />
          </div>
          <nav className="flex-1 space-y-px px-2 py-3" aria-label="cPanel">
            {rail.map((r) => (
              <NavLink
                key={r.to}
                to={r.to}
                end={r.end}
                onClick={close}
                className={({ isActive }) =>
                  clsx(
                    "flex items-center gap-2.5 rounded-md px-2.5 py-2 text-[13px] transition-colors",
                    isActive ? "bg-accent-soft font-medium text-accent" : "text-muted hover:bg-surface2 hover:text-ink",
                  )
                }
              >
                <r.icon className="size-4" aria-hidden />
                {r.label}
              </NavLink>
            ))}
            {panel.webmail_url && (
              <a href={panel.webmail_url} target="_blank" rel="noreferrer noopener" className="flex items-center gap-2.5 rounded-md px-2.5 py-2 text-[13px] text-muted hover:bg-surface2 hover:text-ink">
                <Mail className="size-4" aria-hidden /> Webmail
              </a>
            )}
          </nav>
          <div className="border-t border-line p-3 lg:hidden">
            <UserChip subtitle={acct.data?.primary_domain} />
          </div>
        </aside>
        <main className="mx-auto w-full min-w-0 max-w-[1240px] px-4 py-6 sm:px-8 sm:py-8">
          {panel.announcement && !hideNotice && (
            <div className="mb-5 flex items-start gap-2.5 rounded-lg border border-line bg-accent-soft px-4 py-3 text-[13px] text-ink" role="status">
              <Info className="mt-0.5 size-4 shrink-0 text-accent" aria-hidden />
              <span className="flex-1">{panel.announcement}</span>
              <IconButton icon={X} label="Dismiss notice" onClick={() => setHideNotice(true)} />
            </div>
          )}
          <Outlet />
        </main>
      </div>
    </div>
  );
}
