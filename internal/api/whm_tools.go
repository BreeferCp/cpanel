package api

import (
	"context"
	"fmt"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/system"
)

type procInfo struct {
	PID        int     `json:"pid"`
	User       string  `json:"user"`
	State      string  `json:"state"`
	CPUSeconds float64 `json:"cpu_seconds"`
	MemMB      float64 `json:"mem_mb"`
	Command    string  `json:"command"`
}

func (s *Server) whmToolRoutes(mux *http.ServeMux) {
	s.route(mux, "GET /api/v1/server/info", rbac.ServerRead, s.getServerInfo)
	s.route(mux, "GET /api/v1/server/processes", rbac.ServerRead, s.getProcesses)
	s.route(mux, "POST /api/v1/server/processes/{pid}/kill", rbac.ServerWrite, s.killProc)
	s.route(mux, "GET /api/v1/server/mail-queue", rbac.ServerRead, s.getMailQueue)
	s.route(mux, "POST /api/v1/server/mail-queue/{action}", rbac.ServerWrite, s.mailQueueAction)

	s.route(mux, "GET /api/v1/server/login-rules", rbac.ServerRead, s.listLoginRules)
	s.route(mux, "POST /api/v1/server/login-rules", rbac.SettingsWrite, s.addLoginRule)
	s.route(mux, "DELETE /api/v1/server/login-rules/{id}", rbac.SettingsWrite, s.deleteLoginRule)
	s.route(mux, "GET /api/v1/server/login-failures", rbac.AuditRead, s.loginFailures)

	s.route(mux, "GET /api/v1/server/security", rbac.ServerRead, s.getSecurity)
	s.route(mux, "PUT /api/v1/server/security", rbac.SettingsWrite, s.putSecurity)
	s.route(mux, "GET /api/v1/server/backup-config", rbac.ServerRead, s.getBackupConfig)
	s.route(mux, "PUT /api/v1/server/backup-config", rbac.SettingsWrite, s.putBackupConfig)

	s.route(mux, "POST /api/v1/server/broadcast", rbac.AccountsWrite, s.broadcast)
	s.route(mux, "GET /api/v1/server/dns-zones", rbac.AccountsRead, s.listAllZones)
	s.route(mux, "GET /api/v1/server/quota-report", rbac.AccountsRead, s.quotaReport)
}

// putSetting stores one setting (insert or update, portable across drivers).
func (s *Server) putSetting(ctx context.Context, k, v string) error {
	res, err := s.db.Exec(ctx, `UPDATE settings SET value = ?, updated_at = ? WHERE key = ?`, v, time.Now().Unix(), k)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_, err = s.db.Exec(ctx, `INSERT INTO settings (key, value, updated_at) VALUES (?,?,?)`, k, v, time.Now().Unix())
	}
	return err
}

// ---- server information and processes ------------------------------------------

func (s *Server) getServerInfo(w http.ResponseWriter, r *http.Request) error {
	info := serverInfo()
	info["panel_version"] = s.version
	info["web_server"] = s.cfg.WebServer
	info["database"] = s.db.Driver()
	info["memory"] = hostMemory()
	info["load"] = hostLoad()
	type disk struct {
		Mount   string `json:"mount"`
		FS      string `json:"fs"`
		TotalMB int64  `json:"total_mb"`
		UsedMB  int64  `json:"used_mb"`
	}
	disks := []disk{}
	if res, err := s.prov.Runner().Run(r.Context(), "df", "-P", "-B1M", "-x", "tmpfs", "-x", "devtmpfs", "-x", "overlay", "-x", "squashfs"); err == nil {
		for i, l := range strings.Split(res.Stdout, "\n") {
			f := strings.Fields(l)
			if i == 0 || len(f) < 6 {
				continue
			}
			t, _ := strconv.ParseInt(f[1], 10, 64)
			u, _ := strconv.ParseInt(f[2], 10, 64)
			disks = append(disks, disk{Mount: f[5], FS: f[0], TotalMB: t, UsedMB: u})
		}
	}
	if len(disks) == 0 {
		d := hostDisk(s.cfg.HomeBase)
		disks = append(disks, disk{Mount: s.cfg.HomeBase, FS: "-", TotalMB: d["total_mb"], UsedMB: d["used_mb"]})
	}
	info["disks"] = disks
	writeJSON(w, 200, info)
	return nil
}

func (s *Server) getProcesses(w http.ResponseWriter, r *http.Request) error {
	procs := listProcesses()
	if q := strings.TrimSpace(r.URL.Query().Get("user")); q != "" {
		filtered := procs[:0]
		for _, p := range procs {
			if p.User == q {
				filtered = append(filtered, p)
			}
		}
		procs = filtered
	}
	if len(procs) > 500 {
		procs = procs[:500]
	}
	writeJSON(w, 200, procs)
	return nil
}

func (s *Server) killProc(w http.ResponseWriter, r *http.Request) error {
	pid, err := strconv.Atoi(r.PathValue("pid"))
	if err != nil || pid <= 1 || pid == os.Getpid() {
		return errBadRequest("this process cannot be stopped from the panel")
	}
	sig := r.URL.Query().Get("signal")
	if sig != "KILL" {
		sig = "TERM"
	}
	if s.cfg.DryRun {
		s.audit(r, "process.kill", strconv.Itoa(pid), true, sig+" (dry run)")
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	if err := killProcess(pid, sig); err != nil {
		return errConflict("could not signal the process: " + err.Error())
	}
	s.audit(r, "process.kill", strconv.Itoa(pid), true, sig)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- mail queue ------------------------------------------------------------------

func (s *Server) getMailQueue(w http.ResponseWriter, r *http.Request) error {
	msgs, err := s.prov.MailQueue(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, 200, msgs)
	return nil
}

func (s *Server) mailQueueAction(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		ID string `json:"id"`
	}
	_ = decode(r, &req)
	action := r.PathValue("action")
	if err := s.prov.MailQueueAction(r.Context(), action, req.ID); err != nil {
		return err
	}
	s.audit(r, "mailqueue."+action, req.ID, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- Host Access Control / brute-force protection -----------------------------------

type loginRule struct {
	ID        string `json:"id"`
	CIDR      string `json:"cidr"`
	Kind      string `json:"kind"`
	Note      string `json:"note"`
	CreatedAt int64  `json:"created_at"`
}

func (s *Server) loginRules(ctx context.Context) []loginRule {
	rows, err := s.db.Query(ctx, `SELECT id, cidr, kind, note, created_at FROM login_rules ORDER BY created_at DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []loginRule
	for rows.Next() {
		var x loginRule
		if rows.Scan(&x.ID, &x.CIDR, &x.Kind, &x.Note, &x.CreatedAt) == nil {
			out = append(out, x)
		}
	}
	return out
}

// loginRuleFor returns "allow", "deny" or "" for a client IP. A deny entry
// wins over an allow entry that also matches.
func (s *Server) loginRuleFor(ctx context.Context, ip string) string {
	verdict := ""
	for _, rule := range s.loginRules(ctx) {
		if ipAllowed(rule.CIDR, ip) {
			if rule.Kind == "deny" {
				return "deny"
			}
			verdict = "allow"
		}
	}
	return verdict
}

func (s *Server) listLoginRules(w http.ResponseWriter, r *http.Request) error {
	out := s.loginRules(r.Context())
	if out == nil {
		out = []loginRule{}
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) addLoginRule(w http.ResponseWriter, r *http.Request) error {
	var req loginRule
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Kind != "allow" && req.Kind != "deny" {
		return errBadRequest("kind must be allow or deny")
	}
	req.CIDR = strings.TrimSpace(req.CIDR)
	if req.CIDR == "" || system.CIDR(req.CIDR) != nil {
		return errBadRequest("enter an IP address or CIDR range")
	}
	if err := system.OneLine(req.Note, 120); err != nil {
		return err
	}
	if req.Kind == "deny" && ipAllowed(req.CIDR, s.clientIP(r)) {
		return errBadRequest("that rule would block your own address")
	}
	req.ID, req.CreatedAt = ids.New(), time.Now().Unix()
	if _, err := s.db.Exec(r.Context(), `INSERT INTO login_rules (id, cidr, kind, note, created_at) VALUES (?,?,?,?,?)`,
		req.ID, req.CIDR, req.Kind, req.Note, req.CreatedAt); err != nil {
		return err
	}
	s.audit(r, "security.loginrule.add", req.CIDR, true, req.Kind)
	writeJSON(w, 201, req)
	return nil
}

func (s *Server) deleteLoginRule(w http.ResponseWriter, r *http.Request) error {
	if _, err := s.db.Exec(r.Context(), `DELETE FROM login_rules WHERE id = ?`, r.PathValue("id")); err != nil {
		return err
	}
	s.audit(r, "security.loginrule.delete", r.PathValue("id"), true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// loginFailures summarises failed sign-ins per IP over the last 24 hours.
func (s *Server) loginFailures(w http.ResponseWriter, r *http.Request) error {
	since := time.Now().Add(-24 * time.Hour).Unix()
	rows, err := s.db.Query(r.Context(), `SELECT ip, COUNT(*), MAX(at), MAX(actor_name) FROM audit_log
		WHERE action IN ('auth.login','auth.2fa') AND ok = 0 AND at > ? GROUP BY ip ORDER BY COUNT(*) DESC LIMIT 200`, since)
	if err != nil {
		return err
	}
	defer rows.Close()
	type v struct {
		IP       string `json:"ip"`
		Failures int    `json:"failures"`
		Last     int64  `json:"last"`
		Username string `json:"username"`
		Blocked  bool   `json:"blocked"`
	}
	out := []v{}
	for rows.Next() {
		var x v
		if err := rows.Scan(&x.IP, &x.Failures, &x.Last, &x.Username); err != nil {
			return err
		}
		x.Blocked = s.loginLimit.blocked("ip:" + x.IP)
		out = append(out, x)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

// ---- security policy -----------------------------------------------------------------

type securityPolicy struct {
	Require2FA    string `json:"require_2fa"` // off | admins | all
	WHMAllowedIPs string `json:"whm_allowed_ips"`
}

func (s *Server) securityPolicy(ctx context.Context) securityPolicy {
	p := securityPolicy{Require2FA: s.setting(ctx, "require_2fa"), WHMAllowedIPs: s.setting(ctx, "whm_allowed_ips")}
	if p.Require2FA == "" {
		p.Require2FA = "off"
	}
	return p
}

// twoFactorRequired reports whether role must have 2FA before using the panel.
func (s *Server) twoFactorRequired(ctx context.Context, role rbac.Role) bool {
	switch s.setting(ctx, "require_2fa") {
	case "all":
		return true
	case "admins":
		return role == rbac.Root || role == rbac.Reseller
	}
	return false
}

// whmAllowed reports whether ip may use WHM under the allow list.
func (s *Server) whmAllowed(ctx context.Context, ip string) bool {
	list := s.setting(ctx, "whm_allowed_ips")
	return strings.TrimSpace(list) == "" || ipAllowed(list, ip)
}

func (s *Server) getSecurity(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, 200, s.securityPolicy(r.Context()))
	return nil
}

func (s *Server) putSecurity(w http.ResponseWriter, r *http.Request) error {
	var req securityPolicy
	if err := decode(r, &req); err != nil {
		return err
	}
	switch req.Require2FA {
	case "off", "admins", "all":
	default:
		return errBadRequest("require_2fa must be off, admins or all")
	}
	entries := strings.FieldsFunc(req.WHMAllowedIPs, func(c rune) bool { return c == ',' || c == ' ' || c == '\n' })
	for _, e := range entries {
		if system.CIDR(e) != nil {
			return errBadRequest(e + " is not an IP address or CIDR range")
		}
	}
	req.WHMAllowedIPs = strings.Join(entries, ", ")
	if len(req.WHMAllowedIPs) > 2000 {
		return errBadRequest("the allow list is too long")
	}
	if req.WHMAllowedIPs != "" && !ipAllowed(req.WHMAllowedIPs, s.clientIP(r)) {
		return errBadRequest("the allow list must include your current address (" + s.clientIP(r) + ")")
	}
	p := principalFrom(r.Context())
	if req.Require2FA != "off" && !p.TOTP {
		return errBadRequest("turn on two-factor authentication for your own login first")
	}
	if err := s.putSetting(r.Context(), "require_2fa", req.Require2FA); err != nil {
		return err
	}
	if err := s.putSetting(r.Context(), "whm_allowed_ips", req.WHMAllowedIPs); err != nil {
		return err
	}
	s.audit(r, "security.policy", "", true, req.Require2FA)
	writeJSON(w, 200, req)
	return nil
}

// ---- scheduled backups -------------------------------------------------------------------

type backupConfig struct {
	Enabled   bool   `json:"enabled"`
	Frequency string `json:"frequency"` // daily | weekly
	Weekday   int    `json:"weekday"`   // 0 = Sunday, weekly only
	Hour      int    `json:"hour"`
	Retention int    `json:"retention"`
	Kind      string `json:"kind"`
	LastRun   int64  `json:"last_run"`
}

func (s *Server) backupConfig(ctx context.Context) backupConfig {
	c := backupConfig{Frequency: "daily", Hour: 2, Retention: 7, Kind: "full"}
	c.Enabled = s.setting(ctx, "backup_enabled") == "1"
	if v := s.setting(ctx, "backup_frequency"); v != "" {
		c.Frequency = v
	}
	if v, err := strconv.Atoi(s.setting(ctx, "backup_hour")); err == nil {
		c.Hour = v
	}
	if v, err := strconv.Atoi(s.setting(ctx, "backup_weekday")); err == nil {
		c.Weekday = v
	}
	if v, err := strconv.Atoi(s.setting(ctx, "backup_retention")); err == nil {
		c.Retention = v
	}
	if v := s.setting(ctx, "backup_kind"); v != "" {
		c.Kind = v
	}
	c.LastRun, _ = strconv.ParseInt(s.setting(ctx, "backup_last_run"), 10, 64)
	return c
}

func (s *Server) getBackupConfig(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, 200, s.backupConfig(r.Context()))
	return nil
}

func (s *Server) putBackupConfig(w http.ResponseWriter, r *http.Request) error {
	var req backupConfig
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Frequency != "daily" && req.Frequency != "weekly" {
		return errBadRequest("frequency must be daily or weekly")
	}
	if req.Hour < 0 || req.Hour > 23 || req.Weekday < 0 || req.Weekday > 6 {
		return errBadRequest("invalid schedule")
	}
	if req.Retention < 1 || req.Retention > 60 {
		return errBadRequest("keep between 1 and 60 backups per account")
	}
	if req.Kind != "full" && req.Kind != "files" && req.Kind != "databases" {
		return errBadRequest("kind must be full, files or databases")
	}
	for k, v := range map[string]string{
		"backup_enabled": strconv.Itoa(b2i(req.Enabled)), "backup_frequency": req.Frequency, "backup_hour": strconv.Itoa(req.Hour),
		"backup_weekday": strconv.Itoa(req.Weekday), "backup_retention": strconv.Itoa(req.Retention), "backup_kind": req.Kind,
	} {
		if err := s.putSetting(r.Context(), k, v); err != nil {
			return err
		}
	}
	s.audit(r, "backup.config", "", true, "")
	return s.getBackupConfig(w, r)
}

// runScheduledBackups is called by the housekeeping loop; it runs at most
// once per scheduled slot and backs accounts up one at a time.
func (s *Server) runScheduledBackups(ctx context.Context) {
	c := s.backupConfig(ctx)
	now := time.Now()
	if !c.Enabled || now.Hour() != c.Hour || (c.Frequency == "weekly" && int(now.Weekday()) != c.Weekday) {
		return
	}
	if c.LastRun > 0 && now.Sub(time.Unix(c.LastRun, 0)) < 20*time.Hour {
		return
	}
	_ = s.putSetting(ctx, "backup_last_run", strconv.FormatInt(now.Unix(), 10))
	rows, err := s.db.Query(ctx, acctSelect+` WHERE a.status = 'active'`)
	if err != nil {
		s.log.Error("scheduled backups", "err", err)
		return
	}
	var accts []*acct
	for rows.Next() {
		a, err := scanAcct(rows)
		if err == nil {
			accts = append(accts, a)
		}
	}
	rows.Close()
	go func() {
		for _, a := range accts {
			id := ids.New()
			file := filepath.Join(s.backupDir(a), "auto", fmt.Sprintf("%s-%s-%d.tar.gz", a.SysUser, c.Kind, time.Now().Unix()))
			if _, err := s.db.Exec(ctx, `INSERT INTO backups (id, account_id, kind, path, status, created_at) VALUES (?,?,?,?,'running',?)`,
				id, a.ID, c.Kind, file, time.Now().Unix()); err != nil {
				continue
			}
			s.runBackup(*a, id, c.Kind, file)
			s.pruneScheduledBackups(ctx, a, c.Retention)
		}
		s.log.Info("scheduled backups finished", "accounts", len(accts))
	}()
}

func (s *Server) pruneScheduledBackups(ctx context.Context, a *acct, keep int) {
	rows, err := s.db.Query(ctx, `SELECT id, path FROM backups WHERE account_id = ? AND path LIKE ? ORDER BY created_at DESC`,
		a.ID, "%"+string(filepath.Separator)+"auto"+string(filepath.Separator)+"%")
	if err != nil {
		return
	}
	type b struct{ id, path string }
	var old []b
	n := 0
	for rows.Next() {
		var x b
		if rows.Scan(&x.id, &x.path) == nil {
			n++
			if n > keep {
				old = append(old, x)
			}
		}
	}
	rows.Close()
	for _, x := range old {
		_ = s.prov.Runner().Remove(x.path)
		_, _ = s.db.Exec(ctx, `DELETE FROM backups WHERE id = ?`, x.id)
	}
}

// ---- broadcast (Email All Users / announcement) ------------------------------------------------

func (s *Server) broadcast(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Audience     string `json:"audience"` // users | resellers | all
		Subject      string `json:"subject"`
		Body         string `json:"body"`
		Announcement bool   `json:"announcement"`
		Email        bool   `json:"email"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	p := principalFrom(r.Context())
	req.Subject = strings.TrimSpace(req.Subject)
	if req.Subject == "" || len(req.Subject) > 150 || strings.ContainsAny(req.Subject, "\r\n") {
		return errBadRequest("a one-line subject (max 150 characters) is required")
	}
	if strings.TrimSpace(req.Body) == "" || len(req.Body) > 20000 {
		return errBadRequest("the message is required (max 20000 characters)")
	}
	if req.Audience != "users" && req.Audience != "resellers" && req.Audience != "all" {
		return errBadRequest("audience must be users, resellers or all")
	}
	if p.Role != rbac.Root && req.Audience != "users" {
		return errForbidden()
	}
	if req.Announcement {
		if p.Role != rbac.Root {
			return errForbidden()
		}
		if err := s.putSetting(r.Context(), "announcement", req.Subject); err != nil {
			return err
		}
	}
	sent := 0
	if req.Email {
		q := `SELECT u.email FROM users u WHERE u.suspended = 0 AND u.email <> ''`
		var args []any
		switch req.Audience {
		case "users":
			q += ` AND u.role = 'user'`
		case "resellers":
			q += ` AND u.role = 'reseller'`
		default:
			q += ` AND u.role <> 'root'`
		}
		if p.Role == rbac.Reseller {
			q += ` AND (u.owner_id = ? OR u.owner_id IN (SELECT id FROM users WHERE owner_id = ?))`
			args = append(args, p.UserID, p.UserID)
		}
		rows, err := s.db.Query(r.Context(), q, args...)
		if err != nil {
			return err
		}
		var to []string
		for rows.Next() {
			var e string
			if rows.Scan(&e) == nil {
				if _, err := mail.ParseAddress(e); err == nil {
					to = append(to, e)
				}
			}
		}
		rows.Close()
		from := s.setting(r.Context(), "contact_email")
		if from == "" {
			host := s.setting(r.Context(), "hostname")
			if host == "" {
				host = "localhost"
			}
			from = "noreply@" + host
		}
		for _, addr := range to {
			msg := "From: " + from + "\r\nTo: " + addr + "\r\nSubject: " + req.Subject +
				"\r\nContent-Type: text/plain; charset=utf-8\r\nAuto-Submitted: auto-generated\r\n\r\n" + req.Body + "\r\n"
			if _, err := s.prov.Runner().RunInput(r.Context(), []byte(msg), "/usr/sbin/sendmail", "-i", "--", addr); err != nil {
				s.log.Warn("broadcast mail", "to", addr, "err", err)
				continue
			}
			sent++
		}
	}
	s.audit(r, "broadcast", req.Audience, true, fmt.Sprintf("%s (%d emails)", req.Subject, sent))
	writeJSON(w, 200, map[string]int{"sent": sent})
	return nil
}

// ---- DNS zones and quota report --------------------------------------------------------------

func (s *Server) listAllZones(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	q := `SELECT d.name, a.id, a.sys_user, COALESCE(z.dnssec, 0) FROM domains d JOIN accounts a ON a.id = d.account_id
		JOIN users u ON u.id = a.user_id LEFT JOIN users o ON o.id = u.owner_id LEFT JOIN dns_zones z ON z.name = d.name
		WHERE d.kind IN ('primary','addon')`
	var args []any
	if p.Role == rbac.Reseller {
		q += ` AND (u.owner_id = ? OR o.owner_id = ?)`
		args = append(args, p.UserID, p.UserID)
	}
	rows, err := s.db.Query(r.Context(), q+` ORDER BY d.name`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	type v struct {
		Zone      string `json:"zone"`
		AccountID string `json:"account_id"`
		User      string `json:"user"`
		DNSSEC    bool   `json:"dnssec"`
	}
	out := []v{}
	for rows.Next() {
		var x v
		var sec int
		if err := rows.Scan(&x.Zone, &x.AccountID, &x.User, &sec); err != nil {
			return err
		}
		x.DNSSEC = sec == 1
		out = append(out, x)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) quotaReport(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	q := acctSelect
	var args []any
	if p.Role == rbac.Reseller {
		q += ` WHERE (u.owner_id = ? OR o.owner_id = ?)`
		args = append(args, p.UserID, p.UserID)
	}
	rows, err := s.db.Query(r.Context(), q+` ORDER BY a.disk_used_mb DESC`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	type v struct {
		ID       string  `json:"id"`
		User     string  `json:"user"`
		Domain   string  `json:"domain"`
		Status   string  `json:"status"`
		UsedMB   int     `json:"used_mb"`
		LimitMB  int     `json:"limit_mb"`
		BWUsedMB int     `json:"bw_used_mb"`
		Percent  float64 `json:"percent"`
		Over     bool    `json:"over"`
	}
	out := []v{}
	for rows.Next() {
		a, err := scanAcct(rows)
		if err != nil {
			return err
		}
		x := v{ID: a.ID, User: a.SysUser, Domain: a.PrimaryDomain, Status: a.Status, UsedMB: a.DiskUsedMB, LimitMB: a.pkg.DiskMB, BWUsedMB: a.BWUsedMB}
		if a.pkg.DiskMB > 0 {
			x.Percent = float64(a.DiskUsedMB) * 100 / float64(a.pkg.DiskMB)
			x.Over = a.DiskUsedMB > a.pkg.DiskMB
		}
		out = append(out, x)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}
