package provision

import (
	"context"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/breefercp/breefercp/internal/system"
)

// Everything below an account's home is controlled by the customer, symlinks
// included, and the panel runs as root. Work on those paths is done as the
// account, so the kernel refuses whatever the customer could not do.

// logBase holds web server logs. nginx and OpenLiteSpeed open logs as root,
// so the directory must not be writable (or replaceable) by the customer.
const logBase = "/var/log/breefercp/sites"

// LogDir is where an account's web server logs live.
func LogDir(a Account) string { return path.Join(logBase, a.SysUser) }

var reConfigPath = regexp.MustCompile(`^[A-Za-z0-9/._:~+@-]+$`)

// ConfigPath reports whether a path can be written into a config file
// without being read as syntax, a variable or a comment.
func ConfigPath(p string) bool { return reConfigPath.MatchString(p) }

func (p *Provisioner) ensureLogDir(ctx context.Context, a Account) error {
	if err := system.SysUser(a.SysUser); err != nil {
		return err
	}
	if _, err := p.run.Run(ctx, "install", "-d", "-m", "0755", logBase); err != nil {
		return err
	}
	_, err := p.run.Run(ctx, "install", "-d", "-m", "0750", "-o", "root", "-g", p.cfg.WebUser, LogDir(a))
	return err
}

// MkdirAsUser creates a directory in the home with the account's own rights.
func (p *Provisioner) MkdirAsUser(ctx context.Context, a Account, dir, mode string) error {
	_, err := p.asUser(ctx, a, nil, "install", "-d", "-m", mode, "--", filepath.ToSlash(dir))
	return err
}

// PrepareDocRoot creates a document root and lets nginx read what is created
// in it. The default ACL matters: files made by the panel, git or WP-CLI are
// 0640 or 0660, and the web server is not in the customer's group. A file the
// customer makes 0600 stays private, because the mode caps the ACL.
func (p *Provisioner) PrepareDocRoot(ctx context.Context, a Account, dir string) error {
	if err := p.MkdirAsUser(ctx, a, dir, "0755"); err != nil {
		return err
	}
	home, dir := filepath.ToSlash(a.Home), filepath.ToSlash(dir)
	web := "u:" + p.cfg.WebUser + ":r-x"
	for parent := path.Dir(dir); strings.HasPrefix(parent, home+"/"); parent = path.Dir(parent) {
		if _, err := p.asUser(ctx, a, nil, "setfacl", "-m", web, "--", parent); err != nil {
			return err
		}
	}
	_, err := p.asUser(ctx, a, nil, "setfacl", "-m", web+",d:"+web, "--", dir)
	return err
}

// WriteAuthorizedKeys replaces ~/.ssh/authorized_keys as the account.
func (p *Provisioner) WriteAuthorizedKeys(ctx context.Context, a Account, keys string) error {
	dir := path.Join(filepath.ToSlash(a.Home), ".ssh")
	if err := p.MkdirAsUser(ctx, a, dir, "0700"); err != nil {
		return err
	}
	file := path.Join(dir, "authorized_keys")
	if _, err := p.asUser(ctx, a, []byte(keys), "tee", "--", file); err != nil {
		return err
	}
	_, err := p.asUser(ctx, a, nil, "chmod", "0600", "--", file)
	return err
}

// ValidAppName reports whether an app name is safe in paths and unit names.
func ValidAppName(s string) bool { return reAppName.MatchString(s) }
