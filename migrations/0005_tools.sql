-- v5: dedicated cPanel/WHM tools: email routing, domain forwarders, mailing
-- lists, email filters, directory indexes, MIME types, compression,
-- dynamic DNS, contact preferences.

-- local: this server accepts mail for the domain; remote: mail is handled
-- elsewhere (Google Workspace, Microsoft 365) and Postfix relays to the MX.
ALTER TABLE domains ADD COLUMN mail_routing TEXT NOT NULL DEFAULT 'local';

-- Rewrites user@domain to user@target for every address of domain.
CREATE TABLE mail_domain_forwards (
    id         TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    domain     TEXT NOT NULL UNIQUE,
    target     TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

-- Distribution lists: one address delivering to many. Postfix reads members
-- (comma separated) through a virtual alias map.
CREATE TABLE mailing_lists (
    id          TEXT PRIMARY KEY,
    account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    address     TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    members     TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL
);

-- Sieve rules. mailbox_id '' means every mailbox of the account (global).
-- (0001's mail_filters table, holding raw Sieve, was never used.)
CREATE TABLE mail_rules (
    id         TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    mailbox_id TEXT NOT NULL DEFAULT '',
    name       TEXT NOT NULL,
    field      TEXT NOT NULL,
    op         TEXT NOT NULL,
    value      TEXT NOT NULL,
    action     TEXT NOT NULL,
    dest       TEXT NOT NULL DEFAULT '',
    position   INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_mail_rules_account ON mail_rules(account_id);

CREATE TABLE dir_indexes (
    id         TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    domain_id  TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    path       TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (domain_id, path)
);

CREATE TABLE mime_types (
    id         TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    ext        TEXT NOT NULL,
    mime       TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (account_id, ext)
);

CREATE TABLE web_settings (
    account_id TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    gzip       TEXT NOT NULL DEFAULT 'off',   -- off | all | custom
    gzip_types TEXT NOT NULL DEFAULT ''
);

CREATE TABLE ddns_hosts (
    id          TEXT PRIMARY KEY,
    account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    zone        TEXT NOT NULL,
    name        TEXT NOT NULL,
    token_hash  TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    last_ip     TEXT NOT NULL DEFAULT '',
    last_update INTEGER NOT NULL DEFAULT 0,
    created_at  INTEGER NOT NULL,
    UNIQUE (zone, name)
);

CREATE TABLE user_prefs (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key     TEXT NOT NULL,
    value   TEXT NOT NULL,
    PRIMARY KEY (user_id, key)
);
