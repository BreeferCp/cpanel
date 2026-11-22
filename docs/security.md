# Security model

BreeferCP manages root-level system state on internet-facing servers, so its defaults are conservative. This page lists what is enforced today and, as importantly, what is not.

## Authentication

- Passwords: Argon2id (m=64 MiB, t=3, p=2), 12-character minimum with character-variety check. Verification refuses attacker-supplied absurd cost parameters.
- Unknown usernames run a dummy hash so response time does not reveal whether an account exists.
- Rate limiting per IP and per username, plus a 15-minute lockout after 5 consecutive failures.
- TOTP (RFC 6238) with replay protection per time step and eight single-use recovery codes stored only as hashes.
- Sessions are opaque 256-bit tokens stored hashed; cookie is `HttpOnly`, `SameSite=Strict`, and `Secure` over TLS. The pending-2FA state and the full session use different tokens (no session fixation). Changing a password revokes all other sessions.
- Cookie sessions require a per-session CSRF token header on every unsafe request.
- API tokens: `bcp_`-prefixed, stored hashed, scoped (`billing`, `accounts:write`, …), optionally IP-restricted and expiring. A token cannot create tokens.
- Single sign-on tokens are HMAC-signed, expire in 60 seconds, are single-use, and travel in the URL fragment so they never reach server logs or `Referer`.

## Authorisation

Role matrix in `internal/rbac`; tenancy in one function, `loadAccount` (see [architecture](architecture.md#tenancy)). Cross-tenant access returns 404, not 403, so existence is not disclosed. Integration tests assert this for customers and the billing token.

## Injection resistance

Every value reaching an nginx template, PHP-FPM pool, systemd unit, crontab, nftables rule, DNS record, MariaDB statement or command line is validated against a strict allow-list first. SQL identifiers are validated then quoted; values are always bound parameters. Tests feed newline, `;` and `}` payloads to the nginx, cron, firewall and validator code paths; DNS and MariaDB inputs are covered by the validators but have no dedicated injection tests yet.

## File manager

Every path is resolved with Go's `os.Root`, which refuses `..` and symlink escapes at the kernel level instead of checking strings first (so there is no check-then-use race). Archives are extracted with entry-count and uncompressed-size caps, symlink entries are skipped, and setuid/setgid/sticky bits can never be set. Downloads are always `attachment` with a sandboxing CSP, which prevents stored XSS from uploaded HTML or SVG on the panel origin.

## Features added in 0.2

- **Browser terminal:** a WebSocket bridged to a PTY. Requires the account shell mode *full*, which in turn requires the package to allow shell access. The shell runs as the account user inside its cgroup slice. The WebSocket library rejects cross-origin upgrades, which together with the SameSite=Strict cookie prevents cross-site WebSocket hijacking. Sessions end after 30 minutes idle or 4 hours.
- **phpMyAdmin SSO:** the panel creates a temporary MariaDB user that can reach only that account's databases. The one-time token is redeemable only from loopback, only once and only within two minutes; the stored password is erased on redemption and the database user is dropped after two hours.
- **App installer and WordPress manager:** every command runs as the account user via `runuser`. Passwords reach WP-CLI on stdin (`--prompt`), never on the command line. WP-CLI actions are an allow-list, and plugin, theme and user names are validated. The archive installer runs a constant script, with URLs and paths passed as positional parameters.
- **php.ini overrides:** an allow-list of keys, each with a strict value pattern. Directives that affect isolation (`disable_functions`, `open_basedir`, `allow_url_include`) cannot be overridden.
- **Redirects:** paths and targets admit neither `;` nor `$`, quotes, braces or whitespace, so they cannot inject nginx directives or variables (covered by tests).
- **Reseller hierarchy:** tenancy includes the owner chain (a master reseller sees its resellers' customers). Integration tests assert that an unrelated reseller gets 404 and that allocation limits are enforced.
- **Login rate limiting** counts only failed attempts, so busy offices behind one IP are not locked out by successful sign-ins.

## Features added in 0.3

- **Dedicated panels:** WHM (2087) and cPanel (2083) are separate listeners with separate cookies (`breefer_whm`, `breefer_cpanel`). A session or API token is ignored on the panel its role does not belong to, so a stolen customer session cannot reach WHM endpoints even by URL. The wrong-panel message is given only after a correct password, so it reveals nothing to a guesser. Tests cover both directions.
- **Feature lists** are enforced by a middleware on every `/accounts/{aid}/…` route for account holders, not only hidden in the UI.
- **Security policy:** optional mandatory 2FA (sessions without it can reach only their profile), a WHM IP allow list (enforced on sign-in and on every request; it cannot be saved without the caller's own address), and host access rules (deny lists, plus allow lists exempt from rate limiting; you cannot deny your own address).
- **nginx rules from customers:** IP blocks are parsed and re-serialised as canonical CIDRs, ranges wider than /8 are refused, and protected-directory paths, realms and hotlink hosts and extensions pass strict allow-lists. Password files live in `/etc/nginx/breefercp-htpasswd/<user>/`, outside every home, so a customer can neither read other hashes nor point nginx at arbitrary files. Injection tests cover all four.
- **Password hashes for external programs** (nginx basic auth, Pure-FTPd) use SHA-512 crypt, tested against the reference vectors. Panel logins keep Argon2id.
- **FTP** users are virtual, mapped to the account's uid, chrooted to a folder that passes a relative-path allow-list, with TLS required. The Pure-FTPd database is regenerated from the panel database; suspended accounts drop out of it.
- **Sieve scripts** (spam filters, autoresponders) are rendered with RFC 5228 string escaping and written through the home-directory jail, so a planted symlink cannot redirect the write.
- **Git** clones accept public `https://` URLs only (no credentials, `file://`, `ssh://` or `ext::`), with `protocol.allow=never` except https, and run as the account user.
- **Remote MySQL** host patterns pass an allow-list (IP, host name, `a.b.c.%`); `localhost` cannot be re-added.
- **WordPress toolkit:** every WP-CLI call runs as the account user. Database dumps and imports run as root but only against the installation's own validated database names, and dump files stay in root-owned directories. Restores unpack the archive as the account user. Clone and push refuse a destination inside its source (and the reverse). Smart updates back up first and roll back if the home page starts failing.
- **Self-update:** releases install only when the tarball's SHA-256 matches the release's `SHA256SUMS`. The archive is extracted with path-traversal, link and size checks, and the previous binary and UI are kept for rollback. The checksum and the tarball come from the same GitHub release, so this protects against corrupted and tampered downloads in transit, not against a compromised GitHub account; signed releases are planned.

## Features added in 0.4

- **Email filters** are stored as structured rules (field, comparison, value, action) and rendered to Sieve with RFC 5228 escaping; folder names and redirect targets pass allow-lists, and `:matches` wildcards in values are escaped. Injection tests cover both.
- **Mailing lists, domain forwarders and email routing** are read by Postfix through SQLite maps with fixed queries; members and targets are validated addresses and domains.
- **Directory indexes, MIME types and compression** become nginx directives only after validation: index paths use the protected-directory allow-list, script extensions (`.php`, `.phar`, `.cgi` and similar) cannot be remapped, and MIME types must match `type/subtype`. An index on or inside a protected folder carries that folder's password, and dotfiles are refused inside both kinds of location.
- **Dynamic DNS** update URLs carry a 256-bit token stored only as a hash. A token changes one A/AAAA record and nothing else, is refused for suspended accounts, and failures are rate limited.
- **WHM server tools:** changing the root password requires the caller's panel password again; SSH settings are validated with `sshd -t` and rolled back on error, and a change, or a key deletion, is refused when it would leave neither root nor a sudo user a way to log in; root keys with options (`command=`) are refused; server-wide domain and PHP views apply reseller tenancy.
- **Track Delivery** only returns log lines whose sender or recipient is one of the caller's domains.

## Transport and headers

TLS 1.2+ on the panel and all generated vhosts. The API sends `Content-Security-Policy` (`script-src 'self'`, no inline scripts), `X-Frame-Options: DENY`, `nosniff`, a strict referrer policy and HSTS over TLS.

## Operational

- `breeferd` runs as root by design (it must create users and write system config). The systemd unit applies the hardening that is compatible with that.
- Everything security-relevant is written to the audit log (sign-ins, failures, token and 2FA changes, account/domain/file/DNS operations) and kept for a year.
- The panel database holds password hashes and TOTP secrets. TOTP secrets are stored unencrypted in this release; encrypting them at rest with the instance key is on the roadmap. Protect `/var/lib/breefercp` accordingly.

## Not done yet

- No third-party security audit. Treat this as pre-release software.
- No ModSecurity/WAF, malware scanning, or per-account outbound mail rate limiting.
- No WebAuthn/passkeys.
- No automatic brute-force banning at the firewall level (fail2ban is installed but has no BreeferCP jail yet).

Report vulnerabilities privately as described in [SECURITY.md](../SECURITY.md).
