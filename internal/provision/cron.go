package provision

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/breefercp/breefercp/internal/system"
)

// CronJob is one scheduled command for an account.
type CronJob struct {
	Schedule string
	Command  string
	Enabled  bool
}

// WriteCron writes /etc/cron.d/breefer-<user>. Jobs run as the account user.
// Schedule and command are validated so no field can inject extra lines.
func (p *Provisioner) WriteCron(ctx context.Context, a Account, jobs []CronJob, mailTo string) error {
	if err := system.SysUser(a.SysUser); err != nil {
		return err
	}
	var b bytes.Buffer
	b.WriteString("# Managed by BreeferCP. Do not edit.\nSHELL=/bin/sh\nPATH=/usr/local/bin:/usr/bin:/bin\n")
	if mailTo != "" && !CronMailTo(mailTo) {
		return fmt.Errorf("%w: cron email", system.ErrInvalid)
	}
	fmt.Fprintf(&b, "MAILTO=\"%s\"\n", mailTo)
	for _, j := range jobs {
		if !j.Enabled {
			continue
		}
		if err := system.Cron(j.Schedule); err != nil {
			return err
		}
		if err := system.OneLine(j.Command, 1024); err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %s %s\n", strings.Join(strings.Fields(j.Schedule), " "), a.SysUser, j.Command)
	}
	return p.run.WriteFile(filepath.Join("/etc/cron.d", "breefer-"+a.SysUser), b.Bytes(), 0o644)
}

// RemoveCron deletes the account's cron file.
func (p *Provisioner) RemoveCron(_ context.Context, a Account) error {
	return p.run.Remove(filepath.Join("/etc/cron.d", "breefer-"+a.SysUser))
}
