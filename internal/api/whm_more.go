package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/breefercp/breefercp/internal/auth"
	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/store"
	"github.com/breefercp/breefercp/internal/system"
)

func (s *Server) whmMoreRoutes(mux *http.ServeMux) {
	s.route(mux, "GET /api/v1/server/domains", rbac.AccountsRead, s.serverDomains)
	s.route(mux, "PUT /api/v1/server/domains/php", rbac.AccountsWrite, s.bulkDomainPHP)
	s.route(mux, "POST /api/v1/server/autossl", rbac.ServerWrite, s.runAutoSSL)
	s.route(mux, "GET /api/v1/server/backups", rbac.ServerRead, s.serverBackups)
	s.route(mux, "POST /api/v1/accounts/{aid}/password", rbac.AccountsWrite, s.adminSetPassword)

	s.route(mux, "GET /api/v1/server/time", rbac.ServerRead, s.getServerTime)
	s.route(mux, "PUT /api/v1/server/time", rbac.SettingsWrite, s.putServerTime)
	s.route(mux, "GET /api/v1/server/ssh", rbac.ServerRead, s.getSSHConfig)
	s.route(mux, "PUT /api/v1/server/ssh", rbac.SettingsWrite, s.putSSHConfig)
	s.route(mux, "GET /api/v1/server/root-keys", rbac.ServerRead, s.listRootKeys)
	s.route(mux, "POST /api/v1/server/root-keys", rbac.SettingsWrite, s.addRootKey)
	s.route(mux, "DELETE /api/v1/server/root-keys/{fp}", rbac.SettingsWrite, s.deleteRootKey)
	s.route(mux, "POST /api/v1/server/root-password", rbac.SettingsWrite, s.rootPassword)
	s.route(mux, "GET /api/v1/server/security-advisor", rbac.ServerRead, s.securityAdvisor)
	s.route(mux, "POST /api/v1/server/reboot", rbac.ServerWrite, s.reboot)
}

// ---- server-wide domain list ------------------------------------------------------

type serverDomain struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	AccountID string `json:"account_id"`
	User      string `json:"user"`
	Owner     string `json:"owner"`
	Status    string `json:"status"`
	PHP       string `json:"php_version"`
	DocRoot   string `json:"doc_root"`
	Issuer    string `json:"ssl_issuer"`
	NotAfter  int64  `json:"ssl_not_after"`
	CreatedAt int64  `json:"created_at"`
}

// tenantFilter limits a query over accounts (alias a, users u) to what the
// caller may see: everything for root, the owner chain for resellers.
func tenantFilter(p *Principal) (string, []any) {
	if p.Role == rbac.Root {
		return "", nil
	}
	return ` AND (u.owner_id = ? OR u.owner_id IN (SELECT id FROM users WHERE owner_id = ?))`, []any{p.UserID, p.UserID}
}

func (s *Server) serverDomains(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	where, args := tenantFilter(p)
	rows, err := s.db.Query(r.Context(), `SELECT d.id, d.name, d.kind, a.id, u.username, COALESCE(o.username, ''), a.status, d.php_version, d.doc_root, d.created_at,
		COALESCE((SELECT c.issuer FROM ssl_certs c WHERE c.domain_id = d.id ORDER BY c.created_at DESC LIMIT 1), ''),
		COALESCE((SELECT c.not_after FROM ssl_certs c WHERE c.domain_id = d.id ORDER BY c.created_at DESC LIMIT 1), 0)
		FROM domains d JOIN accounts a ON a.id = d.account_id JOIN users u ON u.id = a.user_id LEFT JOIN users o ON o.id = u.owner_id
		WHERE 1=1`+where+` ORDER BY d.name LIMIT 5000`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []serverDomain{}
	for rows.Next() {
		var d serverDomain
		if err := rows.Scan(&d.ID, &d.Name, &d.Kind, &d.AccountID, &d.User, &d.Owner, &d.Status, &d.PHP, &d.DocRoot, &d.CreatedAt, &d.Issuer, &d.NotAfter); err != nil {
			return err
		}
		out = append(out, d)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

// accountFor loads an account by id, applying the caller's tenancy.
func (s *Server) accountFor(r *http.Request, id string) (*acct, error) {
	a, err := scanAcct(s.db.QueryRow(r.Context(), acctSelect+` WHERE a.id = ?`, id))
	if err != nil {
		return nil, err
	}
	p := principalFrom(r.Context())
	if p.Role != rbac.Root && a.OwnerID != p.UserID && a.grandOwnerID != p.UserID {
		return nil, store.ErrNotFound
	}
	return a, nil
}

func (s *Server) bulkDomainPHP(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		DomainIDs []string `json:"domain_ids"`
		PHP       string   `json:"php_version"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if err := system.PHPVersion(req.PHP); err != nil {
		return err
	}
	if len(req.DomainIDs) == 0 || len(req.DomainIDs) > 500 {
		return errBadRequest("select between 1 and 500 domains")
	}
	type fail struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}
	done, failed := 0, []fail{}
	for _, id := range req.DomainIDs {
		var accountID string
		if err := s.db.QueryRow(r.Context(), `SELECT account_id FROM domains WHERE id = ?`, id).Scan(&accountID); err != nil {
			failed = append(failed, fail{id, "not found"})
			continue
		}
		a, err := s.accountFor(r, accountID)
		if err != nil {
			failed = append(failed, fail{id, "not found"})
			continue
		}
		if _, err := s.db.Exec(r.Context(), `UPDATE domains SET php_version = ? WHERE id = ?`, req.PHP, id); err != nil {
			return err
		}
		d, err := s.domainOf(r.Context(), a.ID, id)
		if err == nil {
			pa := a.prov()
			pa.PHP = req.PHP
			if err := s.prov.ApplyPHPPool(r.Context(), pa); err != nil {
				failed = append(failed, fail{id, err.Error()})
				continue
			}
			err = s.applyDomain(r.Context(), a, d)
		}
		if err != nil {
			failed = append(failed, fail{id, err.Error()})
			continue
		}
		done++
	}
	s.audit(r, "php.bulk", req.PHP, len(failed) == 0, fmt.Sprintf("%d domains", done))
	writeJSON(w, 200, map[string]any{"updated": done, "failed": failed})
	return nil
}

// runAutoSSL issues certificates for every active domain that has none and
// renews those close to expiry, in the background.
func (s *Server) runAutoSSL(w http.ResponseWriter, r *http.Request) error {
	if s.cfg.DryRun {
		s.audit(r, "autossl.run", "", true, "dry-run")
		writeJSON(w, 202, map[string]any{"queued": 0, "message": "Dry-run: no certificates are requested."})
		return nil
	}
	rows, err := s.db.Query(r.Context(), `SELECT d.id, d.account_id FROM domains d JOIN accounts a ON a.id = d.account_id
		WHERE a.status = 'active' AND NOT EXISTS (SELECT 1 FROM ssl_certs c WHERE c.domain_id = d.id)`)
	if err != nil {
		return err
	}
	type job struct{ d, a string }
	var jobs []job
	for rows.Next() {
		var j job
		if rows.Scan(&j.d, &j.a) == nil {
			jobs = append(jobs, j)
		}
	}
	rows.Close()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()
		s.renewCerts(ctx)
		for _, j := range jobs {
			a, err := scanAcct(s.db.QueryRow(ctx, acctSelect+` WHERE a.id = ?`, j.a))
			if err != nil {
				continue
			}
			d, err := s.domainOf(ctx, a.ID, j.d)
			if err != nil {
				continue
			}
			names := []string{d.Name}
			if d.Kind == "primary" || d.Kind == "addon" {
				names = append(names, "www."+d.Name)
			}
			cert, err := s.prov.IssueLetsEncrypt(ctx, a.Email, names, false)
			if err != nil {
				s.log.Warn("autossl", "domain", d.Name, "err", err)
				continue
			}
			if err := s.saveCert(ctx, a, d, "letsencrypt", names, cert); err == nil {
				_ = s.applyDomain(ctx, a, d)
			}
		}
	}()
	s.audit(r, "autossl.run", "", true, strconv.Itoa(len(jobs))+" domains")
	writeJSON(w, 202, map[string]any{"queued": len(jobs), "message": "AutoSSL is running in the background."})
	return nil
}

func (s *Server) serverBackups(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.db.Query(r.Context(), `SELECT b.id, b.account_id, u.username, a.primary_domain, b.kind, b.size_bytes, b.status, b.created_at
		FROM backups b JOIN accounts a ON a.id = b.account_id JOIN users u ON u.id = a.user_id ORDER BY b.created_at DESC LIMIT 2000`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type v struct {
		ID        string `json:"id"`
		AccountID string `json:"account_id"`
		User      string `json:"user"`
		Domain    string `json:"domain"`
		Kind      string `json:"kind"`
		Size      int64  `json:"size_bytes"`
		Status    string `json:"status"`
		CreatedAt int64  `json:"created_at"`
	}
	out := []v{}
	for rows.Next() {
		var x v
		if err := rows.Scan(&x.ID, &x.AccountID, &x.User, &x.Domain, &x.Kind, &x.Size, &x.Status, &x.CreatedAt); err != nil {
			return err
		}
		out = append(out, x)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) adminSetPassword(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.accountFor(r, r.PathValue("aid"))
	if err != nil {
		return err
	}
	if err := s.setUserPassword(r.Context(), a.UserID, req.Password); err != nil {
		return err
	}
	s.audit(r, "account.password", a.Username, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- server time --------------------------------------------------------------------

var reTZ = regexp.MustCompile(`^[A-Za-z]+(/[A-Za-z0-9_+-]+){0,2}$|^UTC$`)

func (s *Server) timezones(ctx context.Context) []string {
	if !s.cfg.DryRun {
		if res, err := s.prov.Runner().Run(ctx, "timedatectl", "list-timezones"); err == nil {
			return strings.Fields(res.Stdout)
		}
	}
	return []string{"UTC", "Africa/Johannesburg", "America/Chicago", "America/Los_Angeles", "America/New_York", "America/Sao_Paulo", "Asia/Dubai", "Asia/Kolkata", "Asia/Singapore", "Asia/Tokyo", "Australia/Sydney", "Europe/Berlin", "Europe/London", "Europe/Paris"}
}

func (s *Server) getServerTime(w http.ResponseWriter, r *http.Request) error {
	tz := "UTC"
	ntp := true
	if !s.cfg.DryRun {
		if res, err := s.prov.Runner().Run(r.Context(), "timedatectl", "show", "-p", "Timezone", "-p", "NTPSynchronized"); err == nil {
			for _, l := range strings.Split(res.Stdout, "\n") {
				k, v, _ := strings.Cut(strings.TrimSpace(l), "=")
				switch k {
				case "Timezone":
					tz = v
				case "NTPSynchronized":
					ntp = v == "yes"
				}
			}
		}
	} else if v := s.setting(r.Context(), "dev_timezone"); v != "" {
		tz = v
	}
	now := time.Now()
	if loc, err := time.LoadLocation(tz); err == nil {
		now = now.In(loc)
	}
	writeJSON(w, 200, map[string]any{"timezone": tz, "now": now.Format(time.RFC3339), "unix": now.Unix(), "ntp_synced": ntp, "timezones": s.timezones(r.Context())})
	return nil
}

func (s *Server) putServerTime(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Timezone string `json:"timezone"`
		SyncNow  bool   `json:"sync_now"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if !reTZ.MatchString(req.Timezone) {
		return errBadRequest("choose a time zone from the list")
	}
	found := false
	for _, t := range s.timezones(r.Context()) {
		found = found || t == req.Timezone
	}
	if !found {
		return errBadRequest("unknown time zone")
	}
	if s.cfg.DryRun {
		if err := s.putSetting(r.Context(), "dev_timezone", req.Timezone); err != nil {
			return err
		}
	} else {
		if _, err := s.prov.Runner().Run(r.Context(), "timedatectl", "set-timezone", req.Timezone); err != nil {
			return err
		}
		if req.SyncNow {
			_, _ = s.prov.Runner().Run(r.Context(), "systemctl", "restart", "systemd-timesyncd")
		}
	}
	s.audit(r, "server.timezone", req.Timezone, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- SSH daemon ---------------------------------------------------------------------

const sshTweaks = "/etc/ssh/sshd_config.d/40-breefercp-tweaks.conf"

type sshConfig struct {
	PasswordAuth bool   `json:"password_auth"`
	RootLogin    string `json:"root_login"` // yes | prohibit-password | no
	Port         int    `json:"port"`
}

func (s *Server) sshConfig(ctx context.Context) sshConfig {
	c := sshConfig{PasswordAuth: true, RootLogin: "prohibit-password", Port: 22}
	if s.cfg.DryRun {
		if v := s.setting(ctx, "dev_ssh"); v != "" {
			parts := strings.Split(v, ",")
			if len(parts) == 3 {
				c.PasswordAuth = parts[0] == "1"
				c.RootLogin = parts[1]
				c.Port, _ = strconv.Atoi(parts[2])
			}
		}
		return c
	}
	res, err := s.prov.Runner().Run(ctx, "sshd", "-T")
	if err != nil {
		return c
	}
	for _, l := range strings.Split(res.Stdout, "\n") {
		k, v, _ := strings.Cut(strings.TrimSpace(l), " ")
		switch k {
		case "passwordauthentication":
			c.PasswordAuth = v == "yes"
		case "permitrootlogin":
			c.RootLogin = map[string]string{"without-password": "prohibit-password"}[v]
			if c.RootLogin == "" {
				c.RootLogin = v
			}
		case "port":
			c.Port, _ = strconv.Atoi(v)
		}
	}
	return c
}

func (s *Server) getSSHConfig(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, 200, s.sshConfig(r.Context()))
	return nil
}

func (s *Server) putSSHConfig(w http.ResponseWriter, r *http.Request) error {
	var c sshConfig
	if err := decode(r, &c); err != nil {
		return err
	}
	switch c.RootLogin {
	case "yes", "prohibit-password", "no":
	default:
		return errBadRequest("root login must be yes, prohibit-password or no")
	}
	if c.Port != 22 && (c.Port < 1024 || c.Port > 65535) {
		return errBadRequest("use port 22 or a port between 1024 and 65535")
	}
	if !c.PasswordAuth && c.RootLogin != "no" && !s.cfg.DryRun {
		// Refuse to lock root out: key login must actually be possible.
		if keys, _ := s.rootKeys(r.Context()); len(keys) == 0 && c.RootLogin == "prohibit-password" {
			return errBadRequest("add a root SSH key before turning off password logins")
		}
	}
	if s.cfg.DryRun {
		if err := s.putSetting(r.Context(), "dev_ssh", fmt.Sprintf("%d,%s,%d", b2i(c.PasswordAuth), c.RootLogin, c.Port)); err != nil {
			return err
		}
	} else {
		pw := "no"
		if c.PasswordAuth {
			pw = "yes"
		}
		conf := fmt.Sprintf("# Managed by BreeferCP (WHM > SSH Settings).\nPort %d\nPasswordAuthentication %s\nKbdInteractiveAuthentication %s\nPermitRootLogin %s\n", c.Port, pw, pw, c.RootLogin)
		old, _ := os.ReadFile(sshTweaks)
		if err := s.prov.Runner().WriteFile(sshTweaks, []byte(conf), 0o644); err != nil {
			return err
		}
		if _, err := s.prov.Runner().Run(r.Context(), "sshd", "-t"); err != nil {
			_ = s.prov.Runner().WriteFile(sshTweaks, old, 0o644)
			return fmt.Errorf("sshd rejected the configuration: %w", err)
		}
		if c.Port != 22 {
			// Open the new port before sshd moves there.
			_, _ = s.db.Exec(r.Context(), `INSERT INTO firewall_rules (id, action, protocol, port, source, comment, created_at) SELECT ?, 'allow', 'tcp', ?, '', 'SSH', ? WHERE NOT EXISTS (SELECT 1 FROM firewall_rules WHERE port = ? AND protocol = 'tcp')`,
				fmt.Sprintf("ssh%d", c.Port), strconv.Itoa(c.Port), time.Now().Unix(), strconv.Itoa(c.Port))
			_ = s.applyFirewall(r.Context())
		}
		if _, err := s.prov.Runner().Run(r.Context(), "systemctl", "reload", "ssh"); err != nil {
			_, _ = s.prov.Runner().Run(r.Context(), "systemctl", "reload", "sshd")
		}
	}
	s.audit(r, "server.ssh", "", true, fmt.Sprintf("password=%v root=%s port=%d", c.PasswordAuth, c.RootLogin, c.Port))
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- root SSH keys ------------------------------------------------------------------

const (
	rootKeysFile = "/root/.ssh/authorized_keys"
	keysBegin    = "# BEGIN BreeferCP"
	keysEnd      = "# END BreeferCP"
)

type rootKey struct {
	Fingerprint string `json:"fingerprint"`
	Type        string `json:"type"`
	Comment     string `json:"comment"`
	Managed     bool   `json:"managed"`
	line        string
}

func (s *Server) rootKeys(ctx context.Context) ([]rootKey, error) {
	var data []byte
	if s.cfg.DryRun {
		data = []byte(s.setting(ctx, "dev_root_keys"))
	} else {
		b, err := os.ReadFile(rootKeysFile)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		data = b
	}
	var out []rootKey
	managed := false
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimSpace(l)
		switch {
		case l == keysBegin:
			managed = true
			continue
		case l == keysEnd:
			managed = false
			continue
		case l == "" || strings.HasPrefix(l, "#"):
			continue
		}
		pub, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(l))
		if err != nil {
			continue
		}
		out = append(out, rootKey{Fingerprint: ssh.FingerprintSHA256(pub), Type: pub.Type(), Comment: comment, Managed: managed, line: l})
	}
	return out, nil
}

func (s *Server) writeRootKeys(ctx context.Context, keys []rootKey) error {
	var unmanaged, managed []string
	for _, k := range keys {
		if k.Managed {
			managed = append(managed, k.line)
		} else {
			unmanaged = append(unmanaged, k.line)
		}
	}
	body := strings.Join(unmanaged, "\n")
	if body != "" {
		body += "\n"
	}
	body += keysBegin + "\n" + strings.Join(managed, "\n")
	if len(managed) > 0 {
		body += "\n"
	}
	body += keysEnd + "\n"
	if s.cfg.DryRun {
		return s.putSetting(ctx, "dev_root_keys", body)
	}
	if _, err := s.prov.Runner().Run(ctx, "install", "-d", "-m", "0700", "/root/.ssh"); err != nil {
		return err
	}
	return s.prov.Runner().WriteFile(rootKeysFile, []byte(body), 0o600)
}

func (s *Server) listRootKeys(w http.ResponseWriter, r *http.Request) error {
	keys, err := s.rootKeys(r.Context())
	if err != nil {
		return err
	}
	if keys == nil {
		keys = []rootKey{}
	}
	writeJSON(w, 200, keys)
	return nil
}

func (s *Server) addRootKey(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		PublicKey string `json:"public_key"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	line := strings.TrimSpace(req.PublicKey)
	if strings.ContainsAny(line, "\r\n") {
		return errBadRequest("paste a single public key line")
	}
	pub, comment, opts, _, err := ssh.ParseAuthorizedKey([]byte(line))
	if err != nil || len(opts) > 0 {
		return errBadRequest("not a valid OpenSSH public key (options are not allowed)")
	}
	keys, err := s.rootKeys(r.Context())
	if err != nil {
		return err
	}
	fp := ssh.FingerprintSHA256(pub)
	for _, k := range keys {
		if k.Fingerprint == fp {
			return &apiError{409, "conflict", "this key is already authorized"}
		}
	}
	norm := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
	if comment != "" && system.OneLine(comment, 100) == nil {
		norm += " " + comment
	}
	keys = append(keys, rootKey{Fingerprint: fp, Managed: true, line: norm})
	if err := s.writeRootKeys(r.Context(), keys); err != nil {
		return err
	}
	s.audit(r, "server.root_key_add", fp, true, comment)
	w.WriteHeader(http.StatusCreated)
	return nil
}

func (s *Server) deleteRootKey(w http.ResponseWriter, r *http.Request) error {
	fp := r.PathValue("fp")
	keys, err := s.rootKeys(r.Context())
	if err != nil {
		return err
	}
	out := keys[:0]
	found := false
	for _, k := range keys {
		if k.Fingerprint == fp {
			found = true
			continue
		}
		out = append(out, k)
	}
	if !found {
		return store.ErrNotFound
	}
	if !s.sshConfig(r.Context()).PasswordAuth && len(out) == 0 {
		return errBadRequest("password logins are off; this is the last key, so removing it would lock root out")
	}
	if err := s.writeRootKeys(r.Context(), out); err != nil {
		return err
	}
	s.audit(r, "server.root_key_delete", fp, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- root password -------------------------------------------------------------------

func (s *Server) rootPassword(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		CurrentPassword string `json:"current_password"`
		Password        string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	p := principalFrom(r.Context())
	u, err := s.userByID(r.Context(), p.UserID)
	if err != nil {
		return err
	}
	// Re-authenticate: a hijacked session alone must not reset root.
	if !auth.VerifyPassword(req.CurrentPassword, u.PasswordHash) {
		s.audit(r, "server.root_password", "", false, "wrong panel password")
		return &apiError{403, "forbidden", "your panel password is incorrect"}
	}
	if err := auth.CheckPasswordPolicy(req.Password); err != nil {
		return errBadRequest(err.Error())
	}
	if strings.ContainsAny(req.Password, ":\r\n") {
		return errBadRequest("the password cannot contain a colon or line break")
	}
	if !s.cfg.DryRun {
		if _, err := s.prov.Runner().RunInput(r.Context(), []byte("root:"+req.Password+"\n"), "chpasswd"); err != nil {
			return err
		}
	}
	s.audit(r, "server.root_password", "", true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- security advisor -------------------------------------------------------------------

type advice struct {
	ID     string `json:"id"`
	Status string `json:"status"` // ok | warn | bad | info
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Fix    string `json:"fix"` // UI route that fixes it
}

func (s *Server) securityAdvisor(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var out []advice
	add := func(id, status, title, detail, fix string) {
		out = append(out, advice{id, status, title, detail, fix})
	}

	sc := s.sshConfig(ctx)
	switch sc.RootLogin {
	case "yes":
		add("ssh_root", "bad", "Root can log in over SSH with a password", "Set root login to “keys only” or “no”, and use a normal user with sudo.", "/ssh")
	case "prohibit-password":
		add("ssh_root", "ok", "Root SSH login requires a key", "", "/ssh")
	default:
		add("ssh_root", "ok", "Root SSH login is disabled", "", "/ssh")
	}
	if sc.PasswordAuth {
		add("ssh_password", "warn", "SSH accepts passwords", "Password logins can be brute-forced. Add SSH keys, then turn off password authentication.", "/ssh")
	} else {
		add("ssh_password", "ok", "SSH accepts keys only", "", "/ssh")
	}
	if sc.Port == 22 {
		add("ssh_port", "info", "SSH listens on port 22", "A different port does not add real security but cuts down log noise from scanners.", "/ssh")
	}

	p := principalFrom(ctx)
	if !p.TOTP {
		add("root_2fa", "bad", "Your administrator login has no two-factor authentication", "Anyone with your password can control the whole server.", "/two-factor")
	} else {
		add("root_2fa", "ok", "Two-factor authentication is on for your login", "", "/two-factor")
	}
	switch s.setting(ctx, "require_2fa") {
	case "all":
		add("policy_2fa", "ok", "Two-factor authentication is required for everyone", "", "/security")
	case "admins":
		add("policy_2fa", "ok", "Two-factor authentication is required for administrators", "", "/security")
	default:
		add("policy_2fa", "warn", "Two-factor authentication is optional", "Require it at least for administrators and resellers.", "/security")
	}
	if s.setting(ctx, "whm_allowed_ips") == "" {
		add("whm_ips", "info", "WHM is reachable from any address", "If your team works from fixed addresses, restrict WHM to them.", "/security")
	} else {
		add("whm_ips", "ok", "WHM is limited to an IP allow list", "", "/security")
	}
	if s.cfg.TLSCert == "" {
		add("panel_tls", "bad", "The panel is served over plain HTTP", "Passwords and sessions travel unencrypted. Set tls_cert and tls_key in breefercp.conf.", "/settings")
	} else {
		add("panel_tls", "ok", "The panel uses HTTPS", "", "")
	}

	fw := s.cfg.DryRun
	if !s.cfg.DryRun && runtime.GOOS == "linux" {
		if res, err := s.prov.Runner().Run(ctx, "nft", "list", "table", "inet", "breefercp"); err == nil && strings.Contains(res.Stdout, "chain") {
			fw = true
		}
	}
	if fw {
		add("firewall", "ok", "The firewall is active", "", "/firewall")
	} else {
		add("firewall", "bad", "The firewall is not loaded", "Open Firewall and save the rules to load them.", "/firewall")
	}

	f2b := s.cfg.DryRun
	if !s.cfg.DryRun {
		if res, _ := s.prov.Runner().Run(ctx, "systemctl", "is-active", "fail2ban"); strings.TrimSpace(res.Stdout) == "active" {
			f2b = true
		}
	}
	if f2b {
		add("fail2ban", "ok", "fail2ban is running", "Repeated SSH failures are banned at the firewall.", "/services")
	} else {
		add("fail2ban", "warn", "fail2ban is not running", "Start it in Service Manager to ban addresses that brute-force SSH.", "/services")
	}

	if s.setting(ctx, "backup_enabled") == "1" {
		add("backups", "ok", "Scheduled backups are on", "Copy them off the server too: a backup on the same disk does not survive a disk failure.", "/backup-config")
	} else {
		add("backups", "warn", "Scheduled backups are off", "Turn on daily or weekly backups for all accounts.", "/backup-config")
	}
	switch s.setting(ctx, "update_tier") {
	case "auto":
		add("updates", "ok", "BreeferCP updates install automatically", "", "/updates")
	default:
		add("updates", "info", "BreeferCP updates are manual", "Check the Updates page regularly, or switch to automatic updates.", "/updates")
	}

	var shells int
	_ = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM accounts WHERE shell = 'full'`).Scan(&shells)
	if shells > 0 {
		add("shells", "info", fmt.Sprintf("%d account(s) have full shell access", shells), "Shell users can run any program inside their limits. Prefer SFTP-only where you can.", "/accounts")
	}
	var weak int
	_ = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE role IN ('root','reseller') AND totp_enabled = 0`).Scan(&weak)
	if weak > 0 {
		add("admin_2fa", "warn", fmt.Sprintf("%d administrator or reseller login(s) without two-factor authentication", weak), "Require two-factor authentication for administrators in Security Policy.", "/security")
	}

	rank := map[string]int{"bad": 0, "warn": 1, "info": 2, "ok": 3}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Status] < rank[out[j].Status] })
	writeJSON(w, 200, out)
	return nil
}

// ---- reboot -------------------------------------------------------------------------------

func (s *Server) reboot(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Mode string `json:"mode"` // graceful | force
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	args := []string{"--on-active=5", "--unit=breefercp-reboot", "systemctl", "reboot"}
	switch req.Mode {
	case "graceful":
	case "force":
		args = append(args, "--force")
	default:
		return errBadRequest("mode must be graceful or force")
	}
	s.audit(r, "server.reboot", req.Mode, true, "")
	if !s.cfg.DryRun {
		if _, err := s.prov.Runner().Run(r.Context(), "systemd-run", args...); err != nil {
			return err
		}
	}
	writeJSON(w, 202, map[string]any{"in_seconds": 5, "dry_run": s.cfg.DryRun})
	return nil
}
