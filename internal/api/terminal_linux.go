//go:build linux

package api

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

type linuxPTY struct {
	f   *os.File
	cmd *exec.Cmd
}

func (p *linuxPTY) Read(b []byte) (int, error)  { return p.f.Read(b) }
func (p *linuxPTY) Write(b []byte) (int, error) { return p.f.Write(b) }
func (p *linuxPTY) Resize(cols, rows uint16) error {
	return pty.Setsize(p.f, &pty.Winsize{Cols: cols, Rows: rows})
}
func (p *linuxPTY) Wait() error { return p.cmd.Wait() }
func (p *linuxPTY) Close() error {
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	err := p.f.Close()
	_ = p.cmd.Wait()
	return err
}

// startPTY starts a login shell as the account user, placed in the account's
// systemd slice so cgroup CPU, memory and process limits apply to it.
func startPTY(user string, uid int, home string) (ptySession, error) {
	cmd := exec.Command("systemd-run", "--quiet", "--scope", "--collect",
		fmt.Sprintf("--slice=user-%d.slice", uid), "--",
		"runuser", "--login", user)
	cmd.Dir = home
	cmd.Env = []string{"TERM=xterm-256color", "LANG=C.UTF-8", "PATH=/usr/local/bin:/usr/bin:/bin"}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 120, Rows: 32})
	if err != nil {
		return nil, err
	}
	return &linuxPTY{f: f, cmd: cmd}, nil
}
