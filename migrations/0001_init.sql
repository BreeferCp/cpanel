-- BreeferCP schema v1. Portable across SQLite and PostgreSQL:
-- TEXT ids, INTEGER unix timestamps, no vendor-specific types.

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    email         TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('root','reseller','user')),
    owner_id      TEXT REFERENCES users(id) ON DELETE RESTRICT, -- reseller or root that owns this user
    totp_secret   TEXT NOT NULL DEFAULT '',
    totp_enabled  INTEGER NOT NULL DEFAULT 0,
    recovery_codes TEXT NOT NULL DEFAULT '',
    suspended     INTEGER NOT NULL DEFAULT 0,
    failed_logins INTEGER NOT NULL DEFAULT 0,
    locked_until  INTEGER NOT NULL DEFAULT 0,
    locale        TEXT NOT NULL DEFAULT 'en',
    theme         TEXT NOT NULL DEFAULT 'light',
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);
CREATE INDEX idx_users_owner ON users(owner_id);

CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,           -- sha256 of the opaque cookie token
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    csrf_token TEXT NOT NULL,
    ip         TEXT NOT NULL,
    user_agent TEXT NOT NULL,
    pending_2fa INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    last_seen  INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
CREATE INDEX idx_sessions_user ON sessions(user_id);

CREATE TABLE api_tokens (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,       -- sha256 of the bearer token
    scopes     TEXT NOT NULL,              -- space separated, e.g. "accounts:write billing"
    allowed_ips TEXT NOT NULL DEFAULT '',
    last_used  INTEGER NOT NULL DEFAULT 0,
    expires_at INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE TABLE packages (
    id            TEXT PRIMARY KEY,
    owner_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    disk_mb       INTEGER NOT NULL DEFAULT 0, -- 0 = unlimited
    bandwidth_mb  INTEGER NOT NULL DEFAULT 0,
    max_domains   INTEGER NOT NULL DEFAULT 0,
    max_databases INTEGER NOT NULL DEFAULT 0,
    max_mailboxes INTEGER NOT NULL DEFAULT 0,
    max_cron      INTEGER NOT NULL DEFAULT 0,
    max_apps      INTEGER NOT NULL DEFAULT 0,
    cpu_percent   INTEGER NOT NULL DEFAULT 100, -- cgroup cpu.max, 100 = one core
    memory_mb     INTEGER NOT NULL DEFAULT 1024,
    io_weight     INTEGER NOT NULL DEFAULT 100,
    max_procs     INTEGER NOT NULL DEFAULT 200,
    php_versions  TEXT NOT NULL DEFAULT '8.3',
    shell_access  INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL,
    UNIQUE (owner_id, name)
);

CREATE TABLE accounts (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    package_id   TEXT NOT NULL REFERENCES packages(id) ON DELETE RESTRICT,
    sys_user     TEXT NOT NULL UNIQUE,     -- linux username
    sys_uid      INTEGER NOT NULL UNIQUE,
    home_dir     TEXT NOT NULL,
    primary_domain TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','terminating')),
    suspend_reason TEXT NOT NULL DEFAULT '',
    node_id      TEXT NOT NULL DEFAULT 'local',
    external_ref TEXT NOT NULL DEFAULT '', -- billing system service id (WHMCS/Paymenter)
    disk_used_mb INTEGER NOT NULL DEFAULT 0,
    bw_used_mb   INTEGER NOT NULL DEFAULT 0,
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);
CREATE INDEX idx_accounts_ref ON accounts(external_ref);

CREATE TABLE domains (
    id          TEXT PRIMARY KEY,
    account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name        TEXT NOT NULL UNIQUE,
    kind        TEXT NOT NULL CHECK (kind IN ('primary','addon','subdomain','alias')),
    doc_root    TEXT NOT NULL,
    php_version TEXT NOT NULL DEFAULT '8.3',
    force_https INTEGER NOT NULL DEFAULT 1,
    created_at  INTEGER NOT NULL
);
CREATE INDEX idx_domains_account ON domains(account_id);

CREATE TABLE db_instances (
    id         TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name       TEXT NOT NULL UNIQUE,
    size_mb    INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE TABLE db_users (
    id         TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name       TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL
);

CREATE TABLE db_grants (
    db_id      TEXT NOT NULL REFERENCES db_instances(id) ON DELETE CASCADE,
    db_user_id TEXT NOT NULL REFERENCES db_users(id) ON DELETE CASCADE,
    privileges TEXT NOT NULL,
    PRIMARY KEY (db_id, db_user_id)
);

-- Mail tables are read directly by Postfix and Dovecot through SQL maps.
CREATE TABLE mailboxes (
    id         TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    domain_id  TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    local_part TEXT NOT NULL,
    address    TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,           -- {ARGON2ID} for Dovecot
    quota_mb   INTEGER NOT NULL DEFAULT 1024,
    used_mb    INTEGER NOT NULL DEFAULT 0,
    active     INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL
);

CREATE TABLE mail_aliases (
    id          TEXT PRIMARY KEY,
    account_id   TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    source       TEXT NOT NULL UNIQUE,
    destination  TEXT NOT NULL,
    created_at   INTEGER NOT NULL
);

CREATE TABLE mail_filters (
    id         TEXT PRIMARY KEY,
    mailbox_id TEXT NOT NULL REFERENCES mailboxes(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    sieve      TEXT NOT NULL,
    position   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE dns_zones (
    id         TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name       TEXT NOT NULL UNIQUE,
    dnssec     INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE TABLE ssl_certs (
    id         TEXT PRIMARY KEY,
    domain_id  TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    issuer     TEXT NOT NULL,              -- 'letsencrypt' | 'custom' | 'selfsigned'
    names      TEXT NOT NULL,
    not_after  INTEGER NOT NULL,
    cert_path  TEXT NOT NULL,
    key_path   TEXT NOT NULL,
    auto_renew INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL
);

CREATE TABLE cron_jobs (
    id         TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    schedule   TEXT NOT NULL,
    command    TEXT NOT NULL,
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL
);

CREATE TABLE ssh_keys (
    id          TEXT PRIMARY KEY,
    account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    public_key  TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    created_at  INTEGER NOT NULL,
    UNIQUE (account_id, fingerprint)
);

CREATE TABLE ftp_accounts (
    id         TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    login      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    directory  TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE TABLE backups (
    id         TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('full','files','databases','mail')),
    path       TEXT NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    status     TEXT NOT NULL DEFAULT 'running',
    destination TEXT NOT NULL DEFAULT 'local',
    created_at INTEGER NOT NULL
);

CREATE TABLE apps (
    id         TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    domain_id  TEXT REFERENCES domains(id) ON DELETE SET NULL,
    name       TEXT NOT NULL,
    runtime    TEXT NOT NULL,              -- node | python | docker
    image      TEXT NOT NULL DEFAULT '',
    port       INTEGER NOT NULL DEFAULT 0,
    env        TEXT NOT NULL DEFAULT '{}',
    status     TEXT NOT NULL DEFAULT 'stopped',
    created_at INTEGER NOT NULL,
    UNIQUE (account_id, name)
);

CREATE TABLE firewall_rules (
    id         TEXT PRIMARY KEY,
    action     TEXT NOT NULL CHECK (action IN ('allow','deny')),
    protocol   TEXT NOT NULL CHECK (protocol IN ('tcp','udp','any')),
    port       TEXT NOT NULL DEFAULT '',
    source     TEXT NOT NULL DEFAULT '',
    comment    TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);

CREATE TABLE nodes (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    address    TEXT NOT NULL,
    role       TEXT NOT NULL DEFAULT 'web',
    status     TEXT NOT NULL DEFAULT 'online',
    last_seen  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE audit_log (
    id         TEXT PRIMARY KEY,
    at         INTEGER NOT NULL,
    actor_id   TEXT NOT NULL DEFAULT '',
    actor_name TEXT NOT NULL DEFAULT '',
    action     TEXT NOT NULL,
    target     TEXT NOT NULL DEFAULT '',
    ip         TEXT NOT NULL DEFAULT '',
    detail     TEXT NOT NULL DEFAULT '',
    ok         INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX idx_audit_at ON audit_log(at);
CREATE INDEX idx_audit_actor ON audit_log(actor_id);
