-- v3: master resellers, reseller allocation limits, per-account shell access.

-- A master reseller is a reseller that may create resellers of its own.
ALTER TABLE users ADD COLUMN is_master INTEGER NOT NULL DEFAULT 0;
-- Allocation limits for resellers (0 = unlimited).
ALTER TABLE users ADD COLUMN max_accounts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN max_disk_mb INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN max_resellers INTEGER NOT NULL DEFAULT 0;

-- Shell access per account: none (no login shell), sftp (chrooted file
-- transfer only) or full (/bin/bash, also enables the web terminal).
ALTER TABLE accounts ADD COLUMN shell TEXT NOT NULL DEFAULT 'none';

-- Rolling resource samples per account, written by the metrics collector.
CREATE TABLE account_metrics (
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    at         INTEGER NOT NULL,
    cpu_pct    REAL NOT NULL,
    mem_mb     INTEGER NOT NULL,
    procs      INTEGER NOT NULL,
    PRIMARY KEY (account_id, at)
);
