// Package provision turns panel state into host configuration: Linux users,
// systemd/cgroup slices, nginx vhosts, PHP-FPM pools, cron, firewall rules,
// DNS zones and containers. Every generated value is validated first and all
// commands go through system.Runner (no shell).
package provision

import (
	"log/slog"

	"github.com/breefercp/breefercp/internal/config"
	"github.com/breefercp/breefercp/internal/system"
)

// Provisioner applies desired state to the machine.
type Provisioner struct {
	cfg config.Config
	run *system.Runner
	log *slog.Logger
	dns *PowerDNS
}

// New builds a provisioner from daemon config.
func New(cfg config.Config, run *system.Runner, log *slog.Logger) *Provisioner {
	return &Provisioner{
		cfg: cfg,
		run: run,
		log: log,
		dns: NewPowerDNS(cfg.PowerDNSURL, cfg.PowerDNSAPIKey, run.DryRun),
	}
}

// Runner exposes the command runner (used by tests to read the dry-run log).
func (p *Provisioner) Runner() *system.Runner { return p.run }

// DNS returns the PowerDNS client.
func (p *Provisioner) DNS() *PowerDNS { return p.dns }

// Limits are the per-account resource ceilings enforced with cgroups v2.
type Limits struct {
	CPUPercent int // 100 == one full core
	MemoryMB   int
	IOWeight   int // 1-10000, default 100
	MaxProcs   int
	// DiskMB is the filesystem quota (0 = unlimited).
	DiskMB int
}

// Account is the minimal view of a hosting account the provisioner needs.
type Account struct {
	SysUser string
	UID     int
	Home    string
	Limits  Limits
	PHP     string
	// PHPIni holds customer php.ini overrides (validated by CleanPHPIni).
	PHPIni map[string]string
}
