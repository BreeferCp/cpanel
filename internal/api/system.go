package api

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/provision"
	"github.com/breefercp/breefercp/internal/system"
)

// managedServices are the daemons an administrator may inspect and control.
var managedServices = map[string]string{
	"nginx":      "Web server (nginx)",
	"php8.3-fpm": "PHP-FPM 8.3",
	"php8.2-fpm": "PHP-FPM 8.2",
	"mariadb":    "MariaDB",
	"postfix":    "Postfix (SMTP)",
	"dovecot":    "Dovecot (IMAP/POP3)",
	"pdns":       "PowerDNS",
	"opendkim":   "OpenDKIM",
	"docker":     "Docker",
	"nftables":   "Firewall (nftables)",
	"fail2ban":   "Fail2ban",
	"breeferd":   "BreeferCP daemon",
}

func (s *Server) serverStatus(w http.ResponseWriter, r *http.Request) error {
	// An administrator-configured hostname (Settings) wins over the OS value.
	hostname := s.setting(r.Context(), "hostname")
	if hostname == "" {
		hostname, _ = os.Hostname()
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	var accounts, suspended int
	_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM accounts`).Scan(&accounts)
	_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM accounts WHERE status = 'suspended'`).Scan(&suspended)
	load := hostLoad()
	writeJSON(w, 200, map[string]any{
		"hostname":  hostname,
		"version":   s.version,
		"os":        runtime.GOOS + "/" + runtime.GOARCH,
		"uptime_s":  int(time.Since(s.started).Seconds()),
		"cpus":      runtime.NumCPU(),
		"load":      load,
		"memory":    hostMemory(),
		"disk":      hostDisk(s.cfg.HomeBase),
		"accounts":  map[string]int{"total": accounts, "suspended": suspended},
		"database":  s.db.Driver(),
		"dry_run":   s.cfg.DryRun,
		"daemon_mb": int(m.Alloc / 1048576),
	})
	return nil
}

func (s *Server) listServices(w http.ResponseWriter, r *http.Request) error {
	type sv struct {
		Name   string `json:"name"`
		Label  string `json:"label"`
		Active bool   `json:"active"`
		State  string `json:"state"`
	}
	out := make([]sv, 0, len(managedServices))
	for name, label := range managedServices {
		v := sv{Name: name, Label: label, State: "unknown"}
		if res, err := s.prov.Runner().Run(r.Context(), "systemctl", "is-active", name); err == nil {
			v.State = strings.TrimSpace(res.Stdout)
			v.Active = v.State == "active"
		} else {
			v.State = "inactive"
		}
		if s.cfg.DryRun {
			v.State, v.Active = "active", true
		}
		out = append(out, v)
	}
	sortServices(out, func(i int) string { return out[i].Name })
	writeJSON(w, 200, out)
	return nil
}

func sortServices[T any](v []T, key func(int) string) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && key(j) < key(j-1); j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

func (s *Server) serviceAction(w http.ResponseWriter, r *http.Request) error {
	name, action := r.PathValue("name"), r.PathValue("action")
	if _, ok := managedServices[name]; !ok {
		return errBadRequest("unknown service")
	}
	if action != "start" && action != "stop" && action != "restart" && action != "reload" {
		return errBadRequest("action must be start, stop, restart or reload")
	}
	if name == "breeferd" && action == "stop" {
		return errBadRequest("stopping the panel from the panel is not allowed")
	}
	_, err := s.prov.Runner().Run(r.Context(), "systemctl", action, name)
	s.audit(r, "service."+action, name, err == nil, "")
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- firewall ---------------------------------------------------------------

type fwRule struct {
	ID       string `json:"id"`
	Action   string `json:"action"`
	Protocol string `json:"protocol"`
	Port     string `json:"port"`
	Source   string `json:"source"`
	Comment  string `json:"comment"`
}

func (s *Server) firewallRules(ctx context.Context) ([]fwRule, error) {
	rows, err := s.db.Query(ctx, `SELECT id, action, protocol, port, source, comment FROM firewall_rules ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []fwRule{}
	for rows.Next() {
		var f fwRule
		if err := rows.Scan(&f.ID, &f.Action, &f.Protocol, &f.Port, &f.Source, &f.Comment); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Server) applyFirewall(ctx context.Context) error {
	rules, err := s.firewallRules(ctx)
	if err != nil {
		return err
	}
	pr := make([]provision.FirewallRule, len(rules))
	for i, f := range rules {
		pr[i] = provision.FirewallRule{Action: f.Action, Protocol: f.Protocol, Port: f.Port, Source: f.Source}
	}
	return s.prov.ApplyFirewall(ctx, pr, nil)
}

func (s *Server) listFirewall(w http.ResponseWriter, r *http.Request) error {
	out, err := s.firewallRules(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) addFirewall(w http.ResponseWriter, r *http.Request) error {
	var req fwRule
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Action != "allow" && req.Action != "deny" {
		return errBadRequest("action must be allow or deny")
	}
	if req.Protocol != "tcp" && req.Protocol != "udp" && req.Protocol != "any" {
		return errBadRequest("protocol must be tcp, udp or any")
	}
	if err := system.Port(req.Port); err != nil {
		return err
	}
	if err := system.CIDR(req.Source); err != nil {
		return err
	}
	if req.Port == "" && req.Source == "" && req.Protocol == "any" {
		return errBadRequest("a rule must restrict at least a port, protocol or source")
	}
	if len(req.Comment) > 120 || strings.ContainsAny(req.Comment, "\r\n") {
		return errBadRequest("comment is too long")
	}
	req.ID = ids.New()
	if _, err := s.db.Exec(r.Context(), `INSERT INTO firewall_rules (id, action, protocol, port, source, comment, created_at) VALUES (?,?,?,?,?,?,?)`,
		req.ID, req.Action, req.Protocol, req.Port, req.Source, req.Comment, time.Now().Unix()); err != nil {
		return err
	}
	if err := s.applyFirewall(r.Context()); err != nil {
		_, _ = s.db.Exec(context.WithoutCancel(r.Context()), `DELETE FROM firewall_rules WHERE id = ?`, req.ID)
		return err
	}
	s.audit(r, "firewall.add", req.Action+" "+req.Protocol+" "+req.Port, true, req.Source)
	writeJSON(w, 201, req)
	return nil
}

func (s *Server) deleteFirewall(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.db.Exec(r.Context(), `DELETE FROM firewall_rules WHERE id = ?`, r.PathValue("id")); err != nil {
		return err
	}
	if err := s.applyFirewall(r.Context()); err != nil {
		return err
	}
	s.audit(r, "firewall.delete", r.PathValue("id"), true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- settings -----------------------------------------------------------------

// allowedSettings is the allow-list of keys the settings API may write.
var allowedSettings = map[string]bool{
	"server_ip": true, "hostname": true, "nameserver1": true, "nameserver2": true,
	"nameserver1_ip": true, "nameserver2_ip": true, "tools_url": true,
	"contact_email": true, "company_name": true, "acme_email": true, "domain_verification": true,
}

func (s *Server) setting(ctx context.Context, key string) string {
	var v string
	_ = s.db.QueryRow(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	return v
}

func (s *Server) serverIP(ctx context.Context) string {
	if v := s.setting(ctx, "server_ip"); v != "" {
		return v
	}
	return "127.0.0.1"
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) error {
	out := map[string]string{}
	for k := range allowedSettings {
		out[k] = s.setting(r.Context(), k)
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) error {
	var req map[string]string
	if err := decode(r, &req); err != nil {
		return err
	}
	for k, v := range req {
		if !allowedSettings[k] {
			return errBadRequest("unknown setting " + strconv.Quote(k))
		}
		if len(v) > 255 || strings.ContainsAny(v, "\r\n") {
			return errBadRequest("invalid value for " + k)
		}
		if v == "" {
			continue
		}
		switch k {
		case "server_ip", "nameserver1_ip", "nameserver2_ip":
			if net.ParseIP(v) == nil {
				return errBadRequest(k + " must be an IP address")
			}
		case "hostname", "nameserver1", "nameserver2":
			if err := system.Domain(v); err != nil {
				return errBadRequest(k + " must be a fully qualified host name")
			}
		case "domain_verification":
			if v != "on" && v != "off" {
				return errBadRequest("domain_verification must be on or off")
			}
		case "tools_url":
			if u, err := url.Parse(v); err != nil || u.Scheme != "https" || u.Host == "" {
				return errBadRequest("tools_url must be an https:// URL")
			}
		}
	}
	for k, v := range req {
		if s.db.Driver() == "postgres" {
			_, _ = s.db.Exec(r.Context(), `INSERT INTO settings (key, value, updated_at) VALUES (?,?,?) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at`, k, v, time.Now().Unix())
		} else {
			_, _ = s.db.Exec(r.Context(), `INSERT INTO settings (key, value, updated_at) VALUES (?,?,?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, k, v, time.Now().Unix())
		}
	}
	s.audit(r, "settings.update", "", true, "")
	s.publishGlue(r.Context())
	return s.getSettings(w, r)
}

// nameservers returns the configured NS host names for new zones (nil lets
// the DNS layer fall back to ns1/ns2.<zone>).
func (s *Server) nameservers(ctx context.Context) []string {
	var out []string
	for _, k := range []string{"nameserver1", "nameserver2"} {
		if v := s.setting(ctx, k); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// publishGlue writes A records for private nameservers (ns1.example.com) into
// their parent zone when that zone is hosted on this server. Failures are
// logged: the zone may simply live at another provider.
func (s *Server) publishGlue(ctx context.Context) {
	for _, n := range []string{"1", "2"} {
		host, ip := s.setting(ctx, "nameserver"+n), s.setting(ctx, "nameserver"+n+"_ip")
		if host == "" || ip == "" {
			continue
		}
		label, zone, ok := strings.Cut(host, ".")
		if !ok || system.Domain(zone) != nil {
			continue
		}
		rec := provision.Record{Name: label, Type: "A", TTL: 3600, Content: ip}
		if strings.Contains(ip, ":") {
			rec.Type = "AAAA"
		}
		if err := s.prov.DNS().UpsertRecord(ctx, zone, rec); err != nil {
			s.log.Warn("nameserver glue not published", "host", host, "err", err)
		}
	}
}
