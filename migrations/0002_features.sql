-- v2: support tickets, app installer, redirects, PHP settings, phpMyAdmin SSO,
-- runtime versions for apps.

CREATE TABLE tickets (
    id          TEXT PRIMARY KEY,
    number      INTEGER NOT NULL UNIQUE,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,   -- customer who opened it
    owner_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,   -- reseller/root who answers it
    subject     TEXT NOT NULL,
    department  TEXT NOT NULL DEFAULT 'support',
    priority    TEXT NOT NULL DEFAULT 'normal' CHECK (priority IN ('low','normal','high','urgent')),
    status      TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','answered','customer_reply','closed')),
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);
CREATE INDEX idx_tickets_user ON tickets(user_id);
CREATE INDEX idx_tickets_owner ON tickets(owner_id, status);

CREATE TABLE ticket_messages (
    id         TEXT PRIMARY KEY,
    ticket_id  TEXT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    author_id  TEXT NOT NULL,
    author     TEXT NOT NULL,
    staff      INTEGER NOT NULL DEFAULT 0,
    body       TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_ticket_messages ON ticket_messages(ticket_id, created_at);

CREATE TABLE installations (
    id          TEXT PRIMARY KEY,
    account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    domain_id   TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    app         TEXT NOT NULL,           -- catalog id, e.g. wordpress
    path        TEXT NOT NULL,           -- absolute install directory
    url         TEXT NOT NULL,
    version     TEXT NOT NULL DEFAULT '',
    db_name     TEXT NOT NULL DEFAULT '',
    admin_user  TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'installing',
    created_at  INTEGER NOT NULL,
    UNIQUE (domain_id, path)
);

CREATE TABLE redirects (
    id         TEXT PRIMARY KEY,
    domain_id  TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    source     TEXT NOT NULL,             -- path on the domain, e.g. /old
    target     TEXT NOT NULL,             -- absolute URL
    code       INTEGER NOT NULL DEFAULT 301 CHECK (code IN (301, 302, 307, 308)),
    created_at INTEGER NOT NULL,
    UNIQUE (domain_id, source)
);

CREATE TABLE php_settings (
    account_id TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    settings   TEXT NOT NULL DEFAULT '{}' -- JSON map of allow-listed php.ini keys
);

CREATE TABLE pma_sessions (
    token_hash TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    db_user    TEXT NOT NULL,             -- temporary MariaDB user
    password   TEXT NOT NULL,
    redeemed   INTEGER NOT NULL DEFAULT 0,
    expires_at INTEGER NOT NULL
);

ALTER TABLE apps ADD COLUMN runtime_version TEXT NOT NULL DEFAULT '';
ALTER TABLE apps ADD COLUMN command TEXT NOT NULL DEFAULT '';
ALTER TABLE domains ADD COLUMN error_pages TEXT NOT NULL DEFAULT '';
