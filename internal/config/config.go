// Package config loads BreeferCP daemon configuration from a TOML-like
// key=value file and environment overrides.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the full runtime configuration of breeferd.
type Config struct {
	// Listen is the address of the WHM (administrator and reseller) panel.
	Listen string
	// CpanelListen is the address of the separate customer (cPanel) panel.
	// Empty serves both panels from Listen, choosing the layout by role.
	CpanelListen string
	// TLSCert / TLSKey enable TLS on the panel. Empty means plain HTTP
	// (only acceptable behind a reverse proxy or on loopback).
	TLSCert string
	TLSKey  string

	// DBDriver is "sqlite" (single node) or "postgres" (cluster).
	DBDriver string
	// DBDSN is the sqlite file path or the postgres connection string.
	DBDSN string

	// DataDir holds state: backups, ACME accounts, generated secrets.
	DataDir string
	// WebDir is the directory containing the built React UI.
	WebDir string

	// SecretKey is the 32+ byte key used to sign short-lived tokens and
	// encrypt secrets at rest. Generated on first start when empty.
	SecretKey string

	// SessionTTL is how long an idle panel session stays valid.
	SessionTTL time.Duration

	// DryRun makes the provisioner log commands instead of executing them.
	// It is what allows the daemon to run on a developer workstation.
	DryRun bool

	// Hosting service integration endpoints.
	PowerDNSURL    string
	PowerDNSAPIKey string
	MariaDBDSN     string // admin DSN used to create customer databases
	HomeBase       string // base dir for account homes, e.g. /home
	NginxConfDir   string
	// WebUser is the account nginx and PHP-FPM sockets run as (www-data on Debian/Ubuntu).
	WebUser string
	// WebServer is "nginx" (default) or "openlitespeed".
	WebServer     string
	PHPFPMPoolDir string

	// TrustedProxies lists CIDRs from which X-Forwarded-For is honoured.
	TrustedProxies []string
}

// Default returns production defaults.
func Default() Config {
	return Config{
		Listen:        "0.0.0.0:2087",
		CpanelListen:  "0.0.0.0:2083",
		DBDriver:      "sqlite",
		DBDSN:         "/var/lib/breefercp/breefercp.db",
		DataDir:       "/var/lib/breefercp",
		WebDir:        "/usr/share/breefercp/web",
		SessionTTL:    12 * time.Hour,
		PowerDNSURL:   "http://127.0.0.1:8081",
		HomeBase:      "/home",
		NginxConfDir:  "/etc/nginx/breefercp.d",
		WebUser:       "www-data",
		WebServer:     "nginx",
		PHPFPMPoolDir: "/etc/php",
	}
}

// Load reads path (if it exists) and applies BREEFER_* environment overrides.
func Load(path string) (Config, error) {
	c := Default()
	if path != "" {
		f, err := os.Open(path)
		switch {
		case err == nil:
			defer f.Close()
			if err := parse(f, &c); err != nil {
				return c, fmt.Errorf("config %s: %w", path, err)
			}
		case errors.Is(err, os.ErrNotExist):
			// Missing file is fine: defaults + environment.
		default:
			return c, err
		}
	}
	applyEnv(&c)
	return c, c.Validate()
}

func parse(f *os.File, c *Config) error {
	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		s := strings.TrimSpace(sc.Text())
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		k, v, ok := strings.Cut(s, "=")
		if !ok {
			return fmt.Errorf("line %d: expected key = value", line)
		}
		if err := set(c, strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"`)); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
	}
	return sc.Err()
}

func applyEnv(c *Config) {
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if name, ok := strings.CutPrefix(k, "BREEFER_"); ok {
			_ = set(c, strings.ToLower(name), v)
		}
	}
}

func set(c *Config, k, v string) error {
	switch k {
	case "listen":
		c.Listen = v
	case "cpanel_listen":
		c.CpanelListen = v
	case "tls_cert":
		c.TLSCert = v
	case "tls_key":
		c.TLSKey = v
	case "db_driver":
		c.DBDriver = v
	case "db_dsn":
		c.DBDSN = v
	case "data_dir":
		c.DataDir = v
	case "web_dir":
		c.WebDir = v
	case "secret_key":
		c.SecretKey = v
	case "session_ttl":
		d, err := time.ParseDuration(v)
		if err != nil {
			return err
		}
		c.SessionTTL = d
	case "dry_run":
		b, err := strconv.ParseBool(v)
		if err != nil {
			return err
		}
		c.DryRun = b
	case "powerdns_url":
		c.PowerDNSURL = v
	case "powerdns_api_key":
		c.PowerDNSAPIKey = v
	case "mariadb_dsn":
		c.MariaDBDSN = v
	case "home_base":
		c.HomeBase = v
	case "nginx_conf_dir":
		c.NginxConfDir = v
	case "web_server":
		c.WebServer = v
	case "web_user":
		c.WebUser = v
	case "phpfpm_pool_dir":
		c.PHPFPMPoolDir = v
	case "trusted_proxies":
		c.TrustedProxies = strings.Split(v, ",")
	default:
		return fmt.Errorf("unknown key %q", k)
	}
	return nil
}

// Validate rejects configurations that cannot work or are unsafe.
func (c Config) Validate() error {
	switch c.DBDriver {
	case "sqlite", "postgres":
	default:
		return fmt.Errorf("db_driver must be sqlite or postgres, got %q", c.DBDriver)
	}
	if c.DBDSN == "" {
		return errors.New("db_dsn is required")
	}
	if c.WebServer != "nginx" && c.WebServer != "openlitespeed" {
		return fmt.Errorf("web_server must be nginx or openlitespeed, got %q", c.WebServer)
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return errors.New("tls_cert and tls_key must be set together")
	}
	if c.SecretKey != "" && len(c.SecretKey) < 32 {
		return errors.New("secret_key must be at least 32 characters")
	}
	return nil
}
