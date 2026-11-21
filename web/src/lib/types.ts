export type Role = "root" | "reseller" | "user";

export interface User {
  id: string;
  username: string;
  email: string;
  role: Role;
  totp_enabled: boolean;
  locale: string;
  theme: "light" | "dark" | "system";
  account_id?: string;
  is_master: boolean;
}

export interface Pkg {
  id: string;
  name: string;
  disk_mb: number;
  bandwidth_mb: number;
  max_domains: number;
  max_databases: number;
  max_mailboxes: number;
  max_cron: number;
  max_apps: number;
  cpu_percent: number;
  memory_mb: number;
  io_weight: number;
  max_procs: number;
  php_versions: string;
  shell_access: boolean;
  max_ftp: number;
  feature_list_id: string;
}

export interface Account {
  id: string;
  user_id: string;
  username: string;
  email: string;
  package_id: string;
  package_name: string;
  sys_user: string;
  home_dir: string;
  primary_domain: string;
  status: "active" | "suspended" | "terminating";
  suspend_reason: string;
  external_ref: string;
  disk_used_mb: number;
  bw_used_mb: number;
  owner_id: string;
  owner_name: string;
  shell: "none" | "sftp" | "full";
  disabled_features: string[] | null;
  created_at: number;
}

export interface Domain {
  id: string;
  name: string;
  kind: "primary" | "addon" | "subdomain" | "alias";
  doc_root: string;
  php_version: string;
  force_https: boolean;
  has_ssl: boolean;
  created_at: number;
}

export interface Cert {
  id: string;
  domain_id: string;
  domain: string;
  issuer: string;
  names: string;
  not_after: number;
  auto_renew: boolean;
}

export interface DnsRecord {
  name: string;
  type: string;
  ttl: number;
  content: string;
  priority?: number;
}

export interface Database {
  id: string;
  name: string;
  size_mb: number;
  users: string[];
}
export interface DbUser {
  id: string;
  name: string;
}

export interface Mailbox {
  id: string;
  address: string;
  quota_mb: number;
  used_mb: number;
  active: boolean;
}
export interface MailAlias {
  id: string;
  source: string;
  destination: string;
}

export interface CronJob {
  id: string;
  schedule: string;
  command: string;
  enabled: boolean;
}

export interface SshKey {
  id: string;
  name: string;
  fingerprint: string;
  created_at: number;
}

export interface App {
  id: string;
  name: string;
  runtime: "node" | "python" | "ruby" | "docker";
  version?: string;
  image?: string;
  port: number;
  status: string;
}

export interface Backup {
  id: string;
  kind: string;
  size_bytes: number;
  status: string;
  created_at: number;
}

export interface FileEntry {
  name: string;
  path: string;
  dir: boolean;
  symlink: boolean;
  size: number;
  mode: string;
  mod_time: string;
}

export interface Meter {
  used: number;
  limit: number;
}
export interface Usage {
  disk: Meter;
  bandwidth: Meter;
  domains: Meter;
  databases: Meter;
  mailboxes: Meter;
  cron: Meter;
  apps: Meter;
  package: string;
  status: string;
}

export interface ServerStatus {
  hostname: string;
  version: string;
  os: string;
  uptime_s: number;
  cpus: number;
  load: number[];
  memory: { total_mb: number; used_mb: number };
  disk: { total_mb: number; used_mb: number };
  accounts: { total: number; suspended: number };
  database: string;
  dry_run: boolean;
}

export interface ServiceState {
  name: string;
  label: string;
  active: boolean;
  state: string;
}

export interface FirewallRule {
  id: string;
  action: "allow" | "deny";
  protocol: "tcp" | "udp" | "any";
  port: string;
  source: string;
  comment: string;
}

export interface AuditEntry {
  id: string;
  at: number;
  actor: string;
  action: string;
  target: string;
  ip: string;
  detail: string;
  ok: boolean;
}

export interface Reseller {
  id: string;
  username: string;
  email: string;
  suspended: boolean;
  customers: number;
}

export interface ApiToken {
  id: string;
  name: string;
  scopes: string;
  allowed_ips: string;
  last_used: number;
  expires_at: number;
  created_at: number;
}

export interface SessionInfo {
  id: string;
  ip: string;
  user_agent: string;
  created_at: number;
  last_seen: number;
  current: boolean;
}

export interface HostSample {
  t: number;
  cpu: number;
  mem: number;
  load: number;
  disk: number;
  rx: number;
  tx: number;
}

export interface AccountSample {
  t: number;
  cpu: number;
  mem_mb: number;
  procs: number;
}

export interface TopAccount {
  id: string;
  domain: string;
  user: string;
  cpu: number;
  mem_mb: number;
  disk_used_mb: number;
  disk_limit_mb: number;
}

export interface ResellerFull {
  id: string;
  username: string;
  email: string;
  owner_id: string;
  owner_name: string;
  is_master: boolean;
  suspended: boolean;
  max_accounts: number;
  max_disk_mb: number;
  max_resellers: number;
  accounts: number;
  resellers: number;
  disk_allocated_mb: number;
  disk_used_mb: number;
  created_at: number;
}

export interface Ticket {
  id: string;
  number: number;
  subject: string;
  department: string;
  priority: "low" | "normal" | "high" | "urgent";
  status: "open" | "answered" | "customer_reply" | "closed";
  user_id: string;
  customer: string;
  mine: boolean;
  created_at: number;
  updated_at: number;
}

export interface TicketMessage {
  id: string;
  author: string;
  staff: boolean;
  body: string;
  created_at: number;
}

export interface CatalogApp {
  id: string;
  name: string;
  category: string;
  description: string;
  website: string;
  needs_db: boolean;
  method: "wpcli" | "archive" | "composer";
  webroot?: string;
}

export interface Installation {
  id: string;
  app: string;
  app_name: string;
  domain_id: string;
  domain: string;
  path: string;
  url: string;
  version: string;
  db_name: string;
  admin_user: string;
  status: "installing" | "installed" | "failed";
  created_at: number;
}

export interface WPItem {
  name: string;
  title?: string;
  status: string;
  update: string;
  version: string;
}

export interface WPInfo {
  version: string;
  update_version?: string;
  plugins: WPItem[];
  themes: WPItem[];
  maintenance: boolean;
}

export interface PHPInfo {
  versions: { version: string; installed: boolean; allowed: boolean }[];
  default_version: string;
  ini: Record<string, string> | null;
  defaults: Record<string, string>;
  keys: string[];
}

export interface RedirectRule {
  id: string;
  source: string;
  target: string;
  code: number;
}

export interface ErrorPage {
  code: number;
  custom: boolean;
  content: string;
}

export interface DiskUsage {
  directories: { path: string; size_mb: number }[];
  databases_mb: number;
  limit_mb: number;
  used_mb: number;
}

// ---- dedicated panels, feature lists and new tools -------------------------

export interface PanelInfo {
  panel: "whm" | "cpanel" | "both";
  whm_url: string;
  cpanel_url: string;
  webmail_url: string;
  hostname: string;
  announcement?: string;
}

export interface FeatureDef {
  key: string;
  label: string;
  group: string;
}

export interface FeatureList {
  id: string;
  name: string;
  disabled: string[];
  packages: number;
}

export interface FtpAccount {
  id: string;
  login: string;
  directory: string;
  quota_mb: number;
  created_at: number;
}

export interface Autoresponder {
  mailbox_id: string;
  address: string;
  subject: string;
  body: string;
  interval_days: number;
  starts_on: string;
  ends_on: string;
}

export interface SpamPolicy {
  enabled: boolean;
  threshold: number;
  action: "junk" | "discard";
  allow: string[];
  deny: string[];
}

export interface ProtectedDir {
  id: string;
  domain_id: string;
  domain: string;
  path: string;
  realm: string;
  users: string[];
}

export interface IPBlock {
  id: string;
  cidr: string;
  created_at: number;
}

export interface HotlinkPolicy {
  enabled: boolean;
  allowed: string[];
  extensions: string[];
  allow_empty: boolean;
}

export interface RemoteHost {
  id: string;
  host: string;
  note: string;
  created_at: number;
}

export interface GitRepo {
  id: string;
  name: string;
  path: string;
  clone_url: string;
  created_at: number;
  ssh_url: string;
  info: { branch: string; head: string; commit: string; when: string };
}

export interface DNSSECState {
  enabled: boolean;
  keys: { id: number; keytype: string; active: boolean; algorithm: string; ds: string[] | null }[];
}

export interface Proc {
  pid: number;
  user: string;
  state: string;
  cpu_seconds: number;
  mem_mb: number;
  command: string;
}

export interface QueuedMail {
  id: string;
  queue: string;
  arrival: number;
  size: number;
  sender: string;
  recipients: string[] | null;
  reason: string;
}

export interface LoginRule {
  id: string;
  cidr: string;
  kind: "allow" | "deny";
  note: string;
  created_at: number;
}

export interface LoginFailure {
  ip: string;
  failures: number;
  last: number;
  username: string;
  blocked: boolean;
}

export interface BackupConfig {
  enabled: boolean;
  frequency: "daily" | "weekly";
  weekday: number;
  hour: number;
  retention: number;
  kind: "full" | "files" | "databases";
  last_run: number;
}

export interface ServerInfo {
  os?: string;
  kernel?: string;
  cpu?: string;
  cores?: number;
  uptime_s?: number;
  cgroups?: string;
  panel_version: string;
  web_server: string;
  database: string;
  memory: { total_mb: number; used_mb: number };
  load: number[];
  disks: { mount: string; fs: string; total_mb: number; used_mb: number }[];
  demo?: boolean;
}

export interface UpdateStatus {
  current: string;
  tier: "manual" | "automatic";
  running: boolean;
  error: string;
  check_error: string;
  available: boolean;
  can_rollback: boolean;
  checked_at?: number;
  latest?: { version: string; name: string; notes: string; published_at: string; url: string };
}

export interface QuotaRow {
  id: string;
  user: string;
  domain: string;
  status: string;
  used_mb: number;
  limit_mb: number;
  bw_used_mb: number;
  percent: number;
  over: boolean;
}

export interface ZoneRow {
  zone: string;
  account_id: string;
  user: string;
  dnssec: boolean;
}

export interface WPSite {
  site: { site_url: string; indexing: boolean; debug: boolean; admins: string[] | null; db_name: string; table_prefix: string };
  policy: { auto_core: "off" | "minor" | "major"; auto_plugins: boolean; auto_themes: boolean; staging_of: string; last_update_at: number; last_update_result: string };
}

export interface WPCheck {
  id: string;
  label: string;
  ok: boolean;
  fixable: boolean;
  detail?: string;
}

export interface InstallBackup {
  id: string;
  reason: string;
  size_bytes: number;
  status: string;
  created_at: number;
}
