# Customer guide (cPanel role)

![Overview](../images/cp-overview.png)

The **Overview** shows your plan, status and how much of each quota you have used.

## Domains

![Domains](../images/cp-domains.png)

Add an **addon domain** (a separate site), a **subdomain**, or an **alias** (same content on another name). Choose the PHP version per domain. **Force HTTPS** turns on once the domain has a certificate. The primary domain cannot be removed.

## SSL/TLS

Click **Get free certificate** to issue a Let's Encrypt certificate for the domain and its `www` name. It renews automatically. The domain must already point at this server. To use your own certificate, choose **Upload** and paste the certificate chain and key; the panel checks that they match and cover the domain.

## File Manager

![File Manager](../images/cp-files.png)

- Drag files onto the list or use **Upload**.
- Click a file to edit it (text files up to 5 MB; **Ctrl+S** saves). Click a folder to open it.
- Hover a row for rename, copy, permissions, download and delete. Select several rows to compress them to a zip or delete them together. Click a `.zip` to extract it here.
- You can only reach your own home directory. Your site files go in `public_html`.

## Databases

Create a database, create a user, then use **Add user to database**. Names are prefixed with your account name (`acme_shop`). Connect from your site with host `localhost`.

## Email

![Email](../images/cp-email.png)

Create mailboxes with a quota, forwarders (use `*` as the name for a catch-all), and enable **DKIM** on the Deliverability tab, which also publishes a DMARC record. Mail clients: IMAP `mail.<yourdomain>` port 993 (SSL), SMTP port 587 (STARTTLS) or 465 (SSL), username is the full address.

## DNS Zone Editor

![DNS](../images/cp-dns.png)

Add, replace or delete A, AAAA, CNAME, MX, TXT, NS, SRV and CAA records. Saving a record with the same name and type replaces the existing one. SOA records are managed by the server.

## Applications

Run Node.js, Python 3 or a Docker image. Put your code in `apps/<name>`, give a start command (the `PORT` environment variable is set for you) and optionally attach a domain; the panel proxies it for you. Applications run inside your account's CPU and memory limits.

## Cron jobs, SSH keys, backups

- **Cron jobs** run as your user on a schedule (pick a preset or write the five-field expression).
- **SSH access:** authorize a public key; connect with `ssh <user>@<yourdomain>` (shell access depends on your plan).
- **Backups:** create a full or databases-only backup and restore files from it later. Backups run in the background.

## Keep your account safe

Under **Profile & Security** change your password, enable two-factor authentication, and sign out other sessions.

## Dark theme

Use the sun/moon button next to your name in the sidebar to switch between light, dark and system.

![Dark theme](../images/dark-files.png)

## App Installer

![App Installer](../images/cp-installer.png)

Pick an application, a domain and (optionally) a folder. A database and database user are created for you within your plan's limits.

- **WordPress** is installed fully automatically: enter a site title and admin account.
- **Drupal, Nextcloud, Matomo, Grav, DokuWiki** are downloaded from the official source; open the site afterwards to finish the setup wizard. The database name is shown under *Installed applications* and in **Databases**.
- **Laravel** creates a Composer project; point the domain's document root at its `public` folder.

## WordPress

![WordPress Toolkit](../images/cp-wordpress.png)

Sites installed here appear in the **WordPress Toolkit**, described in [its own section](#wordpress-toolkit) below.

## PHP Manager

![PHP Manager](../images/cp-php.png)

Choose the PHP version per domain from the versions your plan allows, and change common `php.ini` settings (memory limit, upload size, execution time, display errors, timezone and more). Security settings such as `disable_functions` and `open_basedir` stay with your provider.

## Redirects and error pages

- **Redirects:** send an exact path (for example `/old-page`) to any URL, permanently (301/308) or temporarily (302/307).
- **Error Pages:** write your own HTML for 400, 401, 403, 404, 500, 502 and 503 errors. A starter template is filled in for you; **Restore default** switches back.

## phpMyAdmin and webmail

- In **Databases**, click **phpMyAdmin**. You are signed in automatically with a temporary login that can see only your databases.
- In **Email → Webmail & clients**, open Roundcube or copy the IMAP/POP3/SMTP settings for your mail app.

## Terminal

If your plan includes shell access and your provider enabled it, **Terminal** opens a shell on the server as your user. Sessions close after 30 minutes idle or 4 hours.

## Disk usage and logs

**Disk Usage** shows which folders and databases use the most space. **Logs** shows the latest lines of each domain's access and error log and your PHP error log.

## Support

**Support** opens a ticket with your hosting provider. You get answers in the same thread, and can close or reopen the ticket.

## Signing in

Your provider gives you a cPanel address such as `https://server.example.com:2083`. The home page groups every tool; type in **Search tools** at the top (or press `/`) to jump to one. Tools your plan does not include are not shown. Notices from your provider appear at the top of the page.

## FTP Accounts

![FTP](../images/cp-ftp.png)

Create logins such as `upload@yourdomain.com` for designers or tools. Each is locked to one folder of your home directory and can have its own quota. Connect with FTPS (FTP with explicit TLS) on port 21; plain FTP is refused.

## Git Version Control

Clone a public https repository (GitHub, GitLab, Bitbucket…) into a folder, for example `public_html` to deploy a site, then press **Pull** to fetch new commits. Or create an empty repository and push to it over SSH using the address shown on its card.

## Autoresponders and spam filters

- **Autoresponders:** an automatic reply for one mailbox, optionally only between two dates. Each sender gets at most one reply per interval. Spam never gets a reply.
- **Spam Filters:** every message is scored. Turn filtering on, choose a threshold (5 suits most people), and move spam to the Junk folder or delete it. Senders on the **Always allow** list are never filtered; **Always block** senders are deleted on arrival.

## Remote MySQL

Add the address of a computer (or a range such as `203.0.113.%`) that should reach your databases directly. All your database users then accept connections from it with their usual password. Your provider must also open port 3306 in the firewall.

## Directory Privacy, IP Blocker and Hotlink Protection

- **Directory Privacy:** require a user name and password for a folder of your site, such as `/staging`. Add one or more users to each protected folder.
- **IP Blocker:** refuse an address or range (for example `203.0.113.0/24`) on all your sites.
- **Hotlink Protection:** stop other sites embedding your images and media. Your own domains are always allowed; add partner sites if needed.

## DNSSEC

At the bottom of the **Zone Editor**, switch on DNSSEC for a domain. The panel signs the zone and shows a **DS record**: add it at your domain registrar to complete the chain of trust. Switch off DNSSEC at the registrar first if you ever turn it off here.

## WordPress Toolkit

![WordPress security](../images/cp-wp-security.png)

The toolkit lists every WordPress site in your account. **Scan for sites** adds ones installed by hand or migrated in.

- **Overview:** WordPress version, **Smart update now**, automatic updates (core minor or all releases, plugins, themes), maintenance mode, search engine indexing and debug mode. Every update, manual or automatic, takes a backup first, checks the home page afterwards and **restores the backup automatically** if the site stopped working. The result of the last update is shown.
- **Plugins / Themes:** install from wordpress.org by slug (the last part of the plugin's address, such as `wordpress-seo`), update, activate, deactivate or delete.
- **Security:** a checklist with a **Fix** button for each fixable item (file editor, debug, XML-RPC and pingbacks, salts, `wp-config.php` permissions) and advice for the rest (modified core files, an `admin` user, pending updates).
- **Backups:** back up files and database and restore any backup; a safety backup is taken before each restore.
- **Clone & staging:** a **staging copy** on another domain or folder (create a subdomain such as `staging.yourdomain.com` first), with its own database and hidden from search engines. When it looks right, **Push to live** copies it over the live site after backing the live site up. **Clone** makes an independent copy.
