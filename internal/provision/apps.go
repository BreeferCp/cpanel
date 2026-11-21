package provision

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"text/template"

	"github.com/breefercp/breefercp/internal/system"
)

// App is a long-running Node.js / Python / container workload.
type App struct {
	Name    string
	Runtime string // node | python | ruby | docker
	// Version selects a runtime under /opt/breefercp/runtimes (e.g. node 22,
	// python 3.12, ruby 3.3). Empty means the system default.
	Version string
	Command string // start command for node/python (relative to Dir)
	Dir     string
	Image   string // docker image
	Port    int
	Env     map[string]string
}

var reAppName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
var reEnvKey = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,63}$`)
var reVersion = regexp.MustCompile(`^[0-9]{1,2}(\.[0-9]{1,2})?$`)

// Runtimes lists supported app runtimes and the versions offered.
var Runtimes = map[string][]string{
	"node":   {"22", "20", "18"},
	"python": {"3.13", "3.12", "3.11"},
	"ruby":   {"3.4", "3.3", "3.2"},
}

var reImage = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]{0,127}(:[A-Za-z0-9._-]{1,64})?$`)

var appUnit = template.Must(template.New("unit").Parse(`# Managed by BreeferCP. Do not edit.
[Unit]
Description=BreeferCP app {{.App.Name}} ({{.Acct.SysUser}})
After=network.target

[Service]
User={{.Acct.SysUser}}
Group={{.Acct.SysUser}}
Slice=user-{{.Acct.UID}}.slice
WorkingDirectory={{.App.Dir}}
ExecStart={{.Exec}}
Restart=on-failure
RestartSec=3
Environment=PORT={{.App.Port}}
{{- range .Env}}
Environment={{.}}
{{- end}}
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=tmpfs
BindPaths={{.Acct.Home}}
ReadWritePaths={{.Acct.Home}}
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
RestrictSUIDSGID=yes
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
CapabilityBoundingSet=
SystemCallFilter=@system-service

[Install]
WantedBy=multi-user.target
`))

func unitName(user, app string) string { return fmt.Sprintf("breefer-app-%s-%s.service", user, app) }

// RenderAppUnit renders the systemd unit for a node/python app. Docker apps
// use DockerRun instead.
func RenderAppUnit(a Account, app App) (string, error) {
	if !reAppName.MatchString(app.Name) {
		return "", fmt.Errorf("%w: app name", system.ErrInvalid)
	}
	if app.Port < 1024 || app.Port > 65535 {
		return "", fmt.Errorf("%w: port must be 1024-65535", system.ErrInvalid)
	}
	if err := system.OneLine(app.Command, 512); err != nil {
		return "", err
	}
	if app.Version != "" && !reVersion.MatchString(app.Version) {
		return "", fmt.Errorf("%w: runtime version", system.ErrInvalid)
	}
	bin := func(def, name string) string {
		if app.Version == "" {
			return "/usr/bin/env " + def
		}
		return fmt.Sprintf("/opt/breefercp/runtimes/%s-%s/bin/%s", app.Runtime, app.Version, name)
	}
	var exec string
	switch app.Runtime {
	case "node":
		exec = bin("node", "node") + " " + app.Command
	case "python":
		exec = bin("python3", "python3") + " " + app.Command
	case "ruby":
		// e.g. "bundle exec puma -p $PORT" or "config.ru" via rackup.
		exec = bin("ruby", "ruby") + " " + app.Command
	default:
		return "", fmt.Errorf("%w: runtime %q", system.ErrInvalid, app.Runtime)
	}
	var env []string
	keys := make([]string, 0, len(app.Env))
	for k := range app.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !reEnvKey.MatchString(k) {
			return "", fmt.Errorf("%w: env key %q", system.ErrInvalid, k)
		}
		if err := system.OneLine(app.Env[k], 1024); err != nil {
			return "", err
		}
		env = append(env, fmt.Sprintf("%q", k+"="+app.Env[k]))
	}
	var b bytes.Buffer
	err := appUnit.Execute(&b, map[string]any{"Acct": a, "App": app, "Exec": exec, "Env": env})
	return b.String(), err
}

// DeployApp installs and starts an app. Docker apps run through the docker
// CLI with strict flags; node/python apps run as systemd services inside the
// account's cgroup slice.
func (p *Provisioner) DeployApp(ctx context.Context, a Account, app App) error {
	if app.Runtime == "docker" {
		return p.dockerRun(ctx, a, app)
	}
	unit, err := RenderAppUnit(a, app)
	if err != nil {
		return err
	}
	name := unitName(a.SysUser, app.Name)
	if err := p.run.WriteFile(filepath.Join("/etc/systemd/system", name), []byte(unit), 0o644); err != nil {
		return err
	}
	if _, err := p.run.Run(ctx, "systemctl", "daemon-reload"); err != nil {
		return err
	}
	_, err = p.run.Run(ctx, "systemctl", "enable", "--now", name)
	return err
}

func (p *Provisioner) dockerRun(ctx context.Context, a Account, app App) error {
	if !reAppName.MatchString(app.Name) || !reImage.MatchString(app.Image) {
		return fmt.Errorf("%w: docker app name or image", system.ErrInvalid)
	}
	name := fmt.Sprintf("breefer-%s-%s", a.SysUser, app.Name)
	_, _ = p.run.Run(ctx, "docker", "rm", "-f", name)
	args := []string{"run", "-d", "--name", name, "--restart", "unless-stopped",
		"--user", fmt.Sprintf("%d:%d", a.UID, a.UID),
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--read-only",
		"--tmpfs", "/tmp", "--pids-limit", fmt.Sprint(a.Limits.MaxProcs),
		"--memory", fmt.Sprintf("%dm", a.Limits.MemoryMB), "--cpus", fmt.Sprintf("%.2f", float64(a.Limits.CPUPercent)/100),
		"--cgroup-parent", fmt.Sprintf("user-%d.slice", a.UID),
		"-p", fmt.Sprintf("127.0.0.1:%d:%d", app.Port, app.Port),
		"-v", filepath.ToSlash(filepath.Join(a.Home, "apps", app.Name)) + ":/data"}
	keys := make([]string, 0, len(app.Env))
	for k := range app.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !reEnvKey.MatchString(k) {
			return fmt.Errorf("%w: env key %q", system.ErrInvalid, k)
		}
		args = append(args, "-e", k+"="+app.Env[k])
	}
	args = append(args, app.Image)
	_, err := p.run.Run(ctx, "docker", args...)
	return err
}

// AppAction starts, stops or restarts an app.
func (p *Provisioner) AppAction(ctx context.Context, a Account, app App, action string) error {
	if action != "start" && action != "stop" && action != "restart" {
		return fmt.Errorf("%w: action", system.ErrInvalid)
	}
	if !reAppName.MatchString(app.Name) {
		return fmt.Errorf("%w: app name", system.ErrInvalid)
	}
	if app.Runtime == "docker" {
		_, err := p.run.Run(ctx, "docker", action, fmt.Sprintf("breefer-%s-%s", a.SysUser, app.Name))
		return err
	}
	_, err := p.run.Run(ctx, "systemctl", action, unitName(a.SysUser, app.Name))
	return err
}

// RemoveApp stops and deletes an app.
func (p *Provisioner) RemoveApp(ctx context.Context, a Account, app App) error {
	if !reAppName.MatchString(app.Name) {
		return fmt.Errorf("%w: app name", system.ErrInvalid)
	}
	if app.Runtime == "docker" {
		_, err := p.run.Run(ctx, "docker", "rm", "-f", fmt.Sprintf("breefer-%s-%s", a.SysUser, app.Name))
		return err
	}
	name := unitName(a.SysUser, app.Name)
	_, _ = p.run.Run(ctx, "systemctl", "disable", "--now", name)
	if err := p.run.Remove(filepath.Join("/etc/systemd/system", name)); err != nil {
		return err
	}
	_, err := p.run.Run(ctx, "systemctl", "daemon-reload")
	return err
}
