import {
  Activity,
  AppWindow,
  Archive,
  AtSign,
  Ban,
  BellRing,
  Boxes,
  Clock,
  Code2,
  Cpu,
  Database,
  DatabaseBackup,
  FileLock2,
  FileWarning,
  Filter,
  FolderOpen,
  FolderUp,
  GitBranch,
  Globe,
  HardDrive,
  ImageOff,
  Inbox,
  KeyRound,
  LayoutGrid,
  LifeBuoy,
  ListChecks,
  Lock,
  Mail,
  MailWarning,
  Megaphone,
  Network,
  Package,
  PackageCheck,
  Repeat,
  ScrollText,
  Server,
  ServerCog,
  Settings,
  Shield,
  ShieldAlert,
  ShieldCheck,
  SquareTerminal,
  ToggleRight,
  UserCog,
  Users,
  UsersRound,
  Webhook,
  type LucideIcon,
} from "lucide-react";

/** One cPanel tool. `feature` ties it to a WHM feature-list key. */
export interface Tool {
  to: string;
  label: string;
  icon: LucideIcon;
  group: string;
  feature?: string;
  keywords?: string;
  /** Hidden unless the account has a full shell. */
  shell?: boolean;
}

export const toolGroups = ["Files", "Databases", "Domains", "Email", "Software", "Security", "Advanced", "Metrics", "Preferences"] as const;

export const tools: Tool[] = [
  { group: "Files", to: "files", label: "File Manager", icon: FolderOpen, feature: "file_manager", keywords: "upload edit zip" },
  { group: "Files", to: "disk", label: "Disk Usage", icon: HardDrive, feature: "disk_usage", keywords: "space quota" },
  { group: "Files", to: "ftp", label: "FTP Accounts", icon: FolderUp, feature: "ftp", keywords: "ftps upload" },
  { group: "Files", to: "backups", label: "Backups", icon: Archive, feature: "backups", keywords: "restore download" },
  { group: "Files", to: "git", label: "Git Version Control", icon: GitBranch, feature: "git", keywords: "repository deploy clone pull" },

  { group: "Databases", to: "databases", label: "MySQL Databases", icon: Database, feature: "databases", keywords: "mariadb user grant" },
  { group: "Databases", to: "databases?pma=1", label: "phpMyAdmin", icon: DatabaseBackup, feature: "phpmyadmin", keywords: "sql" },
  { group: "Databases", to: "remote-mysql", label: "Remote MySQL", icon: Network, feature: "remote_mysql", keywords: "external host access" },

  { group: "Domains", to: "domains", label: "Domains", icon: Globe, feature: "domains", keywords: "addon subdomain alias parked" },
  { group: "Domains", to: "redirects", label: "Redirects", icon: Repeat, feature: "redirects", keywords: "301 302" },
  { group: "Domains", to: "dns", label: "Zone Editor", icon: Network, feature: "dns", keywords: "dns records mx txt dnssec" },

  { group: "Email", to: "email", label: "Email Accounts", icon: Mail, feature: "email", keywords: "mailbox forwarders dkim" },
  { group: "Email", to: "email?tab=webmail", label: "Webmail", icon: Inbox, feature: "email", keywords: "roundcube" },
  { group: "Email", to: "autoresponders", label: "Autoresponders", icon: BellRing, feature: "autoresponders", keywords: "vacation out of office" },
  { group: "Email", to: "spam", label: "Spam Filters", icon: Filter, feature: "spam_filter", keywords: "rspamd junk blacklist whitelist" },
  { group: "Email", to: "email?tab=deliverability", label: "Email Deliverability", icon: AtSign, feature: "email", keywords: "spf dkim dmarc" },

  { group: "Software", to: "installer", label: "App Installer", icon: Package, feature: "installer", keywords: "softaculous one click" },
  { group: "Software", to: "wordpress", label: "WordPress Toolkit", icon: AppWindow, feature: "wordpress", keywords: "staging clone update" },
  { group: "Software", to: "php", label: "PHP Manager", icon: Code2, feature: "php", keywords: "version ini multiphp" },
  { group: "Software", to: "apps", label: "Node.js, Python, Ruby", icon: Boxes, feature: "apps", keywords: "application docker" },

  { group: "Security", to: "ssl", label: "SSL/TLS", icon: ShieldCheck, feature: "ssl", keywords: "certificate https lets encrypt" },
  { group: "Security", to: "directory-privacy", label: "Directory Privacy", icon: FileLock2, feature: "directory_privacy", keywords: "password protect htpasswd" },
  { group: "Security", to: "ip-blocker", label: "IP Blocker", icon: Ban, feature: "ip_blocker", keywords: "deny address" },
  { group: "Security", to: "hotlink", label: "Hotlink Protection", icon: ImageOff, feature: "hotlink", keywords: "referrer images" },
  { group: "Security", to: "ssh", label: "SSH Access", icon: KeyRound, feature: "ssh", keywords: "keys" },

  { group: "Advanced", to: "terminal", label: "Terminal", icon: SquareTerminal, feature: "terminal", shell: true, keywords: "shell bash" },
  { group: "Advanced", to: "cron", label: "Cron Jobs", icon: Clock, feature: "cron", keywords: "schedule" },
  { group: "Advanced", to: "error-pages", label: "Error Pages", icon: FileWarning, feature: "error_pages", keywords: "404 500" },

  { group: "Metrics", to: "logs", label: "Logs", icon: ScrollText, feature: "logs", keywords: "access error raw" },
  { group: "Metrics", to: "", label: "Resource Usage", icon: Activity, keywords: "cpu memory statistics" },

  { group: "Preferences", to: "/profile", label: "Password & Security", icon: Lock, keywords: "2fa two factor sessions api tokens" },
  { group: "Preferences", to: "/tickets", label: "Support", icon: LifeBuoy, keywords: "ticket help" },
];

/** Tools available to an account (feature list and shell mode applied). */
export function availableTools(disabled: string[] | null | undefined, shell: string | undefined): Tool[] {
  const off = new Set(disabled ?? []);
  return tools.filter((t) => (!t.feature || !off.has(t.feature)) && (!t.shell || shell === "full"));
}

export function matchTool(t: Tool, q: string): boolean {
  const s = q.trim().toLowerCase();
  return !s || t.label.toLowerCase().includes(s) || (t.keywords ?? "").includes(s) || t.group.toLowerCase().includes(s);
}

// ---- WHM navigation ---------------------------------------------------------

export interface WhmItem {
  to: string;
  label: string;
  icon: LucideIcon;
  /** root: root only; master: root or master resellers; all: every admin. */
  who: "root" | "master" | "all";
  keywords?: string;
}

export interface WhmCategory {
  title: string;
  items: WhmItem[];
}

export const whmNav: WhmCategory[] = [
  {
    title: "Server",
    items: [
      { to: "/", label: "Home", icon: LayoutGrid, who: "all" },
      { to: "/server-info", label: "Server Information", icon: Cpu, who: "root", keywords: "os kernel cpu disks" },
      { to: "/services", label: "Service Manager", icon: ServerCog, who: "root", keywords: "restart status" },
      { to: "/processes", label: "Process Manager", icon: Activity, who: "root", keywords: "kill top" },
      { to: "/updates", label: "Updates", icon: PackageCheck, who: "root", keywords: "upgrade version system update apt" },
      { to: "/settings", label: "Tweak Settings", icon: Settings, who: "root", keywords: "hostname nameservers ip" },
    ],
  },
  {
    title: "Accounts",
    items: [
      { to: "/accounts", label: "List Accounts", icon: Users, who: "all", keywords: "create modify suspend terminate" },
      { to: "/quota-report", label: "Disk Usage Report", icon: HardDrive, who: "all", keywords: "over quota bandwidth" },
      { to: "/resellers", label: "Resellers", icon: UsersRound, who: "master", keywords: "master reseller" },
      { to: "/broadcast", label: "Email All Users", icon: Megaphone, who: "all", keywords: "announcement notice" },
    ],
  },
  {
    title: "Packages",
    items: [
      { to: "/packages", label: "Packages", icon: Package, who: "all", keywords: "plans limits" },
      { to: "/features", label: "Feature Manager", icon: ToggleRight, who: "all", keywords: "feature list" },
    ],
  },
  {
    title: "DNS & Email",
    items: [
      { to: "/dns-zones", label: "DNS Zones", icon: Globe, who: "all", keywords: "zone editor dnssec" },
      { to: "/mail-queue", label: "Mail Queue Manager", icon: MailWarning, who: "root", keywords: "postfix deferred flush" },
    ],
  },
  {
    title: "Security Center",
    items: [
      { to: "/security", label: "Security Policy", icon: ShieldAlert, who: "root", keywords: "2fa two factor allow list" },
      { to: "/host-access", label: "Host Access Control", icon: Shield, who: "root", keywords: "brute force cphulk block" },
      { to: "/firewall", label: "Firewall", icon: Webhook, who: "root", keywords: "nftables ports" },
      { to: "/audit", label: "Audit Log", icon: ListChecks, who: "root", keywords: "history" },
    ],
  },
  {
    title: "Backup",
    items: [{ to: "/backup-config", label: "Backup Configuration", icon: Server, who: "root", keywords: "schedule retention" }],
  },
  {
    title: "Support",
    items: [
      { to: "/tickets", label: "Support Tickets", icon: LifeBuoy, who: "all" },
      { to: "/profile", label: "My Account", icon: UserCog, who: "all", keywords: "password 2fa api token" },
    ],
  },
];

export function whmVisible(it: WhmItem, role: string, master: boolean): boolean {
  return it.who === "all" || (it.who === "master" ? role === "root" || master : role === "root");
}

