# Architecture

## Components

`cmd/breeferd` is the only server-side program. It runs as root (it creates users, writes system config and loads firewall rules) and exposes one HTTP(S) listener, default `:2087`, that serves both the JSON API under `/api/v1` and the compiled React app.

| Package | Responsibility |
|---|---|
| `internal/config` | Config file and `BREEFER_*` environment overrides, validation |
| `internal/store` | `database/sql` wrapper for SQLite and PostgreSQL, `?` to `$n` rebinding, embedded migrations |
| `internal/auth` | Argon2id hashing, TOTP (RFC 6238), opaque token helpers |
| `internal/rbac` | Roles (`root`, `reseller`, `user`) and the permission matrix |
| `internal/api` | HTTP routing, middleware, handlers, audit trail, background jobs |
| `internal/provision` | Turns panel state into host configuration |
| `internal/system` | Shell-free command runner, strict input validators |
| `internal/files` | File manager core, confined with `os.Root` |
| `web/` | React + TypeScript + Tailwind single-page app |

## Request flow

```
request ─► recoverer ─► security headers ─► rate limit ─► authenticate
        ─► require(permission)   (CSRF check for cookie sessions, scope check for API tokens)
        ─► handler ─► loadAccount (tenant check) ─► store + provision ─► audit
```

Handlers return `error`; a single wrapper maps typed errors to status codes and turns anything unexpected into an opaque `500` after logging it. Internal detail (SQL, paths, command output) never reaches a client.

## Tenancy

There is one rule for resource access: **`loadAccount`**. Every account-scoped route takes `{aid}` (or the literal `me` for a customer), and `loadAccount` returns *not found* unless the caller is root, the reseller who owns that customer, or the customer themselves. Child resources (domains, databases, mailboxes, DNS zones, …) are always queried with `AND account_id = ?` from that loaded account, never by bare id.

## Data model

SQLite and PostgreSQL share one portable schema (`migrations/`): text ids, integer unix timestamps, no vendor types. SQLite runs with WAL, foreign keys on and one writer connection. PostgreSQL is the intended store for clusters.

Postfix and Dovecot read the `domains`, `mailboxes`, `mail_aliases` and `accounts` tables directly (SQL maps in `deploy/`), so mailbox changes take effect without a reload and there is no second source of truth for mail.

## Provisioning

Panel state is the source of truth; the provisioner renders host config from it. Rules that every provisioner follows:

1. **Validate first.** Domains, usernames, cron fields, ports, CIDRs, PHP versions and the like pass through strict allow-list validators in `internal/system/validate.go` before they reach a template or argv. Nothing is "escaped".
2. **No shell.** `system.Runner` uses `exec.Command` with discrete arguments and a fixed `PATH`. Secrets go through stdin.
3. **Validate before activating.** nginx vhosts are tested with `nginx -t` and rolled back on failure; the firewall ruleset is checked with `nft -c` before it replaces the live one.
4. **Compensate on failure.** Account creation writes DB rows first, then host state; if the host step fails, both are rolled back so no half-created account remains.
5. **Dry-run.** With `dry_run = true` every command is recorded instead of executed, which is how the provisioners are unit-tested on any OS.

### Isolation

Each account is a Linux user with a `0700` home. nginx traverses it through an ACL entry only; other accounts cannot enter it. Per account:

- a dedicated PHP-FPM pool (own uid, own socket, `open_basedir`, restricted `disable_functions`, `ondemand` process manager),
- a `user-<uid>.slice` drop-in carrying `CPUQuota`, `MemoryMax`, `IOWeight` and `TasksMax` from the package (cgroups v2),
- Node/Python apps as systemd services inside that slice with `NoNewPrivileges`, `ProtectSystem=strict`, an empty capability set and a syscall filter,
- Docker apps with all capabilities dropped, read-only root, the account's uid, and the account's slice as cgroup parent,
- mail stored in the account's home and served by Dovecot as the account's uid.

## Frontend

A single-page app with React Router and TanStack Query. The same hosting pages mount at `/hosting/*` for a customer and at `/accounts/:aid/*` for an administrator managing that customer, so there is exactly one implementation of each feature. Design tokens live in `web/src/index.css` as CSS variables; the dark theme swaps the variable set.

## Clustering (planned)

`accounts.node_id` and the `nodes` table exist, and PostgreSQL is supported, but the node agent (a `breeferd agent` role that receives provisioning calls over mTLS) is not implemented yet. Until then a deployment is a single node.
