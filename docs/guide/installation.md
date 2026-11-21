# Installation guide

> BreeferCP is early alpha. Try it on a throwaway server first.

## Requirements

- A **dedicated** server or VM with a fresh install of Debian 12+ or Ubuntu 22.04+ (root access, systemd, cgroups v2).
- 2 GB RAM minimum, 4 GB recommended; 20 GB disk plus space for customer data.
- A public IPv4 address and a hostname for the panel (for example `panel.example.com`) with an A record pointing at it.
- Ports open to the internet: 22, 25, 53 (TCP and UDP), 80, 143, 443, 465, 587, 993, 2087.

The installer takes over nginx, Postfix, Dovecot, PowerDNS and nftables. Do not run it on a server that already hosts sites you care about.

## 1. Get a release

Build one yourself (see [Building](#building-from-source)) or download a release tarball, then extract it:

```bash
tar xzf breefercp-<version>-linux-amd64.tar.gz
cd breefercp-<version>-linux-amd64
```

## 2. Run the installer

```bash
sudo ./deploy/install.sh --email you@example.com --hostname panel.example.com
```

It installs and configures nginx, PHP-FPM, MariaDB, Postfix, Dovecot, OpenDKIM, PowerDNS, nftables, fail2ban and Docker, installs `breeferd` as a systemd service, generates all service passwords, and creates the first administrator. The administrator password is printed **once** at the end.

Options: `--admin-user NAME` (default `admin`), `--skip-docker`.

## 3. First sign-in

1. Open `https://panel.example.com:2087`. The first certificate is self-signed, so your browser will warn you.
2. Sign in as the administrator.
3. Go to **Profile & Security** and turn on two-factor authentication.
4. Go to **Settings** and set the **server IP address** (used for DNS records) and your nameservers.
5. Point your domain registrar's nameservers (or glue records) at this server so PowerDNS answers for hosted domains.

## 4. Create your first customer

1. **Packages → Create package** (for example Starter: 10 GB disk, 5 domains, 1 core, 1 GB RAM).
2. **Accounts → Create account**: domain, username, email, package. Leave the password blank to generate one; it is shown once.
3. Use **⋯ → Sign in as customer** to see the customer's view.

## Configuration

The daemon reads `/etc/breefercp/breefercp.conf`. Every key can be overridden with a `BREEFER_` environment variable (`BREEFER_LISTEN`, and so on). See [`deploy/breefercp.conf.example`](../../deploy/breefercp.conf.example) for all keys.

| Key | Purpose |
|---|---|
| `listen` | Panel/API address, default `0.0.0.0:2087` |
| `tls_cert` / `tls_key` | Panel TLS certificate |
| `db_driver` / `db_dsn` | `sqlite` (single node) or `postgres` |
| `mariadb_dsn` | Admin connection used to create customer databases |
| `powerdns_url` / `powerdns_api_key` | PowerDNS HTTP API |
| `web_user` | User nginx and PHP-FPM run as (`www-data` on Debian/Ubuntu) |
| `trusted_proxies` | CIDRs whose `X-Forwarded-For` header is trusted |
| `dry_run` | Log host commands instead of running them (development) |

## Operations

```bash
systemctl status breeferd          # service state
journalctl -u breeferd -f          # JSON logs
breeferd create-admin -config /etc/breefercp/breefercp.conf -email you@example.com   # add another root user
```

Back up `/var/lib/breefercp` (panel database and secret key) and `/etc/breefercp`. Customer data lives in `/home/<user>`.

## Building from source

Requires Go 1.26+, Node 20+ and make.

```bash
make build        # dist/breefercp-<version>-linux-{amd64,arm64}.tar.gz
make test         # vet, Go tests, UI type-check
```

## Development without a server

```bash
cd web && npm install && npm run build && cd ..
export BREEFER_DRY_RUN=true BREEFER_LISTEN=127.0.0.1:2087 BREEFER_DB_DSN=./dev.db \
       BREEFER_DATA_DIR=./dev-data BREEFER_WEB_DIR=./web/dist BREEFER_HOME_BASE=./dev-home
go run ./cmd/breeferd create-admin -config /dev/null -email you@example.com
go run ./cmd/breeferd serve -config /dev/null
```

For live UI reloads run `npm run dev` in `web/`; Vite proxies `/api` to the daemon.

## Troubleshooting

| Symptom | Check |
|---|---|
| Panel unreachable | `systemctl status breeferd`, firewall port 2087, `journalctl -u breeferd` |
| "web UI is not installed" | `web_dir` must point at the built `web/` directory |
| Site shows 502 | `systemctl status php8.3-fpm`; the account's pool file is in `/etc/php/<v>/fpm/pool.d/breefer-<user>.conf` |
| Site shows 403/404 for a new account | Confirm `web_user` matches the user nginx runs as (`ps -o user= -C nginx`) |
| Certificate issue fails | The domain must already resolve to this server and port 80 must be reachable |
| Mail not delivered | `postfix check`, `doveadm user user@domain`, and the DNS MX/SPF/DKIM records |
| Customer cannot create database | `mariadb_dsn` missing or the `breefer` MariaDB user lacks grants |
