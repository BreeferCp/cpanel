package provision

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/breefercp/breefercp/internal/system"
)

// CatalogApp is one entry of the one-click installer (a Softaculous-style
// catalog). Archives are fetched from each project's official "latest" URL.
type CatalogApp struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Website     string `json:"website"`
	NeedsDB     bool   `json:"needs_db"`
	// Method: "wpcli" (fully automated), "archive" (download + extract, finish
	// setup in the browser) or "composer".
	Method  string `json:"method"`
	Archive string `json:"-"` // download URL for "archive"
	Package string `json:"-"` // composer package for "composer"
	// Webroot is the sub-directory that must be served (Laravel: public).
	Webroot string `json:"webroot,omitempty"`
}

// Catalog is the list of installable applications.
var Catalog = []CatalogApp{
	{ID: "wordpress", Name: "WordPress", Category: "Blogs & CMS", Method: "wpcli", NeedsDB: true,
		Description: "The most popular website and blogging platform.", Website: "https://wordpress.org"},
	{ID: "drupal", Name: "Drupal", Category: "Blogs & CMS", Method: "archive", NeedsDB: true,
		Archive:     "https://www.drupal.org/download-latest/tar.gz",
		Description: "Flexible CMS for complex, structured sites.", Website: "https://www.drupal.org"},
	{ID: "grav", Name: "Grav", Category: "Blogs & CMS", Method: "archive",
		Archive:     "https://getgrav.org/download/core/grav-admin/latest",
		Description: "Fast flat-file CMS with an admin panel, no database needed.", Website: "https://getgrav.org"},
	{ID: "nextcloud", Name: "Nextcloud", Category: "File sharing", Method: "archive", NeedsDB: true,
		Archive:     "https://download.nextcloud.com/server/releases/latest.zip",
		Description: "Self-hosted file sync, sharing and collaboration.", Website: "https://nextcloud.com"},
	{ID: "matomo", Name: "Matomo", Category: "Analytics", Method: "archive", NeedsDB: true,
		Archive:     "https://builds.matomo.org/matomo.zip",
		Description: "Privacy-friendly web analytics.", Website: "https://matomo.org"},
	{ID: "dokuwiki", Name: "DokuWiki", Category: "Wikis", Method: "archive",
		Archive:     "https://download.dokuwiki.org/src/dokuwiki/dokuwiki-stable.tgz",
		Description: "Simple wiki that stores pages as plain files.", Website: "https://www.dokuwiki.org"},
	{ID: "laravel", Name: "Laravel", Category: "Frameworks", Method: "composer", NeedsDB: true,
		Package: "laravel/laravel", Webroot: "public",
		Description: "PHP framework skeleton, ready for your code.", Website: "https://laravel.com"},
}

// CatalogByID finds a catalog entry.
func CatalogByID(id string) (CatalogApp, bool) {
	for _, a := range Catalog {
		if a.ID == id {
			return a, true
		}
	}
	return CatalogApp{}, false
}

// InstallSpec describes one installation.
type InstallSpec struct {
	App        CatalogApp
	Dir        string // absolute directory inside the account home
	URL        string
	Title      string
	AdminUser  string
	AdminPass  string
	AdminEmail string
	DBName     string
	DBUser     string
	DBPass     string
}

var reWPUser = regexp.MustCompile(`^[a-zA-Z0-9_.@-]{3,60}$`)

// asUser runs a command as the account user inside its home.
func (p *Provisioner) asUser(ctx context.Context, a Account, stdin []byte, name string, args ...string) (system.Result, error) {
	full := append([]string{"-u", a.SysUser, "--", name}, args...)
	return p.run.RunInput(ctx, stdin, "runuser", full...)
}

// Install performs an installation. It must be called with Dir already
// validated to be inside the account home and empty (or absent).
func (p *Provisioner) Install(ctx context.Context, a Account, s InstallSpec) error {
	if !strings.HasPrefix(s.Dir, a.Home+"/") {
		return fmt.Errorf("%w: install directory outside home", system.ErrInvalid)
	}
	if _, err := p.asUser(ctx, a, nil, "mkdir", "-p", "--", s.Dir); err != nil {
		return err
	}
	switch s.App.Method {
	case "wpcli":
		return p.installWordPress(ctx, a, s)
	case "archive":
		return p.installArchive(ctx, a, s)
	case "composer":
		_, err := p.asUser(ctx, a, nil, "composer", "create-project", "--no-interaction", "--prefer-dist", "--no-dev",
			s.App.Package, s.Dir)
		return err
	}
	return fmt.Errorf("unknown install method %q", s.App.Method)
}

func (p *Provisioner) installWordPress(ctx context.Context, a Account, s InstallSpec) error {
	if !reWPUser.MatchString(s.AdminUser) {
		return fmt.Errorf("%w: admin username", system.ErrInvalid)
	}
	if err := system.OneLine(s.Title, 120); err != nil {
		return err
	}
	wp := func(stdin string, args ...string) error {
		_, err := p.asUser(ctx, a, []byte(stdin), "wp", append([]string{"--path=" + s.Dir, "--no-color"}, args...)...)
		return err
	}
	if err := wp("", "core", "download", "--skip-content"); err != nil {
		return err
	}
	// Secrets are read from stdin (--prompt) so they never appear in `ps`.
	if err := wp(s.DBPass+"\n", "config", "create", "--dbname="+s.DBName, "--dbuser="+s.DBUser,
		"--dbhost=localhost", "--prompt=dbpass"); err != nil {
		return err
	}
	if err := wp(s.AdminPass+"\n", "core", "install", "--url="+s.URL, "--title="+s.Title,
		"--admin_user="+s.AdminUser, "--admin_email="+s.AdminEmail, "--skip-email", "--prompt=admin_password"); err != nil {
		return err
	}
	_ = wp("", "theme", "install", "twentytwentyfive", "--activate")
	return wp("", "config", "set", "DISALLOW_FILE_EDIT", "true", "--raw")
}

// installArchive downloads the official archive as the account user and
// unpacks it into Dir, dropping the archive's top-level folder. The script
// text is constant; all variable data is passed as positional parameters.
func (p *Provisioner) installArchive(ctx context.Context, a Account, s InstallSpec) error {
	const script = `set -eu
url="$1"; dir="$2"; tmp="$(mktemp -d "$HOME/tmp/install.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT
curl -fsSL --proto '=https' --max-filesize 1073741824 -o "$tmp/pkg" "$url"
mkdir -p "$tmp/x"
case "$(file -b --mime-type "$tmp/pkg")" in
  application/zip) unzip -q "$tmp/pkg" -d "$tmp/x" ;;
  *) tar -xf "$tmp/pkg" -C "$tmp/x" --no-same-owner ;;
esac
set -- "$tmp/x"/*
if [ "$#" -eq 1 ] && [ -d "$1" ]; then src="$1"; else src="$tmp/x"; fi
cp -a "$src"/. "$dir"/`
	_, err := p.asUser(ctx, a, nil, "sh", "-c", script, "breefer-install", s.App.Archive, s.Dir)
	return err
}

// Uninstall removes the installation directory.
func (p *Provisioner) Uninstall(ctx context.Context, a Account, dir string) error {
	if !strings.HasPrefix(dir, a.Home+"/") || path.Clean(dir) != dir {
		return fmt.Errorf("%w: directory outside home", system.ErrInvalid)
	}
	_, err := p.asUser(ctx, a, nil, "rm", "-rf", "--", dir)
	return err
}

// ---- WordPress manager --------------------------------------------------------

var reWPSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,99}$`)

// WPItem is a plugin or theme as reported by WP-CLI.
type WPItem struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Update  string `json:"update"`
	Version string `json:"version"`
	Title   string `json:"title,omitempty"`
}

// WPInfo summarises a WordPress installation.
type WPInfo struct {
	Version       string   `json:"version"`
	UpdateVersion string   `json:"update_version,omitempty"`
	Plugins       []WPItem `json:"plugins"`
	Themes        []WPItem `json:"themes"`
	Maintenance   bool     `json:"maintenance"`
}

func (p *Provisioner) wp(ctx context.Context, a Account, dir string, stdin []byte, args ...string) (string, error) {
	if !strings.HasPrefix(dir, a.Home+"/") {
		return "", fmt.Errorf("%w: directory outside home", system.ErrInvalid)
	}
	res, err := p.asUser(ctx, a, stdin, "wp", append([]string{"--path=" + dir, "--no-color", "--skip-themes"}, args...)...)
	return res.Stdout, err
}

// WPStatus collects version, update, plugin and theme information.
func (p *Provisioner) WPStatus(ctx context.Context, a Account, dir string) (WPInfo, error) {
	if p.run.DryRun {
		return WPInfo{Version: "6.8.3", UpdateVersion: "6.9", Plugins: []WPItem{
			{Name: "akismet", Title: "Akismet Anti-spam", Status: "active", Version: "5.3", Update: "available"},
			{Name: "classic-editor", Title: "Classic Editor", Status: "inactive", Version: "1.6.7", Update: "none"},
		}, Themes: []WPItem{{Name: "twentytwentyfive", Title: "Twenty Twenty-Five", Status: "active", Version: "1.3", Update: "none"}}}, nil
	}
	var info WPInfo
	out, err := p.wp(ctx, a, dir, nil, "core", "version")
	if err != nil {
		return info, err
	}
	info.Version = strings.TrimSpace(out)
	if out, err := p.wp(ctx, a, dir, nil, "core", "check-update", "--format=json", "--fields=version"); err == nil && strings.TrimSpace(out) != "" {
		var ups []struct{ Version string }
		if json.Unmarshal([]byte(out), &ups) == nil && len(ups) > 0 {
			info.UpdateVersion = ups[0].Version
		}
	}
	for _, kind := range []string{"plugin", "theme"} {
		out, err := p.wp(ctx, a, dir, nil, kind, "list", "--format=json", "--fields=name,title,status,update,version")
		if err != nil {
			return info, err
		}
		var items []WPItem
		if err := json.Unmarshal([]byte(out), &items); err != nil {
			return info, err
		}
		if kind == "plugin" {
			info.Plugins = items
		} else {
			info.Themes = items
		}
	}
	if _, err := os.Stat(path.Join(dir, ".maintenance")); err == nil {
		info.Maintenance = true
	}
	return info, nil
}

// WPAction runs one allow-listed management action.
func (p *Provisioner) WPAction(ctx context.Context, a Account, dir, action, target, secret string) error {
	if target != "" && !reWPSlug.MatchString(target) && !reWPUser.MatchString(target) {
		return fmt.Errorf("%w: plugin, theme or user name", system.ErrInvalid)
	}
	var args []string
	var stdin []byte
	switch action {
	case "core-update":
		args = []string{"core", "update"}
	case "core-update-db":
		args = []string{"core", "update-db"}
	case "plugins-update-all":
		args = []string{"plugin", "update", "--all"}
	case "themes-update-all":
		args = []string{"theme", "update", "--all"}
	case "plugin-update", "plugin-activate", "plugin-deactivate", "plugin-delete":
		if target == "" {
			return fmt.Errorf("%w: plugin name required", system.ErrInvalid)
		}
		args = []string{"plugin", strings.TrimPrefix(action, "plugin-"), target}
	case "theme-activate", "theme-update":
		if target == "" {
			return fmt.Errorf("%w: theme name required", system.ErrInvalid)
		}
		args = []string{"theme", strings.TrimPrefix(action, "theme-"), target}
	case "maintenance-on":
		args = []string{"maintenance-mode", "activate"}
	case "maintenance-off":
		args = []string{"maintenance-mode", "deactivate"}
	case "cache-flush":
		args = []string{"cache", "flush"}
	case "reset-password":
		if target == "" || len(secret) < 12 {
			return fmt.Errorf("%w: user and a 12+ character password are required", system.ErrInvalid)
		}
		args = []string{"user", "update", target, "--prompt=user_pass"}
		stdin = []byte(secret + "\n")
	default:
		return fmt.Errorf("%w: unknown action %q", system.ErrInvalid, action)
	}
	_, err := p.wp(ctx, a, dir, stdin, args...)
	return err
}
