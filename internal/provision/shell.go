package provision

import (
	"bufio"
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/breefercp/breefercp/internal/system"
)

// Shell access modes for an account.
const (
	ShellNone = "none" // no interactive login, no SFTP
	ShellSFTP = "sftp" // SFTP only, forced by sshd's Match Group breefer-sftp
	ShellFull = "full" // /bin/bash and the browser terminal
)

// SFTPGroup is matched in sshd_config to force internal-sftp.
const SFTPGroup = "breefer-sftp"

// ValidShell reports whether mode is a known shell mode.
func ValidShell(mode string) bool {
	return mode == ShellNone || mode == ShellSFTP || mode == ShellFull
}

// SetShell applies a shell access mode to the Linux user.
func (p *Provisioner) SetShell(ctx context.Context, a Account, mode string) error {
	if err := system.SysUser(a.SysUser); err != nil {
		return err
	}
	shell := "/usr/sbin/nologin"
	switch mode {
	case ShellFull:
		shell = "/bin/bash"
	case ShellNone, ShellSFTP:
	default:
		return fmt.Errorf("%w: shell mode %q", system.ErrInvalid, mode)
	}
	if _, err := p.run.Run(ctx, "usermod", "--shell", shell, a.SysUser); err != nil {
		return err
	}
	if mode == ShellSFTP {
		_, err := p.run.Run(ctx, "gpasswd", "-a", a.SysUser, SFTPGroup)
		return err
	}
	// gpasswd -d fails when the user is not a member; that is the desired state.
	_, _ = p.run.Run(ctx, "gpasswd", "-d", a.SysUser, SFTPGroup)
	return nil
}

// SetDiskQuota applies a filesystem quota (requires quota-enabled /home).
// A host without quota support logs a warning rather than failing account
// operations; the panel still reports usage against the package limit.
func (p *Provisioner) SetDiskQuota(ctx context.Context, a Account) error {
	if err := system.SysUser(a.SysUser); err != nil {
		return err
	}
	kb := strconv.Itoa(a.Limits.DiskMB * 1024)
	if _, err := p.run.Run(ctx, "setquota", "-u", a.SysUser, kb, kb, "0", "0", "-a"); err != nil {
		p.log.Warn("disk quota not applied (is /home mounted with usrquota?)", "user", a.SysUser, "err", err)
	}
	return nil
}

// DirUsage is the size of one top-level directory of a home.
type DirUsage struct {
	Path   string `json:"path"`
	SizeMB int    `json:"size_mb"`
}

// DiskUsage measures the first level of an account's home directory.
func (p *Provisioner) DiskUsage(ctx context.Context, a Account) ([]DirUsage, error) {
	if p.run.DryRun {
		return []DirUsage{{"public_html", 842}, {"mail", 1260}, {"apps", 310}, {"logs", 96}, {"backups", 1840}, {"tmp", 12}}, nil
	}
	res, err := p.run.Run(ctx, "du", "-m", "--max-depth=1", "-x", "--", a.Home)
	if err != nil && res.Stdout == "" {
		return nil, err
	}
	var out []DirUsage
	sc := bufio.NewScanner(strings.NewReader(res.Stdout))
	for sc.Scan() {
		size, dir, ok := strings.Cut(sc.Text(), "\t")
		if !ok || path.Clean(dir) == path.Clean(a.Home) {
			continue
		}
		mb, _ := strconv.Atoi(size)
		out = append(out, DirUsage{Path: path.Base(dir), SizeMB: mb})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SizeMB > out[j].SizeMB })
	return out, nil
}
