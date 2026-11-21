import clsx from "clsx";
import {
  Activity,
  AppWindow,
  Archive,
  Boxes,
  Code2,
  FileWarning,
  HardDrive,
  LifeBuoy,
  Repeat,
  SquareTerminal,
  ChevronLeft,
  Clock,
  Database,
  FolderOpen,
  Globe,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Mail,
  Menu,
  Monitor,
  Moon,
  Network,
  Package,
  ScrollText,
  Server,
  Settings,
  Shield,
  ShieldCheck,
  Sun,
  Terminal,
  Users,
  UsersRound,
  type LucideIcon,
} from "lucide-react";
import { useState } from "react";
import { Link, NavLink, Outlet, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { Account } from "@/lib/types";
import { useAuth } from "@/state/auth";
import { useTheme, type ThemeChoice } from "@/state/theme";
import { IconButton } from "../ui/Button";

interface NavItem {
  to: string;
  label: string;
  icon: LucideIcon;
  end?: boolean;
}
interface NavGroup {
  title?: string;
  items: NavItem[];
}

const hostingNav = (base: string): NavGroup[] => [
  {
    items: [{ to: base, label: "Overview", icon: LayoutDashboard, end: true }],
  },
  {
    title: "Files",
    items: [
      { to: `${base}/files`, label: "File Manager", icon: FolderOpen },
      { to: `${base}/disk`, label: "Disk Usage", icon: HardDrive },
      { to: `${base}/backups`, label: "Backups", icon: Archive },
    ],
  },
  {
    title: "Domains",
    items: [
      { to: `${base}/domains`, label: "Domains", icon: Globe },
      { to: `${base}/redirects`, label: "Redirects", icon: Repeat },
      { to: `${base}/dns`, label: "DNS Zone Editor", icon: Network },
      { to: `${base}/ssl`, label: "SSL/TLS", icon: ShieldCheck },
      { to: `${base}/error-pages`, label: "Error Pages", icon: FileWarning },
    ],
  },
  {
    title: "Email & Databases",
    items: [
      { to: `${base}/email`, label: "Email", icon: Mail },
      { to: `${base}/databases`, label: "Databases", icon: Database },
    ],
  },
  {
    title: "Software",
    items: [
      { to: `${base}/installer`, label: "App Installer", icon: Package },
      { to: `${base}/wordpress`, label: "WordPress", icon: AppWindow },
      { to: `${base}/php`, label: "PHP Manager", icon: Code2 },
      { to: `${base}/apps`, label: "Node / Python / Ruby", icon: Boxes },
    ],
  },
  {
    title: "Advanced",
    items: [
      { to: `${base}/terminal`, label: "Terminal", icon: SquareTerminal },
      { to: `${base}/ssh`, label: "SSH Access", icon: Terminal },
      { to: `${base}/cron`, label: "Cron Jobs", icon: Clock },
      { to: `${base}/logs`, label: "Logs", icon: ScrollText },
    ],
  },
];

const adminNav = (isRoot: boolean, isMaster: boolean): NavGroup[] => [
  {
    title: "Server",
    items: [
      { to: "/", label: "Overview", icon: Activity, end: true },
      ...(isRoot ? [{ to: "/services", label: "Services", icon: Server }] : []),
      ...(isRoot ? [{ to: "/firewall", label: "Firewall", icon: Shield }] : []),
    ],
  },
  {
    title: "Customers",
    items: [
      { to: "/accounts", label: "Accounts", icon: Users },
      { to: "/packages", label: "Packages", icon: Package },
      ...(isRoot || isMaster ? [{ to: "/resellers", label: "Resellers", icon: UsersRound }] : []),
      { to: "/tickets", label: "Support Tickets", icon: LifeBuoy },
    ],
  },
  ...(isRoot
    ? [
        {
          title: "System",
          items: [
            { to: "/audit", label: "Audit log", icon: ScrollText },
            { to: "/settings", label: "Settings", icon: Settings },
          ],
        },
      ]
    : []),
];

function Brand() {
  return (
    <Link to="/" className="flex items-center gap-2.5 px-4 py-4">
      <img src="/favicon.svg" alt="" className="size-6" />
      <span className="text-[15px] font-semibold tracking-tight text-ink">BreeferCP</span>
    </Link>
  );
}

function NavSection({ group, onNavigate }: { group: NavGroup; onNavigate: () => void }) {
  return (
    <div className="mb-4">
      {group.title && <div className="px-4 pb-1 text-[11px] font-semibold uppercase tracking-wider text-subtle">{group.title}</div>}
      <ul className="space-y-px px-2">
        {group.items.map((it) => (
          <li key={it.to}>
            <NavLink
              to={it.to}
              end={it.end}
              onClick={onNavigate}
              className={({ isActive }) =>
                clsx(
                  "flex items-center gap-2.5 rounded-md px-2.5 py-1.5 text-[13px] transition-colors",
                  isActive ? "bg-accent-soft font-medium text-accent" : "text-muted hover:bg-surface2 hover:text-ink",
                )
              }
            >
              <it.icon className="size-4 shrink-0" aria-hidden />
              {it.label}
            </NavLink>
          </li>
        ))}
      </ul>
    </div>
  );
}

function ThemeToggle() {
  const { choice, setChoice } = useTheme();
  const next: Record<ThemeChoice, ThemeChoice> = { light: "dark", dark: "system", system: "light" };
  const Icon = choice === "light" ? Sun : choice === "dark" ? Moon : Monitor;
  return <IconButton icon={Icon} label={`Theme: ${choice} (click to change)`} onClick={() => setChoice(next[choice])} />;
}

/** Layout used for both the admin console and hosting pages. */
export function Shell() {
  const { user, logout } = useAuth();
  const { aid } = useParams();
  const [open, setOpen] = useState(false);
  const isAdmin = user?.role === "root" || user?.role === "reseller";

  // When an administrator is managing a customer, show that account's context.
  const managed = useQuery({
    queryKey: ["account", aid],
    queryFn: () => api.get<Account>(`/accounts/${aid}`),
    enabled: !!aid && isAdmin,
  });

  const groups: NavGroup[] = aid
    ? hostingNav(`/accounts/${aid}`)
    : isAdmin
      ? adminNav(user?.role === "root", !!user?.is_master)
      : hostingNav("/hosting");
  const close = () => setOpen(false);

  return (
    <div className="min-h-screen lg:grid lg:grid-cols-[232px_1fr]">
      {open && <div className="fixed inset-0 z-30 bg-black/40 lg:hidden" onClick={close} aria-hidden />}
      <aside
        className={clsx(
          "fixed inset-y-0 left-0 z-40 flex w-[232px] flex-col border-r border-line bg-surface transition-transform lg:sticky lg:top-0 lg:h-screen lg:translate-x-0",
          open ? "translate-x-0" : "-translate-x-full",
        )}
      >
        <Brand />
        {aid && (
          <div className="mx-3 mb-3 rounded-md border border-line bg-surface2 px-3 py-2">
            <Link to="/accounts" className="mb-1 flex items-center gap-1 text-[12px] text-muted hover:text-ink">
              <ChevronLeft className="size-3.5" aria-hidden /> All accounts
            </Link>
            <div className="truncate text-[13px] font-medium text-ink">{managed.data?.primary_domain ?? "Loading…"}</div>
            <div className="truncate text-[12px] text-muted">{managed.data?.sys_user}</div>
          </div>
        )}
        <nav className="flex-1 overflow-y-auto pb-4" aria-label="Main">
          {groups.map((g, i) => (
            <NavSection key={i} group={g} onNavigate={close} />
          ))}
          {!aid && (
            <NavSection
              group={{
                title: "Account",
                items: [
                  ...(isAdmin ? [] : [{ to: "/tickets", label: "Support", icon: LifeBuoy }]),
                  { to: "/profile", label: "Profile & Security", icon: KeyRound },
                ],
              }}
              onNavigate={close}
            />
          )}
        </nav>
        <div className="border-t border-line p-3">
          <div className="flex items-center gap-2.5">
            <div className="flex size-7 items-center justify-center rounded-full bg-accent-soft text-[12px] font-semibold uppercase text-accent">
              {user?.username.slice(0, 1)}
            </div>
            <div className="min-w-0 flex-1">
              <div className="truncate text-[13px] font-medium text-ink">{user?.username}</div>
              <div className="text-[11.5px] capitalize text-muted">{user?.role}</div>
            </div>
            <ThemeToggle />
            <IconButton icon={LogOut} label="Sign out" onClick={() => void logout()} />
          </div>
        </div>
      </aside>

      <div className="min-w-0">
        <div className="sticky top-0 z-20 flex h-12 items-center gap-2 border-b border-line bg-surface px-4 lg:hidden">
          <IconButton icon={Menu} label="Open navigation" onClick={() => setOpen(true)} />
          <span className="text-[14px] font-semibold">BreeferCP</span>
        </div>
        <main className="mx-auto w-full max-w-[1180px] px-4 py-6 sm:px-8 sm:py-8">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
