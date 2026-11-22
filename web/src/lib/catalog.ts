import {
  Activity,
  AlarmClock,
  AppWindow,
  Archive,
  ArchiveRestore,
  ArrowRightLeft,
  Ban,
  BarChart3,
  BellRing,
  Boxes,
  Bug,
  Clock,
  Code2,
  ContactRound,
  Cpu,
  Database,
  DatabaseBackup,
  DatabaseZap,
  FileCode2,
  FileLock2,
  FileText,
  FileType2,
  FileWarning,
  Filter,
  FolderOpen,
  FolderTree,
  FolderUp,
  Forward,
  GitBranch,
  Globe,
  GlobeLock,
  HardDrive,
  ImageOff,
  Inbox,
  KeyRound,
  KeySquare,
  LayoutGrid,
  LifeBuoy,
  ListChecks,
  ListTree,
  Lock,
  LockKeyhole,
  Mail,
  MailCheck,
  MailPlus,
  MailSearch,
  MailWarning,
  Megaphone,
  Network,
  Package,
  PackageCheck,
  PackagePlus,
  Pause,
  Power,
  Radar,
  Repeat,
  Route,
  ScrollText,
  Server,
  ServerCog,
  Settings,
  Settings2,
  Shield,
  ShieldAlert,
  ShieldCheck,
  ShieldQuestion,
  Signpost,
  Smartphone,
  SquareTerminal,
  Terminal,
  ToggleRight,
  Trash2,
  UserCog,
  UserPlus,
  UserRoundPen,
  Users,
  UsersRound,
  Webhook,
  Wifi,
  Zap,
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
  /** One-line explanation shown under the page title. */
  blurb?: string;
  /** Hidden unless the account has a full shell. */
  shell?: boolean;
}

export const toolGroups = ["Email", "Files", "Databases", "Domains", "Metrics", "Security", "Software", "Advanced", "Preferences"] as const;

export const tools: Tool[] = [
  { group: "Email", to: "email", label: "Email Accounts", icon: Mail, feature: "email", keywords: "mailbox quota password connect devices", blurb: "Create mailboxes, change passwords and quotas, and set up mail apps." },
  { group: "Email", to: "forwarders", label: "Forwarders", icon: Forward, feature: "forwarders", keywords: "alias forward domain", blurb: "Send copies of mail for one address, or a whole domain, somewhere else." },
  { group: "Email", to: "email-routing", label: "Email Routing", icon: Route, feature: "email_routing", keywords: "mx local remote google microsoft 365", blurb: "Choose whether this server or an outside mail service receives a domain's mail." },
  { group: "Email", to: "autoresponders", label: "Autoresponders", icon: BellRing, feature: "autoresponders", keywords: "vacation out of office", blurb: "Reply automatically while you are away." },
  { group: "Email", to: "default-address", label: "Default Address", icon: Inbox, feature: "default_address", keywords: "catch all catchall unrouted", blurb: "Decide what happens to mail sent to an address that does not exist." },
  { group: "Email", to: "mailing-lists", label: "Mailing Lists", icon: UsersRound, feature: "mailing_lists", keywords: "distribution group list", blurb: "One address that delivers to a group of people." },
  { group: "Email", to: "track-delivery", label: "Track Delivery", icon: MailSearch, feature: "track_delivery", keywords: "bounce deferred log delivery report", blurb: "See whether a message was delivered, delayed or bounced, and why." },
  { group: "Email", to: "email-filters", label: "Email Filters", icon: Filter, feature: "email_filters", keywords: "rules sieve folder discard redirect", blurb: "Sort, redirect or delete incoming mail with rules." },
  { group: "Email", to: "deliverability", label: "Email Deliverability", icon: MailCheck, feature: "deliverability", keywords: "spf dkim dmarc ptr reverse dns", blurb: "Check and repair the DNS records that keep your mail out of spam folders." },
  { group: "Email", to: "spam", label: "Spam Filters", icon: ShieldQuestion, feature: "spam_filter", keywords: "rspamd junk blacklist whitelist", blurb: "Score incoming mail and move or delete spam." },
  { group: "Email", to: "address-importer", label: "Address Importer", icon: FolderUp, feature: "email", keywords: "csv bulk import mailboxes forwarders", blurb: "Create many email accounts or forwarders at once from a CSV list." },
  { group: "Email", to: "email-disk-usage", label: "Email Disk Usage", icon: HardDrive, feature: "email", keywords: "mailbox size space messages", blurb: "See which mailboxes take up the most space." },
  { group: "Email", to: "webmail", label: "Webmail", icon: MailPlus, feature: "email", keywords: "roundcube read mail", blurb: "Read and send mail in your browser." },

  { group: "Files", to: "files", label: "File Manager", icon: FolderOpen, feature: "file_manager", keywords: "upload edit zip", blurb: "Upload, edit and organise the files of your sites." },
  { group: "Files", to: "directory-privacy", label: "Directory Privacy", icon: FileLock2, feature: "directory_privacy", keywords: "password protect htpasswd", blurb: "Require a user name and password to open a folder." },
  { group: "Files", to: "disk", label: "Disk Usage", icon: HardDrive, feature: "disk_usage", keywords: "space quota", blurb: "Find out which folders and databases use the most space." },
  { group: "Files", to: "ftp", label: "FTP Accounts", icon: FolderUp, feature: "ftp", keywords: "ftps upload", blurb: "Logins for uploading files with an FTP program." },
  { group: "Files", to: "backups", label: "Backup", icon: Archive, feature: "backups", keywords: "restore download", blurb: "Back up your account and restore it." },
  { group: "Files", to: "git", label: "Git Version Control", icon: GitBranch, feature: "git", keywords: "repository deploy clone pull", blurb: "Deploy a site from a Git repository." },

  { group: "Databases", to: "databases", label: "MySQL Databases", icon: Database, feature: "databases", keywords: "mariadb user grant", blurb: "Create databases and users and decide who can use which." },
  { group: "Databases", to: "database-wizard", label: "Database Wizard", icon: DatabaseZap, feature: "databases", keywords: "step new database user", blurb: "Create a database, a user and its privileges in three steps." },
  { group: "Databases", to: "phpmyadmin", label: "phpMyAdmin", icon: DatabaseBackup, feature: "phpmyadmin", keywords: "sql browse import export", blurb: "Browse, query, import and export your databases." },
  { group: "Databases", to: "remote-mysql", label: "Remote MySQL", icon: Network, feature: "remote_mysql", keywords: "external host access", blurb: "Allow other computers to connect to your databases." },

  { group: "Domains", to: "domains", label: "Domains", icon: Globe, feature: "domains", keywords: "addon parked document root", blurb: "Add websites and choose where their files live." },
  { group: "Domains", to: "subdomains", label: "Subdomains", icon: ListTree, feature: "domains", keywords: "sub blog shop", blurb: "Create names such as blog.example.com with their own folder." },
  { group: "Domains", to: "aliases", label: "Aliases", icon: ArrowRightLeft, feature: "domains", keywords: "parked alias same site", blurb: "Show the same site on another domain name." },
  { group: "Domains", to: "redirects", label: "Redirects", icon: Repeat, feature: "redirects", keywords: "301 302", blurb: "Send visitors from one address to another." },
  { group: "Domains", to: "dns", label: "Zone Editor", icon: Signpost, feature: "dns", keywords: "dns records mx txt dnssec", blurb: "Edit the DNS records of your domains." },
  { group: "Domains", to: "dynamic-dns", label: "Dynamic DNS", icon: Wifi, feature: "dynamic_dns", keywords: "ddns home router changing ip", blurb: "Keep a host name pointed at a connection whose IP address changes." },

  { group: "Metrics", to: "visitors", label: "Visitors", icon: Users, feature: "visitors", keywords: "hits pages referrers traffic", blurb: "The latest requests to your sites, with top pages and referrers." },
  { group: "Metrics", to: "errors", label: "Errors", icon: Bug, feature: "logs", keywords: "error log php warnings", blurb: "The latest web server and PHP errors." },
  { group: "Metrics", to: "bandwidth", label: "Bandwidth", icon: BarChart3, feature: "bandwidth", keywords: "traffic transfer", blurb: "How much data each domain served, day by day." },
  { group: "Metrics", to: "raw-access", label: "Raw Access", icon: FileText, feature: "logs", keywords: "download access log", blurb: "Download the raw access logs of your domains." },
  { group: "Metrics", to: "resources", label: "Resource Usage", icon: Activity, keywords: "cpu memory processes io", blurb: "CPU, memory and process use of your account against its limits." },

  { group: "Security", to: "ssl", label: "SSL/TLS", icon: ShieldCheck, feature: "ssl", keywords: "certificate https lets encrypt", blurb: "Free and custom certificates for HTTPS." },
  { group: "Security", to: "ip-blocker", label: "IP Blocker", icon: Ban, feature: "ip_blocker", keywords: "deny address", blurb: "Refuse visitors from an address or range." },
  { group: "Security", to: "hotlink", label: "Hotlink Protection", icon: ImageOff, feature: "hotlink", keywords: "referrer images", blurb: "Stop other sites from embedding your images and media." },
  { group: "Security", to: "ssh", label: "SSH Access", icon: KeyRound, feature: "ssh", keywords: "keys sftp", blurb: "Manage the keys that can log in over SSH and SFTP." },
  { group: "Security", to: "/two-factor", label: "Two-Factor Authentication", icon: Smartphone, keywords: "2fa totp authenticator", blurb: "Protect your login with a code from your phone." },
  { group: "Security", to: "/api-tokens", label: "Manage API Tokens", icon: KeySquare, keywords: "token automation api", blurb: "Tokens that let scripts use the panel API." },

  { group: "Software", to: "installer", label: "App Installer", icon: Package, feature: "installer", keywords: "softaculous one click", blurb: "Install WordPress and other applications in one step." },
  { group: "Software", to: "wordpress", label: "WordPress Toolkit", icon: AppWindow, feature: "wordpress", keywords: "staging clone update", blurb: "Update, secure, back up and clone your WordPress sites." },
  { group: "Software", to: "php", label: "MultiPHP Manager", icon: Code2, feature: "php", keywords: "version php 8", blurb: "Choose the PHP version of each domain." },
  { group: "Software", to: "php-ini", label: "MultiPHP INI Editor", icon: FileCode2, feature: "php", keywords: "memory_limit upload_max_filesize ini", blurb: "Change PHP settings such as the memory limit and upload size." },
  { group: "Software", to: "apps", label: "Application Manager", icon: Boxes, feature: "apps", keywords: "node python ruby docker", blurb: "Run Node.js, Python and Docker applications." },
  { group: "Software", to: "optimize", label: "Optimize Website", icon: Zap, feature: "optimize", keywords: "gzip compression speed", blurb: "Compress pages so they load faster." },

  { group: "Advanced", to: "terminal", label: "Terminal", icon: SquareTerminal, feature: "terminal", shell: true, keywords: "shell bash", blurb: "A command line on the server, in your browser." },
  { group: "Advanced", to: "cron", label: "Cron Jobs", icon: Clock, feature: "cron", keywords: "schedule", blurb: "Run commands on a schedule." },
  { group: "Advanced", to: "track-dns", label: "Track DNS", icon: Radar, feature: "track_dns", keywords: "lookup dig traceroute", blurb: "Look up DNS records and trace the route to a host." },
  { group: "Advanced", to: "indexes", label: "Indexes", icon: FolderTree, feature: "indexes", keywords: "directory listing autoindex", blurb: "List a folder's files when it has no index page." },
  { group: "Advanced", to: "error-pages", label: "Error Pages", icon: FileWarning, feature: "error_pages", keywords: "404 500", blurb: "Your own pages for errors such as 404 Not Found." },
  { group: "Advanced", to: "mime-types", label: "MIME Types", icon: FileType2, feature: "mime_types", keywords: "content type extension", blurb: "Tell browsers how to handle file extensions." },

  { group: "Preferences", to: "/profile", label: "Password & Security", icon: Lock, keywords: "password sessions", blurb: "Change your password and sign out other sessions." },
  { group: "Preferences", to: "/contact", label: "Contact Information", icon: ContactRound, keywords: "email notifications alerts", blurb: "Where we send notices, and which ones you want." },
  { group: "Preferences", to: "/tickets", label: "Support", icon: LifeBuoy, keywords: "ticket help", blurb: "Ask your hosting provider for help." },
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
  blurb?: string;
}

export interface WhmCategory {
  title: string;
  items: WhmItem[];
}

export const whmNav: WhmCategory[] = [
  {
    title: "Server Configuration",
    items: [
      { to: "/", label: "Home", icon: LayoutGrid, who: "all" },
      { to: "/settings", label: "Basic WebHost Manager Setup", icon: Settings, who: "root", keywords: "hostname nameservers ip contact", blurb: "Host name, shared IP address, nameservers and contact address." },
      { to: "/server-time", label: "Server Time", icon: AlarmClock, who: "root", keywords: "timezone ntp clock", blurb: "The server's time zone and clock synchronisation." },
      { to: "/root-password", label: "Change Root Password", icon: LockKeyhole, who: "root", keywords: "root password", blurb: "Set the operating system password of the root user." },
      { to: "/updates", label: "Update Preferences", icon: PackageCheck, who: "root", keywords: "upgrade version system update apt", blurb: "Install BreeferCP releases and operating system updates." },
    ],
  },
  {
    title: "Server Status",
    items: [
      { to: "/server-info", label: "Server Information", icon: Cpu, who: "root", keywords: "os kernel cpu disks", blurb: "Hardware, operating system, disks and memory." },
      { to: "/services", label: "Service Status", icon: ServerCog, who: "root", keywords: "restart status", blurb: "Start, stop and restart the server's services." },
      { to: "/processes", label: "Process Manager", icon: Activity, who: "root", keywords: "kill top", blurb: "Running processes, and ending the ones that misbehave." },
      { to: "/quota-report", label: "Disk and Bandwidth Usage", icon: HardDrive, who: "all", keywords: "over quota bandwidth", blurb: "Accounts closest to their disk and bandwidth limits." },
    ],
  },
  {
    title: "Account Information",
    items: [
      { to: "/accounts", label: "List Accounts", icon: Users, who: "all", keywords: "modify suspend terminate", blurb: "Every hosting account, with search and bulk actions." },
      { to: "/suspended", label: "List Suspended Accounts", icon: Pause, who: "all", keywords: "suspended", blurb: "Accounts that are currently suspended, and why." },
      { to: "/domains", label: "List Domains", icon: Globe, who: "all", keywords: "all domains subdomain addon", blurb: "Every domain on the server and the account it belongs to." },
    ],
  },
  {
    title: "Account Functions",
    items: [
      { to: "/accounts/new", label: "Create a New Account", icon: UserPlus, who: "all", keywords: "add create", blurb: "Set up a hosting account with a domain and a package." },
      { to: "/modify-account", label: "Modify an Account", icon: UserRoundPen, who: "all", keywords: "edit email shell", blurb: "Change an account's contact address, package and shell access." },
      { to: "/suspension", label: "Manage Account Suspension", icon: Pause, who: "all", keywords: "suspend unsuspend", blurb: "Suspend or reactivate accounts, one or many at a time." },
      { to: "/change-package", label: "Upgrade/Downgrade an Account", icon: ArrowRightLeft, who: "all", keywords: "package plan change", blurb: "Move accounts to another package." },
      { to: "/password", label: "Password Modification", icon: KeyRound, who: "all", keywords: "reset password", blurb: "Set a new cPanel password for an account." },
      { to: "/terminate", label: "Terminate Accounts", icon: Trash2, who: "all", keywords: "delete remove", blurb: "Permanently delete accounts and all their data." },
      { to: "/broadcast", label: "Email All Users", icon: Megaphone, who: "all", keywords: "announcement notice", blurb: "Send a message to your customers or show a notice in cPanel." },
      { to: "/resellers", label: "Resellers", icon: UsersRound, who: "master", keywords: "master reseller", blurb: "Accounts that can sell hosting of their own." },
    ],
  },
  {
    title: "Packages",
    items: [
      { to: "/packages", label: "Packages", icon: Package, who: "all", keywords: "plans limits edit delete", blurb: "The plans you sell, with their limits." },
      { to: "/packages/new", label: "Add a Package", icon: PackagePlus, who: "all", keywords: "create plan", blurb: "Create a plan with disk, bandwidth and feature limits." },
      { to: "/features", label: "Feature Manager", icon: ToggleRight, who: "all", keywords: "feature list", blurb: "Choose which cPanel tools each package includes." },
    ],
  },
  {
    title: "DNS Functions",
    items: [{ to: "/dns-zones", label: "DNS Zones", icon: Signpost, who: "all", keywords: "zone editor dnssec", blurb: "Every DNS zone on the server, with its records." }],
  },
  {
    title: "Email",
    items: [{ to: "/mail-queue", label: "Mail Queue Manager", icon: MailWarning, who: "root", keywords: "postfix deferred flush", blurb: "Messages waiting to be delivered: retry, hold or delete them." }],
  },
  {
    title: "Software",
    items: [{ to: "/multiphp", label: "MultiPHP Manager", icon: Code2, who: "all", keywords: "php version bulk", blurb: "Set the PHP version of many domains at once." }],
  },
  {
    title: "SSL/TLS",
    items: [{ to: "/autossl", label: "Manage AutoSSL", icon: GlobeLock, who: "root", keywords: "lets encrypt certificates expiry", blurb: "Certificate status of every domain, and running AutoSSL." }],
  },
  {
    title: "Security Center",
    items: [
      { to: "/security-advisor", label: "Security Advisor", icon: ShieldAlert, who: "root", keywords: "audit check recommendations", blurb: "Checks the server's configuration and explains what to fix." },
      { to: "/security", label: "Security Policy", icon: Shield, who: "root", keywords: "2fa two factor allow list", blurb: "Mandatory two-factor authentication and the WHM IP allow list." },
      { to: "/host-access", label: "Host Access Control", icon: ListChecks, who: "root", keywords: "brute force cphulk block", blurb: "Always allow or always block addresses from signing in." },
      { to: "/ssh", label: "SSH Settings", icon: Terminal, who: "root", keywords: "password authentication root login port", blurb: "Root login, password authentication and the SSH port." },
      { to: "/root-keys", label: "Manage root's SSH Keys", icon: KeyRound, who: "root", keywords: "authorized keys", blurb: "Public keys that can log in as root." },
      { to: "/firewall", label: "Firewall", icon: Webhook, who: "root", keywords: "nftables ports", blurb: "Open and close ports and block addresses." },
      { to: "/two-factor", label: "Two-Factor Authentication", icon: Smartphone, who: "all", keywords: "2fa totp", blurb: "Protect your own login with an authenticator app." },
      { to: "/audit", label: "Audit Log", icon: ScrollText, who: "root", keywords: "history", blurb: "Who did what, and when." },
    ],
  },
  {
    title: "Backup",
    items: [
      { to: "/backup-config", label: "Backup Configuration", icon: Server, who: "root", keywords: "schedule retention", blurb: "Schedule and retention of automatic backups." },
      { to: "/backup-restore", label: "Backup Restoration", icon: ArchiveRestore, who: "root", keywords: "restore account", blurb: "Restore an account from any of its backups." },
    ],
  },
  {
    title: "System Reboot",
    items: [{ to: "/reboot", label: "Reboot Server", icon: Power, who: "root", keywords: "restart reboot", blurb: "Restart the server gracefully or forcefully." }],
  },
  {
    title: "Development",
    items: [{ to: "/api-tokens", label: "Manage API Tokens", icon: KeySquare, who: "all", keywords: "token billing whmcs", blurb: "Tokens for WHMCS, Paymenter and your own scripts." }],
  },
  {
    title: "Support",
    items: [
      { to: "/tickets", label: "Support Tickets", icon: LifeBuoy, who: "all", blurb: "Questions from your customers." },
      { to: "/profile", label: "My Account", icon: UserCog, who: "all", keywords: "password sessions", blurb: "Your password and signed-in sessions." },
      { to: "/contact", label: "Contact Information", icon: Settings2, who: "all", keywords: "email notifications", blurb: "Your contact address and the notices you receive." },
    ],
  },
];

export function whmVisible(it: WhmItem, role: string, master: boolean): boolean {
  return it.who === "all" || (it.who === "master" ? role === "root" || master : role === "root");
}
