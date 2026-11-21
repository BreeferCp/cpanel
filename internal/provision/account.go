package provision

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/breefercp/breefercp/internal/system"
)

// CreateAccount creates the Linux user, its directory layout and its
// resource slice. UID allocation is the caller's job (stored in the DB).
func (p *Provisioner) CreateAccount(ctx context.Context, a Account) error {
	if err := system.SysUser(a.SysUser); err != nil {
		return err
	}
	uid := strconv.Itoa(a.UID)
	// No login shell by default; shell access is granted explicitly later.
	if _, err := p.run.Run(ctx, "useradd",
		"--uid", uid, "--user-group", "--home-dir", a.Home, "--create-home",
		"--shell", "/usr/sbin/nologin", "--comment", "BreeferCP account", a.SysUser); err != nil {
		return err
	}
	for _, d := range []string{"public_html", "logs", "tmp", "mail", ".ssh", "backups", "apps"} {
		dir := filepath.Join(a.Home, d)
		if _, err := p.run.Run(ctx, "install", "-d", "-m", dirMode(d), "-o", a.SysUser, "-g", a.SysUser, dir); err != nil {
			return err
		}
	}
	// The home directory is closed to every other account (0700). The web
	// server user gets traverse-only access through an ACL, which scales to
	// any number of accounts (group membership would not).
	if _, err := p.run.Run(ctx, "chmod", "0700", a.Home); err != nil {
		return err
	}
	if _, err := p.run.Run(ctx, "setfacl", "-m", "u:"+p.cfg.WebUser+":--x", a.Home); err != nil {
		return err
	}
	if err := p.SetLimits(ctx, a); err != nil {
		return err
	}
	return p.ApplyPHPPool(ctx, a)
}

// dirMode returns the mode of a standard home subdirectory. Only public_html is
// world-traversable, and it is still unreachable by other accounts because
// the home directory above it is 0700.
func dirMode(name string) string {
	switch name {
	case "public_html":
		return "0755"
	case "logs", "apps":
		return "0750"
	default:
		return "0700"
	}
}

// SetLimits writes a systemd slice for the account and applies it live.
// Processes of the account are placed in the slice by PHP-FPM (Slice= in the
// pool service drop-in) and by sshd's pam_systemd.
func (p *Provisioner) SetLimits(ctx context.Context, a Account) error {
	unit := fmt.Sprintf("user-%d.slice.d", a.UID)
	dropin := fmt.Sprintf(`# Managed by BreeferCP. Do not edit.
[Slice]
CPUAccounting=yes
MemoryAccounting=yes
IOAccounting=yes
TasksAccounting=yes
CPUQuota=%d%%
MemoryMax=%dM
MemoryHigh=%dM
IOWeight=%d
TasksMax=%d
`, a.Limits.CPUPercent, a.Limits.MemoryMB, a.Limits.MemoryMB*9/10, a.Limits.IOWeight, a.Limits.MaxProcs)
	path := filepath.Join("/etc/systemd/system", unit, "50-breefer.conf")
	if err := p.run.WriteFile(path, []byte(dropin), 0o644); err != nil {
		return err
	}
	if _, err := p.run.Run(ctx, "systemctl", "daemon-reload"); err != nil {
		return err
	}
	_, err := p.run.Run(ctx, "systemctl", "set-property", "--runtime", fmt.Sprintf("user-%d.slice", a.UID),
		fmt.Sprintf("CPUQuota=%d%%", a.Limits.CPUPercent),
		fmt.Sprintf("MemoryMax=%dM", a.Limits.MemoryMB),
		fmt.Sprintf("TasksMax=%d", a.Limits.MaxProcs))
	return err
}

// Suspend blocks the account's web serving and logins without deleting data.
func (p *Provisioner) Suspend(ctx context.Context, a Account, domains []string) error {
	if _, err := p.run.Run(ctx, "usermod", "--lock", "--expiredate", "1", a.SysUser); err != nil {
		return err
	}
	if err := p.StopPHPPool(ctx, a); err != nil {
		return err
	}
	for _, d := range domains {
		if err := p.WriteVhost(ctx, Vhost{Domain: d, Suspended: true}, a); err != nil {
			return err
		}
	}
	_, err := p.run.Run(ctx, "pkill", "-KILL", "-u", a.SysUser)
	_ = err // pkill exits 1 when nothing matched; not an error here.
	return p.ReloadWeb(ctx)
}

// Unsuspend restores a suspended account. Callers re-write the vhosts.
func (p *Provisioner) Unsuspend(ctx context.Context, a Account) error {
	if _, err := p.run.Run(ctx, "usermod", "--unlock", "--expiredate", "", a.SysUser); err != nil {
		return err
	}
	return p.ApplyPHPPool(ctx, a)
}

// DeleteAccount removes the user, its home, PHP pool and slice.
func (p *Provisioner) DeleteAccount(ctx context.Context, a Account, domains []string) error {
	_ = p.StopPHPPool(ctx, a)
	for _, d := range domains {
		_ = p.RemoveVhost(ctx, d)
	}
	_ = p.RemoveCron(ctx, a)
	_, _ = p.run.Run(ctx, "pkill", "-KILL", "-u", a.SysUser)
	if _, err := p.run.Run(ctx, "userdel", "--remove", a.SysUser); err != nil {
		return err
	}
	_ = p.run.Remove(filepath.Join("/etc/systemd/system", fmt.Sprintf("user-%d.slice.d", a.UID), "50-breefer.conf"))
	return p.ReloadWeb(ctx)
}

// SetPassword changes the system password through chpasswd, passing the
// secret on stdin so it never appears in the process list.
func (p *Provisioner) SetPassword(ctx context.Context, user, password string) error {
	if err := system.SysUser(user); err != nil {
		return err
	}
	_, err := p.run.RunInput(ctx, []byte(user+":"+password+"\n"), "chpasswd")
	return err
}
