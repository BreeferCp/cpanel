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
  runtime: "node" | "python" | "docker";
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
