# Installation guide

> BreeferCP is early alpha. Try it on a throwaway server first.

## Requirements

- A **dedicated** server or VM with a fresh install of Debian 12+ or Ubuntu 22.04+ (root access, systemd, cgroups v2).
- 2 GB RAM minimum, 4 GB recommended; 20 GB disk plus space for customer data.
- A public IPv4 address and a hostname for the panel (for example `panel.example.com`) with an A record pointing at it.
- Ports open to the internet: 21 and 30000–30999 (FTPS), 22, 25, 53 (TCP and UDP), 80, 110, 143, 443, 465, 587, 993, 995, 2083 (cPanel), 2087 (WHM) and 8443 (phpMyAdmin and webmail). The installer's firewall opens these.
- For disk quotas, mount `/home` with `usrquota` and run `quotacheck -cum /home && quotaon /home`. Without it, usage is still tracked and shown, but not enforced by the filesystem.

The installer takes over nginx, Postfix, Dovecot, PowerDNS and nftables. Do not run it on a server that already hosts sites you care about.

## 1. Get a release

Download the tarball for your architecture and `SHA256SUMS` from [Releases](https://github.com/BreeferCp/cpanel/releases), check it with `sha256sum -c --ignore-missing SHA256SUMS` (or build one yourself, see [Building](#building-from-source)), then extract it:

```bash
tar xzf breefercp-<version>-linux-amd64.tar.gz
cd breefercp-<version>-linux-amd64
```

## 2. Run the installer

```bash
sudo ./deploy/install.sh --email you@example.com --hostname panel.example.com
```

It installs and configures nginx, PHP-FPM, MariaDB, Postfix, Dovecot (with Sieve), Rspamd, OpenDKIM, PowerDNS, Pure-FTPd, nftables, fail2ban, Docker, phpMyAdmin (with panel single sign-on), Roundcube, WP-CLI, Composer and Git. It also sets up SFTP-only SSH access, installs `breeferd` as a systemd service, generates all service passwords, and creates the first administrator. The administrator password is printed **once** at the end.

Options: `--admin-user NAME` (default `admin`), `--skip-docker`.

## 3. First sign-in

BreeferCP runs two panels:

| Panel | Address | Who signs in |
|---|---|---|
| WHM | `https://panel.example.com:2087` | root administrators, master resellers, resellers |
| cPanel | `https://panel.example.com:2083` | hosting customers |

Each refuses the other's users and has its own session, so you can be signed in to both at once. **Accounts → ⋯ → Sign in as customer** opens the customer's cPanel in a new tab.

1. Open WHM at `https://panel.example.com:2087`. The first certificate is self-signed, so your browser will warn you.
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
| `listen` | WHM address, default `0.0.0.0:2087` |
| `cpanel_listen` | cPanel address, default `0.0.0.0:2083`; empty serves both panels from `listen` |
| `tls_cert` / `tls_key` | Panel TLS certificate |
| `db_driver` / `db_dsn` | `sqlite` (single node) or `postgres` |
| `mariadb_dsn` | Admin connection used to create customer databases |
| `powerdns_url` / `powerdns_api_key` | PowerDNS HTTP API |
| `web_user` | User nginx and PHP-FPM run as (`www-data` on Debian/Ubuntu) |
| `web_server` | `nginx` (default) or `openlitespeed` (experimental; install OpenLiteSpeed and lsphp first) |
| `trusted_proxies` | CIDRs whose `X-Forwarded-For` header is trusted |
| `dry_run` | Log host commands instead of running them (development) |

## Operations

```bash
systemctl status breeferd          # service state
journalctl -u breeferd -f          # JSON logs
breeferd create-admin -config /etc/breefercp/breefercp.conf -email you@example.com   # add another root user
```

Back up `/var/lib/breefercp` (panel database and secret key) and `/etc/breefercp`. Customer data lives in `/home/<user>`. **WHM → Backup Configuration** schedules account backups into `/var/lib/breefercp/backups`; copy that directory off the server as well.

## Updating

- **WHM → Updates** shows the installed and latest version with release notes. **Update** downloads the release for your architecture from GitHub, installs it only if its SHA-256 matches the release's `SHA256SUMS`, keeps the previous version and restarts the panel. **Roll back** restores the previous version.
- Set **Update preference** to *Automatic* to install new releases between 03:00 and 05:00 server time.
- From a shell: `sudo breeferd update -config /etc/breefercp/breefercp.conf` (`-check` only reports, `-rollback` goes back), then `systemctl restart breeferd`.
- **Operating system packages** on the same page runs `apt-get upgrade` in the background and shows its output.

Database migrations run automatically when the new version starts.

The updater replaces only the binary and the UI. When release notes say that service configuration changed, and for **every install from before 0.5.0**, extract the new release and run `sudo ./deploy/install.sh` again with the same `--email` and `--hostname`. This rewrites `/etc/breefercp/breefercp.conf`, so keep a copy of your own changes.

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
       BREEFER_DATA_DIR=./dev-data BREEFER_WEB_DIR=./web/dist BREEFER_HOME_BASE=./dev-home \
       BREEFER_CPANEL_LISTEN=127.0.0.1:2083
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
