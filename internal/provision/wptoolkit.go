package provision

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/breefercp/breefercp/internal/system"
)

// This file adds WP Toolkit / Softaculous-style management on top of the
// basic WordPress manager: discovery, security checks with fixes, backups,
// cloning and staging, and site settings. Every WordPress command runs as
// the account user through WP-CLI; database dumps and imports run as root
// against the account's own database only.

// WPSite is extra information about an installation.
type WPSite struct {
	SiteURL    string   `json:"site_url"`
	Indexing   bool     `json:"indexing"`
	Debug      bool     `json:"debug"`
	Admins     []string `json:"admins"`
	DBName     string   `json:"db_name"`
	DBUser     string   `json:"-"`
	TablePrefx string   `json:"table_prefix"`
}

// WPCheck is one security check.
type WPCheck struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	OK      bool   `json:"ok"`
	Fixable bool   `json:"fixable"`
	Detail  string `json:"detail,omitempty"`
}

var reSiteURL = regexp.MustCompile(`^https?://[a-z0-9.-]+(:[0-9]{1,5})?(/[A-Za-z0-9._~/-]*)?$`)

// SiteURL validates a WordPress site URL used in search-replace.
func SiteURL(s string) error {
	if !reSiteURL.MatchString(s) {
		return fmt.Errorf("%w: site URL", system.ErrInvalid)
	}
	return nil
}

func (p *Provisioner) wpOut(ctx context.Context, a Account, dir string, args ...string) string {
	out, err := p.wp(ctx, a, dir, nil, args...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// WPDetect finds WordPress installations in the account home (Softaculous
// "Scan" / WP Toolkit "Scan for sites").
func (p *Provisioner) WPDetect(ctx context.Context, a Account) ([]string, error) {
	if p.run.DryRun {
		return []string{path.Join(a.Home, "public_html", "blog")}, nil
	}
	res, err := p.asUser(ctx, a, nil, "find", a.Home, "-maxdepth", "5", "-name", "wp-config.php", "-type", "f",
		"-not", "-path", "*/.trash/*", "-not", "-path", a.Home+"/tmp/*")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(res.Stdout, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		dir := path.Dir(l)
		if strings.HasPrefix(dir, a.Home+"/") && path.Clean(dir) == dir {
			out = append(out, dir)
		}
	}
	return out, nil
}

// WPSiteInfo reads URL, indexing, debug and administrator accounts.
func (p *Provisioner) WPSiteInfo(ctx context.Context, a Account, dir string) WPSite {
	if p.run.DryRun {
		return WPSite{SiteURL: "https://acme-shop.com", Indexing: true, Admins: []string{"acmeadmin"}, DBName: "acme_wp1a2b3c", TablePrefx: "wp_"}
	}
	s := WPSite{SiteURL: p.wpOut(ctx, a, dir, "option", "get", "siteurl")}
	s.Indexing = p.wpOut(ctx, a, dir, "option", "get", "blog_public") == "1"
	s.Debug = p.wpOut(ctx, a, dir, "config", "get", "WP_DEBUG") == "1"
	s.DBName = p.wpOut(ctx, a, dir, "config", "get", "DB_NAME")
	s.DBUser = p.wpOut(ctx, a, dir, "config", "get", "DB_USER")
	s.TablePrefx = p.wpOut(ctx, a, dir, "config", "get", "table_prefix")
	var admins []struct {
		Login string `json:"user_login"`
	}
	if out := p.wpOut(ctx, a, dir, "user", "list", "--role=administrator", "--fields=user_login", "--format=json"); out != "" {
		_ = json.Unmarshal([]byte(out), &admins)
	}
	for _, u := range admins {
		s.Admins = append(s.Admins, u.Login)
	}
	return s
}

// hardeningPlugin is a must-use plugin applied by the "hardening" fix:
// it disables XML-RPC and pingbacks and hides the WordPress version.
const hardeningPlugin = `<?php
/**
 * Plugin Name: BreeferCP hardening
 * Description: Managed by BreeferCP. Disables XML-RPC and pingbacks and hides the WordPress version.
 */
add_filter('xmlrpc_enabled', '__return_false');
add_filter('xmlrpc_methods', function ($m) { unset($m['pingback.ping'], $m['pingback.extensions.getPingbacks']); return $m; });
add_filter('wp_headers', function ($h) { unset($h['X-Pingback']); return $h; });
remove_action('wp_head', 'wp_generator');
add_filter('the_generator', '__return_empty_string');
`

// WPSecurity runs the security checks.
func (p *Provisioner) WPSecurity(ctx context.Context, a Account, dir string) []WPCheck {
	if p.run.DryRun {
		return []WPCheck{
			{ID: "checksums", Label: "WordPress core files match the official release", OK: true},
			{ID: "file_edit", Label: "Theme and plugin editor is disabled", OK: true, Fixable: true},
			{ID: "debug", Label: "Debug output is off", OK: true, Fixable: true},
			{ID: "hardening", Label: "XML-RPC, pingbacks and version disclosure are blocked", OK: false, Fixable: true},
			{ID: "salts", Label: "Security keys and salts are set", OK: true, Fixable: true},
			{ID: "config_perms", Label: "wp-config.php is readable only by your account", OK: false, Fixable: true, Detail: "mode 0644"},
			{ID: "admin_user", Label: "No administrator is called \"admin\"", OK: true},
			{ID: "updates", Label: "WordPress core is up to date", OK: false, Detail: "6.9 is available"},
		}
	}
	checks := []WPCheck{}
	add := func(id, label string, ok, fixable bool, detail string) {
		checks = append(checks, WPCheck{ID: id, Label: label, OK: ok, Fixable: fixable, Detail: detail})
	}
	_, err := p.wp(ctx, a, dir, nil, "core", "verify-checksums")
	add("checksums", "WordPress core files match the official release", err == nil, false,
		map[bool]string{true: "", false: "modified or unknown core files found; reinstall core or restore a backup"}[err == nil])
	add("file_edit", "Theme and plugin editor is disabled", p.wpOut(ctx, a, dir, "config", "get", "DISALLOW_FILE_EDIT") == "1", true, "")
	add("debug", "Debug output is off", p.wpOut(ctx, a, dir, "config", "get", "WP_DEBUG") != "1", true, "")
	_, herr := p.asUser(ctx, a, nil, "test", "-f", path.Join(dir, "wp-content", "mu-plugins", "breefercp-hardening.php"))
	add("hardening", "XML-RPC, pingbacks and version disclosure are blocked", herr == nil, true, "")
	salt := p.wpOut(ctx, a, dir, "config", "get", "AUTH_KEY")
	add("salts", "Security keys and salts are set", len(salt) >= 32 && !strings.Contains(salt, "put your unique phrase"), true, "")
	mode := ""
	if r, err := p.asUser(ctx, a, nil, "stat", "-c", "%a", path.Join(dir, "wp-config.php")); err == nil {
		mode = strings.TrimSpace(r.Stdout)
	}
	add("config_perms", "wp-config.php is readable only by your account", mode == "600" || mode == "400" || mode == "640" || mode == "440", true,
		"mode "+mode)
	admins := p.WPSiteInfo(ctx, a, dir).Admins
	hasAdmin := false
	for _, u := range admins {
		hasAdmin = hasAdmin || strings.EqualFold(u, "admin")
	}
	add("admin_user", `No administrator is called "admin"`, !hasAdmin, false,
		map[bool]string{true: "create a new administrator and delete the \"admin\" user", false: ""}[hasAdmin])
	up := p.wpOut(ctx, a, dir, "core", "check-update", "--field=version")
	add("updates", "WordPress core is up to date", up == "", false, map[bool]string{true: "", false: up + " is available"}[up == ""])
	return checks
}

// WPFix applies the fix for one fixable check.
func (p *Provisioner) WPFix(ctx context.Context, a Account, dir, id string) error {
	switch id {
	case "file_edit":
		_, err := p.wp(ctx, a, dir, nil, "config", "set", "DISALLOW_FILE_EDIT", "true", "--raw")
		return err
	case "debug":
		_, err := p.wp(ctx, a, dir, nil, "config", "set", "WP_DEBUG", "false", "--raw")
		return err
	case "salts":
		_, err := p.wp(ctx, a, dir, nil, "config", "shuffle-salts")
		return err
	case "config_perms":
		_, err := p.asUser(ctx, a, nil, "chmod", "0600", "--", path.Join(dir, "wp-config.php"))
		return err
	case "hardening":
		mu := path.Join(dir, "wp-content", "mu-plugins")
		if _, err := p.asUser(ctx, a, nil, "mkdir", "-p", "--", mu); err != nil {
			return err
		}
		_, err := p.asUser(ctx, a, []byte(hardeningPlugin), "sh", "-c", `cat > "$1"`, "breefer", path.Join(mu, "breefercp-hardening.php"))
		return err
	}
	return fmt.Errorf("%w: unknown security fix", system.ErrInvalid)
}

// WPSetting toggles a site setting.
func (p *Provisioner) WPSetting(ctx context.Context, a Account, dir, name string, on bool) error {
	v := map[bool]string{true: "true", false: "false"}[on]
	switch name {
	case "debug":
		if _, err := p.wp(ctx, a, dir, nil, "config", "set", "WP_DEBUG", v, "--raw"); err != nil {
			return err
		}
		_, err := p.wp(ctx, a, dir, nil, "config", "set", "WP_DEBUG_LOG", v, "--raw")
		return err
	case "indexing":
		_, err := p.wp(ctx, a, dir, nil, "option", "update", "blog_public", map[bool]string{true: "1", false: "0"}[on])
		return err
	}
	return fmt.Errorf("%w: unknown setting", system.ErrInvalid)
}

// WPInstallItem installs a plugin or theme from wordpress.org by slug.
func (p *Provisioner) WPInstallItem(ctx context.Context, a Account, dir, kind, slug string, activate bool) error {
	if kind != "plugin" && kind != "theme" {
		return fmt.Errorf("%w: kind", system.ErrInvalid)
	}
	if !reWPSlug.MatchString(slug) {
		return fmt.Errorf("%w: use the slug from wordpress.org, such as wordpress-seo", system.ErrInvalid)
	}
	args := []string{kind, "install", slug}
	if activate {
		args = append(args, "--activate")
	}
	_, err := p.wp(ctx, a, dir, nil, args...)
	return err
}

// ---- backups, cloning and staging --------------------------------------------------

// WPBackup writes files.tar.gz and db.sql for an installation into dest
// (a root-owned directory outside the home).
func (p *Provisioner) WPBackup(ctx context.Context, a Account, dir, db, dest string) error {
	if !strings.HasPrefix(dir, a.Home+"/") || path.Clean(dir) != dir {
		return fmt.Errorf("%w: directory outside home", system.ErrInvalid)
	}
	if _, err := p.run.Run(ctx, "install", "-d", "-m", "0700", dest); err != nil {
		return err
	}
	if _, err := p.run.Run(ctx, "tar", "-czf", filepath.Join(dest, "files.tar.gz"), "-C", dir, "."); err != nil {
		return err
	}
	if db != "" {
		if err := system.DBName(db); err != nil {
			return err
		}
		if err := p.run.RunToFile(ctx, filepath.Join(dest, "db.sql"), "mariadb-dump", "--single-transaction", "--routines", "--", db); err != nil {
			return err
		}
	}
	return nil
}

// WPRestore replaces the installation's files and database with a backup.
// Files are unpacked by the account user (so archive symlinks cannot reach
// outside the home); the database import goes only into db.
func (p *Provisioner) WPRestore(ctx context.Context, a Account, dir, db, src string, keepConfig bool) error {
	if !strings.HasPrefix(dir, a.Home+"/") || path.Clean(dir) != dir {
		return fmt.Errorf("%w: directory outside home", system.ErrInvalid)
	}
	cfg := path.Join(dir, "wp-config.php")
	saved := ""
	if keepConfig {
		if res, err := p.asUser(ctx, a, nil, "cat", "--", cfg); err == nil {
			saved = res.Stdout
		}
	}
	if _, err := p.asUser(ctx, a, nil, "find", dir, "-mindepth", "1", "-delete"); err != nil {
		return err
	}
	if err := p.run.RunWithFileInput(ctx, filepath.Join(src, "files.tar.gz"), "runuser", "-u", a.SysUser, "--",
		"tar", "-xzf", "-", "-C", dir, "--no-same-owner", "--no-same-permissions"); err != nil {
		return err
	}
	if keepConfig && saved != "" {
		if _, err := p.asUser(ctx, a, []byte(saved), "sh", "-c", `cat > "$1"`, "breefer", cfg); err != nil {
			return err
		}
	}
	if db != "" {
		if err := system.DBName(db); err != nil {
			return err
		}
		if err := p.run.RunWithFileInput(ctx, filepath.Join(src, "db.sql"), "mariadb", "--", db); err != nil {
			return err
		}
	}
	return nil
}

// WPCloneSpec describes a clone of src into dst.
type WPCloneSpec struct {
	SrcDir, SrcDB, SrcURL string
	DstDir, DstURL        string
	DstDB, DstUser        string
	DstPass               string
	Stage                 string // root-owned scratch directory
	NoIndex               bool   // staging copies hide from search engines
}

// WPClone copies files and database of an installation to a new location
// and rewrites URLs in the copy (WP Toolkit "Clone", Softaculous "Staging").
// The destination database and user must already exist.
func (p *Provisioner) WPClone(ctx context.Context, a Account, c WPCloneSpec) error {
	for _, d := range []string{c.SrcDir, c.DstDir} {
		if !strings.HasPrefix(d, a.Home+"/") || path.Clean(d) != d {
			return fmt.Errorf("%w: directory outside home", system.ErrInvalid)
		}
	}
	if strings.HasPrefix(c.DstDir+"/", c.SrcDir+"/") || strings.HasPrefix(c.SrcDir+"/", c.DstDir+"/") {
		return fmt.Errorf("%w: the copy cannot be inside the original (or the other way round)", system.ErrInvalid)
	}
	if err := SiteURL(c.SrcURL); err != nil {
		return err
	}
	if err := SiteURL(c.DstURL); err != nil {
		return err
	}
	if _, err := p.asUser(ctx, a, nil, "mkdir", "-p", "--", c.DstDir); err != nil {
		return err
	}
	if _, err := p.asUser(ctx, a, nil, "cp", "-a", "--", c.SrcDir+"/.", c.DstDir+"/"); err != nil {
		return err
	}
	if _, err := p.run.Run(ctx, "install", "-d", "-m", "0700", c.Stage); err != nil {
		return err
	}
	defer p.run.Run(context.WithoutCancel(ctx), "rm", "-rf", "--", c.Stage) //nolint:errcheck
	dump := filepath.Join(c.Stage, "db.sql")
	if err := system.DBName(c.SrcDB); err != nil {
		return err
	}
	if err := system.DBName(c.DstDB); err != nil {
		return err
	}
	if err := p.run.RunToFile(ctx, dump, "mariadb-dump", "--single-transaction", "--routines", "--", c.SrcDB); err != nil {
		return err
	}
	if err := p.run.RunWithFileInput(ctx, dump, "mariadb", "--", c.DstDB); err != nil {
		return err
	}
	wp := func(stdin string, args ...string) error {
		_, err := p.wp(ctx, a, c.DstDir, []byte(stdin), args...)
		return err
	}
	if err := wp("", "config", "set", "DB_NAME", c.DstDB); err != nil {
		return err
	}
	if err := wp("", "config", "set", "DB_USER", c.DstUser); err != nil {
		return err
	}
	if err := wp(c.DstPass+"\n", "config", "set", "DB_PASSWORD", "--prompt=value"); err != nil {
		return err
	}
	if c.SrcURL != c.DstURL {
		if err := wp("", "search-replace", c.SrcURL, c.DstURL, "--all-tables-with-prefix", "--skip-columns=guid", "--precise"); err != nil {
			return err
		}
	}
	if c.NoIndex {
		_ = wp("", "option", "update", "blog_public", "0")
	}
	return wp("", "cache", "flush")
}

// WPPush copies a staging site over its live site, keeping the live
// wp-config.php (and so the live database credentials).
func (p *Provisioner) WPPush(ctx context.Context, a Account, c WPCloneSpec) error {
	for _, d := range []string{c.SrcDir, c.DstDir} {
		if !strings.HasPrefix(d, a.Home+"/") || path.Clean(d) != d {
			return fmt.Errorf("%w: directory outside home", system.ErrInvalid)
		}
	}
	if err := SiteURL(c.SrcURL); err != nil {
		return err
	}
	if err := SiteURL(c.DstURL); err != nil {
		return err
	}
	const script = `set -eu
src="$1"; dst="$2"; tmp="$(mktemp)"
cp -p "$dst/wp-config.php" "$tmp"
find "$dst" -mindepth 1 -maxdepth 1 ! -name wp-config.php -exec rm -rf -- {} +
cp -a "$src"/. "$dst"/
cp -p "$tmp" "$dst/wp-config.php"; rm -f "$tmp"`
	if _, err := p.asUser(ctx, a, nil, "sh", "-c", script, "breefer-push", c.SrcDir, c.DstDir); err != nil {
		return err
	}
	if _, err := p.run.Run(ctx, "install", "-d", "-m", "0700", c.Stage); err != nil {
		return err
	}
	defer p.run.Run(context.WithoutCancel(ctx), "rm", "-rf", "--", c.Stage) //nolint:errcheck
	dump := filepath.Join(c.Stage, "db.sql")
	if err := system.DBName(c.SrcDB); err != nil {
		return err
	}
	if err := system.DBName(c.DstDB); err != nil {
		return err
	}
	if err := p.run.RunToFile(ctx, dump, "mariadb-dump", "--single-transaction", "--routines", "--", c.SrcDB); err != nil {
		return err
	}
	if err := p.run.RunWithFileInput(ctx, dump, "mariadb", "--", c.DstDB); err != nil {
		return err
	}
	if _, err := p.wp(ctx, a, c.DstDir, nil, "search-replace", c.SrcURL, c.DstURL, "--all-tables-with-prefix", "--skip-columns=guid", "--precise"); err != nil {
		return err
	}
	_, _ = p.wp(ctx, a, c.DstDir, nil, "option", "update", "blog_public", "1")
	_, err := p.wp(ctx, a, c.DstDir, nil, "cache", "flush")
	return err
}

// WPAutoUpdate runs the updates allowed by a policy and returns a summary.
// core is off, minor or major.
func (p *Provisioner) WPAutoUpdate(ctx context.Context, a Account, dir, core string, plugins, themes bool) (string, error) {
	var done []string
	switch core {
	case "minor":
		if _, err := p.wp(ctx, a, dir, nil, "core", "update", "--minor"); err != nil {
			return "", err
		}
		done = append(done, "core (minor)")
	case "major":
		if _, err := p.wp(ctx, a, dir, nil, "core", "update"); err != nil {
			return "", err
		}
		done = append(done, "core")
	}
	if core != "off" {
		_, _ = p.wp(ctx, a, dir, nil, "core", "update-db")
	}
	if plugins {
		if _, err := p.wp(ctx, a, dir, nil, "plugin", "update", "--all"); err != nil {
			return "", err
		}
		done = append(done, "plugins")
	}
	if themes {
		if _, err := p.wp(ctx, a, dir, nil, "theme", "update", "--all"); err != nil {
			return "", err
		}
		done = append(done, "themes")
	}
	return strings.Join(done, ", "), nil
}
