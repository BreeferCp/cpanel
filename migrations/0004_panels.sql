-- v4: feature lists, FTP, mail filtering, web protection, remote MySQL, Git,
-- login access rules.

-- A feature list names the cPanel tools a package does NOT get. Packages
-- without a list get every tool.
CREATE TABLE feature_lists (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    disabled   TEXT NOT NULL DEFAULT '',   -- space separated feature keys
    created_at INTEGER NOT NULL,
    UNIQUE (owner_id, name)
);
ALTER TABLE packages ADD COLUMN feature_list_id TEXT NOT NULL DEFAULT '';
ALTER TABLE packages ADD COLUMN max_ftp INTEGER NOT NULL DEFAULT 0;

-- FTP accounts (table from 0001): virtual users mapped to the account's uid
-- and confined to a directory inside its home; hashes are SHA-512 crypt.
ALTER TABLE ftp_accounts ADD COLUMN quota_mb INTEGER NOT NULL DEFAULT 0;

CREATE TABLE autoresponders (
    mailbox_id TEXT PRIMARY KEY REFERENCES mailboxes(id) ON DELETE CASCADE,
    subject    TEXT NOT NULL,
    body       TEXT NOT NULL,
    interval_days INTEGER NOT NULL DEFAULT 1,
    starts_on  TEXT NOT NULL DEFAULT '',    -- YYYY-MM-DD or empty
    ends_on    TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);

CREATE TABLE spam_settings (
    account_id TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    enabled    INTEGER NOT NULL DEFAULT 0,
    threshold  INTEGER NOT NULL DEFAULT 5,
    action     TEXT NOT NULL DEFAULT 'junk' CHECK (action IN ('junk','discard')),
    allow      TEXT NOT NULL DEFAULT '',    -- newline separated senders
    deny       TEXT NOT NULL DEFAULT ''
);

-- Password-protected directories (HTTP basic auth).
CREATE TABLE protected_dirs (
    id         TEXT PRIMARY KEY,
    domain_id  TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    path       TEXT NOT NULL,               -- URL path, e.g. /private
    realm      TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (domain_id, path)
);
CREATE TABLE protected_dir_users (
    dir_id        TEXT NOT NULL REFERENCES protected_dirs(id) ON DELETE CASCADE,
    username      TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    PRIMARY KEY (dir_id, username)
);

-- Per-account web access rules applied to every domain of the account.
CREATE TABLE ip_blocks (
    id         TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    cidr       TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (account_id, cidr)
);
CREATE TABLE hotlink_settings (
    account_id TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    enabled    INTEGER NOT NULL DEFAULT 0,
    allowed    TEXT NOT NULL DEFAULT '',    -- extra referrer hosts, space separated
    extensions TEXT NOT NULL DEFAULT 'jpg jpeg gif png webp svg bmp mp4 mp3',
    allow_empty INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE remote_mysql_hosts (
    id         TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    host       TEXT NOT NULL,
    note       TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    UNIQUE (account_id, host)
);

CREATE TABLE git_repos (
    id         TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    path       TEXT NOT NULL,               -- relative to home
    clone_url  TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    UNIQUE (account_id, path)
);

-- Panel login access rules (brute force protection): an allow entry skips
-- rate limiting, a deny entry refuses sign-in outright.
CREATE TABLE login_rules (
    id         TEXT PRIMARY KEY,
    cidr       TEXT NOT NULL UNIQUE,
    kind       TEXT NOT NULL CHECK (kind IN ('allow','deny')),
    note       TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);

-- WordPress toolkit: auto-update policy, staging link and backups.
ALTER TABLE installations ADD COLUMN auto_core TEXT NOT NULL DEFAULT 'minor';    -- off | minor | major
ALTER TABLE installations ADD COLUMN auto_plugins INTEGER NOT NULL DEFAULT 0;
ALTER TABLE installations ADD COLUMN auto_themes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE installations ADD COLUMN staging_of TEXT NOT NULL DEFAULT '';
ALTER TABLE installations ADD COLUMN last_update_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE installations ADD COLUMN last_update_result TEXT NOT NULL DEFAULT '';
CREATE TABLE install_backups (
    id              TEXT PRIMARY KEY,
    installation_id TEXT NOT NULL REFERENCES installations(id) ON DELETE CASCADE,
    path            TEXT NOT NULL,              -- directory holding files.tar.gz and db.sql
    reason          TEXT NOT NULL DEFAULT 'manual',  -- manual | pre-update | pre-restore | pre-push
    size_bytes      INTEGER NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'running',
    created_at      INTEGER NOT NULL
);
