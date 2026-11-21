#!/usr/bin/env bash
# BreeferCP installer for Debian 12 / Ubuntu 22.04+ on a fresh server.
#
# Run from the extracted release directory as root:
#     sudo ./deploy/install.sh --email admin@example.com --hostname panel.example.com
#
# It installs the hosting stack (nginx, PHP-FPM, MariaDB, Postfix, Dovecot,
# PowerDNS, OpenDKIM, nftables, Docker), configures it for BreeferCP, installs
# the daemon as a systemd service and creates the first administrator.
# It is meant for a DEDICATED server: it takes ownership of nginx, Postfix,
# Dovecot and the firewall configuration.
set -Eeuo pipefail

HOSTNAME_FQDN=""
ADMIN_EMAIL=""
ADMIN_USER="admin"
SKIP_DOCKER=0

usage() {
  cat <<USAGE
Usage: install.sh --email EMAIL --hostname FQDN [--admin-user NAME] [--skip-docker]
USAGE
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --email) ADMIN_EMAIL="${2:-}"; shift 2 ;;
    --hostname) HOSTNAME_FQDN="${2:-}"; shift 2 ;;
    --admin-user) ADMIN_USER="${2:-}"; shift 2 ;;
    --skip-docker) SKIP_DOCKER=1; shift ;;
    -h|--help) usage ;;
    *) echo "Unknown option: $1" >&2; usage ;;
  esac
done

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }
rand() { openssl rand -base64 48 | tr -dc 'A-Za-z0-9' | head -c "${1:-32}"; }

[[ $EUID -eq 0 ]] || die "run as root"
[[ -n "$ADMIN_EMAIL" && -n "$HOSTNAME_FQDN" ]] || usage
[[ "$HOSTNAME_FQDN" == *.* ]] || die "--hostname must be a fully qualified domain name"

SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[[ -x "$SRC/breeferd" ]] || die "breeferd binary not found in $SRC (run from the release tarball)"
[[ -d "$SRC/web" ]]      || die "web/ directory not found in $SRC"

. /etc/os-release
case "${ID}:${VERSION_ID}" in
  debian:12|debian:13|ubuntu:22.04|ubuntu:24.04|ubuntu:26.04) ;;
  *) die "unsupported OS ${PRETTY_NAME}; supported: Debian 12+, Ubuntu 22.04+" ;;
esac

export DEBIAN_FRONTEND=noninteractive
PHP_VERSIONS=(8.3 8.2)
if [[ "$ID" == "debian" ]]; then PHP_VERSIONS=(8.2 8.4); fi

log "Installing packages"
echo "postfix postfix/main_mailer_type select Internet Site" | debconf-set-selections
echo "postfix postfix/mailname string ${HOSTNAME_FQDN}" | debconf-set-selections
apt-get update -qq
apt-get install -y -qq \
  nginx mariadb-server sqlite3 openssl curl ca-certificates cron nftables fail2ban \
  postfix postfix-sqlite dovecot-core dovecot-imapd dovecot-pop3d dovecot-lmtpd dovecot-sqlite \
  opendkim opendkim-tools pdns-server pdns-backend-sqlite3 acl unzip zip   dovecot-sieve dovecot-managesieved rspamd pure-ftpd quota file git traceroute

for v in "${PHP_VERSIONS[@]}"; do
  apt-get install -y -qq "php${v}-fpm" "php${v}-cli" "php${v}-mysql" "php${v}-curl" "php${v}-mbstring" \
    "php${v}-xml" "php${v}-zip" "php${v}-gd" "php${v}-intl" "php${v}-sqlite3" 2>/dev/null \
    || log "PHP ${v} is not available from the configured repositories; skipping"
done

if [[ $SKIP_DOCKER -eq 0 ]] && ! command -v docker >/dev/null; then
  log "Installing Docker"
  apt-get install -y -qq docker.io
fi

log "Creating the breefer service group and directories"
getent group breefer >/dev/null || groupadd --system breefer
for u in postfix dovecot opendkim; do usermod -aG breefer "$u" 2>/dev/null || true; done
install -d -m 2770 -o root -g breefer /var/lib/breefercp
install -d -m 0750 -o root -g breefer /etc/breefercp
install -d -m 0755 /usr/share/breefercp /usr/share/breefercp/static /etc/breefercp/certs
install -d -m 0755 /etc/nginx/breefercp.d /var/lib/breefercp/acme-webroot

log "Installing the daemon and web UI"
install -m 0755 "$SRC/breeferd" /usr/local/bin/breeferd
rm -rf /usr/share/breefercp/web
cp -r "$SRC/web" /usr/share/breefercp/web
cat > /usr/share/breefercp/static/suspended.html <<'HTML'
<!doctype html><meta charset="utf-8"><title>Account suspended</title>
<style>body{font:16px system-ui,sans-serif;display:grid;place-items:center;min-height:100vh;margin:0;color:#444}</style>
<div><h1>This account has been suspended</h1><p>Please contact your hosting provider.</p></div>
HTML

log "Configuring MariaDB"
systemctl enable --now mariadb
DB_PASS="$(rand 32)"
mariadb <<SQL
CREATE USER IF NOT EXISTS 'breefer'@'localhost' IDENTIFIED BY '${DB_PASS}';
ALTER USER 'breefer'@'localhost' IDENTIFIED BY '${DB_PASS}';
GRANT CREATE, DROP, CREATE USER, RELOAD ON *.* TO 'breefer'@'localhost';
GRANT ALL PRIVILEGES ON \`%\`.* TO 'breefer'@'localhost' WITH GRANT OPTION;
FLUSH PRIVILEGES;
SQL

log "Configuring PowerDNS"
PDNS_KEY="$(rand 40)"
systemctl stop pdns || true
# Debian ships a default bind backend config that conflicts with ours.
rm -f /etc/powerdns/pdns.d/bind.conf
install -d -o pdns -g pdns /var/lib/powerdns
if [[ ! -f /var/lib/powerdns/pdns.sqlite3 ]]; then
  sqlite3 /var/lib/powerdns/pdns.sqlite3 < /usr/share/pdns-backend-sqlite3/schema/schema.sqlite3.sql 2>/dev/null \
    || sqlite3 /var/lib/powerdns/pdns.sqlite3 < /usr/share/doc/pdns-backend-sqlite3/schema.sqlite3.sql
  chown pdns:pdns /var/lib/powerdns/pdns.sqlite3
fi
sed "s/__PDNS_API_KEY__/${PDNS_KEY}/" "$SRC/deploy/pdns/pdns.conf" > /etc/powerdns/pdns.d/breefercp.conf
chmod 0640 /etc/powerdns/pdns.d/breefercp.conf
# systemd-resolved occupies port 53 on Ubuntu.
if systemctl is-active --quiet systemd-resolved; then
  mkdir -p /etc/systemd/resolved.conf.d
  printf '[Resolve]\nDNSStubListener=no\n' > /etc/systemd/resolved.conf.d/breefercp.conf
  systemctl restart systemd-resolved
fi
systemctl enable --now pdns

log "Configuring nginx"
cat > /etc/nginx/conf.d/00-breefercp.conf <<'NGINX'
# Per-account vhosts are generated by BreeferCP.
include /etc/nginx/breefercp.d/*.conf;

# Requests for unknown hosts get no response instead of another tenant's site.
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name _;
    location ^~ /.well-known/acme-challenge/ { root /var/lib/breefercp/acme-webroot; }
    location / { return 444; }
}
NGINX
rm -f /etc/nginx/sites-enabled/default
nginx -t
systemctl enable --now nginx

log "Configuring Postfix, Dovecot and OpenDKIM"
install -d -m 0750 -o root -g postfix /etc/postfix/breefercp
for f in domains mailboxes aliases lists domain_forwards; do install -m 0640 -o root -g postfix "$SRC/deploy/postfix/$f.cf" "/etc/postfix/breefercp/$f.cf"; done
if ! grep -q 'BreeferCP' /etc/postfix/main.cf; then
  { echo; echo "# --- BreeferCP ---"; cat "$SRC/deploy/postfix/main.cf.snippet"; } >> /etc/postfix/main.cf
fi
postconf -e "myhostname = ${HOSTNAME_FQDN}"
install -m 0644 "$SRC/deploy/dovecot/breefercp.conf" /etc/dovecot/conf.d/99-breefercp.conf
install -m 0640 -o root -g dovecot "$SRC/deploy/dovecot/dovecot-sql-breefercp.conf.ext" /etc/dovecot/dovecot-sql-breefercp.conf.ext
install -d -o opendkim -g opendkim /etc/opendkim/keys
cat > /etc/opendkim.conf <<'DKIM'
Syslog yes
UMask 007
Socket inet:8891@localhost
Mode sv
Canonicalization relaxed/simple
SignatureAlgorithm rsa-sha256
KeyTable refile:/etc/opendkim/key.table
SigningTable refile:/etc/opendkim/signing.table
DKIM
touch /etc/opendkim/key.table /etc/opendkim/signing.table

log "Configuring Rspamd (spam scoring for the per-account spam filters)"
install -d /etc/rspamd/local.d
cat > /etc/rspamd/local.d/milter_headers.conf <<'RSPAMD'
# BreeferCP: X-Spam-Level carries one * per point; customer Sieve rules act on it.
use = ["x-spamd-bar", "x-spam-level", "x-spam-status", "authentication-results"];
skip_local = false;
skip_authenticated = true;
RSPAMD
cat > /etc/rspamd/local.d/actions.conf <<'RSPAMD'
# Reject only obvious spam at SMTP time; everything else is delivered and
# filtered by each account's own threshold.
reject = 25;
add_header = 4;
greylist = 12;
RSPAMD
systemctl enable --now rspamd

log "Configuring Pure-FTPd (virtual users, chrooted, TLS)"
PFC=/etc/pure-ftpd/conf
install -d "$PFC" /etc/pure-ftpd/auth
echo yes > "$PFC/ChrootEveryone"
echo no > "$PFC/PAMAuthentication"
echo no > "$PFC/UnixAuthentication"
echo /etc/pure-ftpd/pureftpd.pdb > "$PFC/PureDB"
echo "30000 30999" > "$PFC/PassivePortRange"
echo 1000 > "$PFC/MinUID"
echo yes > "$PFC/NoAnonymous"
echo 1 > "$PFC/TLS"
ln -sf ../conf/PureDB /etc/pure-ftpd/auth/50pure
touch /etc/pure-ftpd/pureftpd.passwd && chmod 0600 /etc/pure-ftpd/pureftpd.passwd
pure-pw mkdb /etc/pure-ftpd/pureftpd.pdb -f /etc/pure-ftpd/pureftpd.passwd
install -d -m 0755 /etc/nginx/breefercp-htpasswd

log "Writing configuration"
cat > /etc/breefercp/breefercp.conf <<CONF
listen = 0.0.0.0:2087
cpanel_listen = 0.0.0.0:2083
tls_cert = /etc/breefercp/panel.crt
tls_key  = /etc/breefercp/panel.key
db_driver = sqlite
db_dsn    = /var/lib/breefercp/breefercp.db
data_dir  = /var/lib/breefercp
web_dir   = /usr/share/breefercp/web
mariadb_dsn = breefer:${DB_PASS}@unix(/run/mysqld/mysqld.sock)/?interpolateParams=true
powerdns_url = http://127.0.0.1:8081
powerdns_api_key = ${PDNS_KEY}
home_base = /home
nginx_conf_dir = /etc/nginx/breefercp.d
web_user = www-data
CONF
chown root:breefer /etc/breefercp/breefercp.conf
chmod 0640 /etc/breefercp/breefercp.conf

# Self-signed certificate for the panel until a real one is issued for the hostname.
if [[ ! -f /etc/breefercp/panel.crt ]]; then
  openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 90 \
    -keyout /etc/breefercp/panel.key -out /etc/breefercp/panel.crt \
    -subj "/CN=${HOSTNAME_FQDN}" -addext "subjectAltName=DNS:${HOSTNAME_FQDN}" 2>/dev/null
  chmod 0640 /etc/breefercp/panel.key
  chown root:breefer /etc/breefercp/panel.key /etc/breefercp/panel.crt
fi
# Pure-FTPd expects key and certificate in one file.
install -d -m 0700 /etc/ssl/private
cat /etc/breefercp/panel.key /etc/breefercp/panel.crt > /etc/ssl/private/pure-ftpd.pem
chmod 0600 /etc/ssl/private/pure-ftpd.pem
systemctl enable --now pure-ftpd && systemctl restart pure-ftpd

log "Installing developer tools (WP-CLI, Composer)"
apt-get install -y -qq composer git
if [[ ! -x /usr/local/bin/wp ]]; then
  curl -fsSL -o /usr/local/bin/wp https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar
  chmod 0755 /usr/local/bin/wp
fi

log "Configuring SSH access control"
getent group breefer-sftp >/dev/null || groupadd --system breefer-sftp
install -m 0644 "$SRC/deploy/ssh/breefercp.conf" /etc/ssh/sshd_config.d/50-breefercp.conf
sshd -t && systemctl reload ssh 2>/dev/null || systemctl reload sshd 2>/dev/null || true

log "Installing phpMyAdmin and Roundcube webmail"
echo "phpmyadmin phpmyadmin/dbconfig-install boolean false" | debconf-set-selections
echo "phpmyadmin phpmyadmin/reconfigure-webserver multiselect" | debconf-set-selections
echo "roundcube-core roundcube/dbconfig-install boolean true" | debconf-set-selections
echo "roundcube-core roundcube/database-type select sqlite3" | debconf-set-selections
apt-get install -y -qq phpmyadmin roundcube roundcube-sqlite3 roundcube-plugins
install -m 0644 "$SRC/deploy/phpmyadmin/breefercp-signon.php" /usr/share/phpmyadmin/breefercp-signon.php
if ! grep -q BreeferPMA /etc/phpmyadmin/config.inc.php; then
  cat >> /etc/phpmyadmin/config.inc.php <<'PMA'
// --- BreeferCP single sign-on ---
$cfg['Servers'][1]['auth_type'] = 'signon';
$cfg['Servers'][1]['SignonSession'] = 'BreeferPMA';
$cfg['Servers'][1]['SignonURL'] = 'breefercp-signon.php';
$cfg['Servers'][1]['host'] = 'localhost';
PMA
fi
# Roundcube talks to the local Dovecot/Postfix with the mailbox credentials.
cat > /etc/roundcube/config.inc.php.d-breefercp 2>/dev/null <<'RC' || true
<?php
$config['imap_host'] = 'ssl://localhost:993';
$config['smtp_host'] = 'tls://localhost:587';
$config['imap_conn_options'] = $config['smtp_conn_options'] = ['ssl' => ['verify_peer' => false, 'verify_peer_name' => false]];
$config['product_name'] = 'Webmail';
RC
grep -q breefercp /etc/roundcube/config.inc.php || echo "include '/etc/roundcube/config.inc.php.d-breefercp';" >> /etc/roundcube/config.inc.php
PHP_TOOLS="${PHP_VERSIONS[0]}"
cat > "/etc/php/${PHP_TOOLS}/fpm/pool.d/breefer-tools.conf" <<POOL
[breefer-tools]
user = www-data
group = www-data
listen = /run/php/breefer-tools.sock
listen.owner = www-data
listen.group = www-data
pm = ondemand
pm.max_children = 10
php_admin_value[upload_max_filesize] = 128M
php_admin_value[post_max_size] = 128M
POOL
systemctl reload "php${PHP_TOOLS}-fpm" || true
install -m 0644 "$SRC/deploy/nginx/tools.conf" /etc/nginx/conf.d/10-breefercp-tools.conf
nginx -t && systemctl reload nginx

log "Starting BreeferCP"
install -m 0644 "$SRC/deploy/breeferd.service" /etc/systemd/system/breeferd.service
systemctl daemon-reload
ADMIN_PASS="$(/usr/local/bin/breeferd create-admin -config /etc/breefercp/breefercp.conf \
  -username "$ADMIN_USER" -email "$ADMIN_EMAIL" | sed -n 's/^Password: //p')"
chgrp breefer /var/lib/breefercp/breefercp.db* 2>/dev/null || true
chmod 0660 /var/lib/breefercp/breefercp.db* 2>/dev/null || true

systemctl enable --now breeferd
systemctl enable --now postfix dovecot opendkim php*-fpm fail2ban 2>/dev/null || true
systemctl restart postfix dovecot opendkim

cat <<DONE

BreeferCP is installed.

  Panel:     https://${HOSTNAME_FQDN}:2087   (self-signed certificate until you replace it)
  Username:  ${ADMIN_USER}
  Password:  ${ADMIN_PASS:-<existing user, unchanged>}

Next steps:
  1. Set the server IP under Settings, then point your domain's nameservers at it.
  2. Create a package, then your first account.
  3. Sign in and enable two-factor authentication from Profile & Security.
DONE
