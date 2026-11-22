-- AutoSSL exclusions, wildcard redirects, bandwidth accounting, Leech Protection.
ALTER TABLE domains ADD COLUMN autossl INTEGER NOT NULL DEFAULT 1;
ALTER TABLE redirects ADD COLUMN wildcard INTEGER NOT NULL DEFAULT 0;
ALTER TABLE protected_dirs ADD COLUMN leech_limit INTEGER NOT NULL DEFAULT 0;
ALTER TABLE protected_dir_users ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0;

CREATE TABLE bw_monthly (
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    month      TEXT NOT NULL,              -- 2026-11 (UTC)
    bytes      BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, month)
);

CREATE TABLE log_cursors (
    domain_id  TEXT PRIMARY KEY REFERENCES domains(id) ON DELETE CASCADE,
    log_offset BIGINT NOT NULL DEFAULT 0,
    head       TEXT NOT NULL DEFAULT ''    -- hash of the first line, to notice rotation
);

CREATE TABLE leech_hits (
    dir_id   TEXT NOT NULL REFERENCES protected_dirs(id) ON DELETE CASCADE,
    username TEXT NOT NULL,
    ip       TEXT NOT NULL,
    at       BIGINT NOT NULL,
    PRIMARY KEY (dir_id, username, ip)
);
