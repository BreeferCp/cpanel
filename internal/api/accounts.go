package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/auth"
	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/provision"
	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/store"
	"github.com/breefercp/breefercp/internal/system"
)

// ---- packages ---------------------------------------------------------------

type pkg struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	DiskMB       int    `json:"disk_mb"`
	BandwidthMB  int    `json:"bandwidth_mb"`
	MaxDomains   int    `json:"max_domains"`
	MaxDatabases int    `json:"max_databases"`
	MaxMailboxes int    `json:"max_mailboxes"`
	MaxCron      int    `json:"max_cron"`
	MaxApps      int    `json:"max_apps"`
	CPUPercent   int    `json:"cpu_percent"`
	MemoryMB     int    `json:"memory_mb"`
	IOWeight     int    `json:"io_weight"`
	MaxProcs     int    `json:"max_procs"`
	PHPVersions  string `json:"php_versions"`
	ShellAccess  bool   `json:"shell_access"`
	MaxFTP       int    `json:"max_ftp"`
	FeatureList  string `json:"feature_list_id"`
}

const pkgCols = `id, name, disk_mb, bandwidth_mb, max_domains, max_databases, max_mailboxes, max_cron, max_apps,
	cpu_percent, memory_mb, io_weight, max_procs, php_versions, shell_access, max_ftp, feature_list_id`

func scanPkg(row rowScanner) (pkg, error) {
	var p pkg
	var shell int
	err := row.Scan(&p.ID, &p.Name, &p.DiskMB, &p.BandwidthMB, &p.MaxDomains, &p.MaxDatabases, &p.MaxMailboxes,
		&p.MaxCron, &p.MaxApps, &p.CPUPercent, &p.MemoryMB, &p.IOWeight, &p.MaxProcs, &p.PHPVersions, &shell, &p.MaxFTP, &p.FeatureList)
	p.ShellAccess = shell == 1
	return p, store.IsNoRows(err)
}

func (p *pkg) validate() error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" || len(p.Name) > 64 {
		return errBadRequest("package name is required (max 64 characters)")
	}
	for _, v := range []int{p.DiskMB, p.BandwidthMB, p.MaxDomains, p.MaxDatabases, p.MaxMailboxes, p.MaxCron, p.MaxApps, p.MaxFTP} {
		if v < 0 {
			return errBadRequest("limits cannot be negative (0 means unlimited)")
		}
	}
	if p.CPUPercent < 10 || p.CPUPercent > 6400 {
		return errBadRequest("CPU must be between 10% and 6400%")
	}
	if p.MemoryMB < 128 || p.MemoryMB > 1<<20 {
		return errBadRequest("memory must be at least 128 MB")
	}
	if p.IOWeight < 1 || p.IOWeight > 10000 {
		p.IOWeight = 100
	}
	if p.MaxProcs < 20 {
		return errBadRequest("process limit must be at least 20")
	}
	if p.PHPVersions == "" {
		p.PHPVersions = "8.3"
	}
	for _, v := range strings.Split(p.PHPVersions, ",") {
		if err := system.PHPVersion(strings.TrimSpace(v)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) listPackages(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	owner := p.UserID
	if p.Role == rbac.Root {
		owner = "" // root sees every package
	}
	q := `SELECT ` + pkgCols + ` FROM packages`
	var args []any
	if owner != "" {
		q += ` WHERE owner_id = ?`
		args = append(args, owner)
	}
	rows, err := s.db.Query(r.Context(), q+` ORDER BY name`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []pkg{}
	for rows.Next() {
		v, err := scanPkg(rows)
		if err != nil {
			return err
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) createPackage(w http.ResponseWriter, r *http.Request) error {
	var req pkg
	if err := decode(r, &req); err != nil {
		return err
	}
	if err := req.validate(); err != nil {
		return err
	}
	p := principalFrom(r.Context())
	if req.FeatureList != "" {
		if err := s.ownedFeatureList(r.Context(), p, req.FeatureList); err != nil {
			return errBadRequest("unknown feature list")
		}
	}
	req.ID = ids.New()
	shell := 0
	if req.ShellAccess {
		shell = 1
	}
	_, err := s.db.Exec(r.Context(), `INSERT INTO packages (id, owner_id, name, disk_mb, bandwidth_mb, max_domains, max_databases,
		max_mailboxes, max_cron, max_apps, cpu_percent, memory_mb, io_weight, max_procs, php_versions, shell_access, max_ftp, feature_list_id, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		req.ID, p.UserID, req.Name, req.DiskMB, req.BandwidthMB, req.MaxDomains, req.MaxDatabases, req.MaxMailboxes,
		req.MaxCron, req.MaxApps, req.CPUPercent, req.MemoryMB, req.IOWeight, req.MaxProcs, req.PHPVersions, shell, req.MaxFTP, req.FeatureList, time.Now().Unix())
	if err != nil {
		return err
	}
	s.audit(r, "package.create", req.Name, true, "")
	writeJSON(w, 201, req)
	return nil
}

func (s *Server) ownedPackage(ctx context.Context, p *Principal, id string) (pkg, error) {
	q := `SELECT ` + pkgCols + ` FROM packages WHERE id = ?`
	args := []any{id}
	if p.Role != rbac.Root {
		q += ` AND owner_id = ?`
		args = append(args, p.UserID)
	}
	return scanPkg(s.db.QueryRow(ctx, q, args...))
}

func (s *Server) updatePackage(w http.ResponseWriter, r *http.Request) error {
	var req pkg
	if err := decode(r, &req); err != nil {
		return err
	}
	if err := req.validate(); err != nil {
		return err
	}
	p := principalFrom(r.Context())
	if _, err := s.ownedPackage(r.Context(), p, r.PathValue("id")); err != nil {
		return err
	}
	if req.FeatureList != "" {
		if err := s.ownedFeatureList(r.Context(), p, req.FeatureList); err != nil {
			return errBadRequest("unknown feature list")
		}
	}
	shell := 0
	if req.ShellAccess {
		shell = 1
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE packages SET name=?, disk_mb=?, bandwidth_mb=?, max_domains=?, max_databases=?,
		max_mailboxes=?, max_cron=?, max_apps=?, cpu_percent=?, memory_mb=?, io_weight=?, max_procs=?, php_versions=?, shell_access=?,
		max_ftp=?, feature_list_id=? WHERE id=?`,
		req.Name, req.DiskMB, req.BandwidthMB, req.MaxDomains, req.MaxDatabases, req.MaxMailboxes, req.MaxCron, req.MaxApps,
		req.CPUPercent, req.MemoryMB, req.IOWeight, req.MaxProcs, req.PHPVersions, shell, req.MaxFTP, req.FeatureList, r.PathValue("id")); err != nil {
		return err
	}
	// Re-apply cgroup limits to every account on this package.
	rows, err := s.db.Query(r.Context(), `SELECT sys_user, sys_uid, home_dir FROM accounts WHERE package_id = ? AND status = 'active'`, r.PathValue("id"))
	if err != nil {
		return err
	}
	var accts []provision.Account
	for rows.Next() {
		var a provision.Account
		if err := rows.Scan(&a.SysUser, &a.UID, &a.Home); err != nil {
			rows.Close()
			return err
		}
		a.Limits = limitsOf(req)
		accts = append(accts, a)
	}
	rows.Close()
	for _, a := range accts {
		if err := s.prov.SetLimits(r.Context(), a); err != nil {
			s.log.Error("reapply limits", "user", a.SysUser, "err", err)
		}
	}
	s.audit(r, "package.update", req.Name, true, "")
	req.ID = r.PathValue("id")
	writeJSON(w, 200, req)
	return nil
}

func (s *Server) deletePackage(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	if _, err := s.ownedPackage(r.Context(), p, r.PathValue("id")); err != nil {
		return err
	}
	var n int
	if err := s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM accounts WHERE package_id = ?`, r.PathValue("id")).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return errConflict(fmt.Sprintf("%d account(s) still use this package", n))
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM packages WHERE id = ?`, r.PathValue("id")); err != nil {
		return err
	}
	s.audit(r, "package.delete", r.PathValue("id"), true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func limitsOf(p pkg) provision.Limits {
	return provision.Limits{CPUPercent: p.CPUPercent, MemoryMB: p.MemoryMB, IOWeight: p.IOWeight, MaxProcs: p.MaxProcs, DiskMB: p.DiskMB}
}

// ---- accounts ---------------------------------------------------------------

type acct struct {
	ID            string   `json:"id"`
	UserID        string   `json:"user_id"`
	Username      string   `json:"username"`
	Email         string   `json:"email"`
	PackageID     string   `json:"package_id"`
	PackageName   string   `json:"package_name"`
	SysUser       string   `json:"sys_user"`
	SysUID        int      `json:"sys_uid"`
	HomeDir       string   `json:"home_dir"`
	PrimaryDomain string   `json:"primary_domain"`
	Status        string   `json:"status"`
	SuspendReason string   `json:"suspend_reason"`
	ExternalRef   string   `json:"external_ref"`
	DiskUsedMB    int      `json:"disk_used_mb"`
	BWUsedMB      int      `json:"bw_used_mb"`
	OwnerID       string   `json:"owner_id"`
	OwnerName     string   `json:"owner_name"`
	Shell         string   `json:"shell"`
	Disabled      []string `json:"disabled_features"`
	CreatedAt     int64    `json:"created_at"`
	grandOwnerID  string
	phpIniJSON    string
	pkg           pkg
}

const acctSelect = `SELECT a.id, a.user_id, u.username, u.email, a.package_id, k.name, a.sys_user, a.sys_uid, a.home_dir,
	a.primary_domain, a.status, a.suspend_reason, a.external_ref, a.disk_used_mb, a.bw_used_mb, COALESCE(u.owner_id,''), a.created_at,
	COALESCE(o.owner_id,''), COALESCE(o.username,''), a.shell,
	k.cpu_percent, k.memory_mb, k.io_weight, k.max_procs, k.php_versions, k.disk_mb, k.max_domains, k.max_databases, k.max_mailboxes, k.max_cron, k.max_apps, k.shell_access,
	COALESCE((SELECT ps.settings FROM php_settings ps WHERE ps.account_id = a.id), '{}'),
	k.max_ftp, COALESCE((SELECT fl.disabled FROM feature_lists fl WHERE fl.id = k.feature_list_id), '')
	FROM accounts a JOIN users u ON u.id = a.user_id JOIN packages k ON k.id = a.package_id LEFT JOIN users o ON o.id = u.owner_id`

func scanAcct(row rowScanner) (*acct, error) {
	var a acct
	var shell int
	var disabled string
	err := row.Scan(&a.ID, &a.UserID, &a.Username, &a.Email, &a.PackageID, &a.PackageName, &a.SysUser, &a.SysUID, &a.HomeDir,
		&a.PrimaryDomain, &a.Status, &a.SuspendReason, &a.ExternalRef, &a.DiskUsedMB, &a.BWUsedMB, &a.OwnerID, &a.CreatedAt,
		&a.grandOwnerID, &a.OwnerName, &a.Shell,
		&a.pkg.CPUPercent, &a.pkg.MemoryMB, &a.pkg.IOWeight, &a.pkg.MaxProcs, &a.pkg.PHPVersions, &a.pkg.DiskMB,
		&a.pkg.MaxDomains, &a.pkg.MaxDatabases, &a.pkg.MaxMailboxes, &a.pkg.MaxCron, &a.pkg.MaxApps, &shell, &a.phpIniJSON,
		&a.pkg.MaxFTP, &disabled)
	a.pkg.ShellAccess = shell == 1
	a.Disabled = strings.Fields(disabled)
	return &a, store.IsNoRows(err)
}

func (a *acct) prov() provision.Account {
	php := strings.Split(a.pkg.PHPVersions, ",")[0]
	var ini map[string]string
	_ = json.Unmarshal([]byte(a.phpIniJSON), &ini)
	return provision.Account{SysUser: a.SysUser, UID: a.SysUID, Home: a.HomeDir, PHP: strings.TrimSpace(php), Limits: limitsOf(a.pkg), PHPIni: ini}
}

// loadAccount resolves {aid} ("me" allowed) and enforces tenancy:
// root sees everything, a reseller only its customers, a user only itself.
func (s *Server) loadAccount(r *http.Request) (*acct, error) {
	p := principalFrom(r.Context())
	aid := r.PathValue("aid")
	var (
		a   *acct
		err error
	)
	if aid == "me" {
		a, err = scanAcct(s.db.QueryRow(r.Context(), acctSelect+` WHERE a.user_id = ?`, p.UserID))
	} else {
		a, err = scanAcct(s.db.QueryRow(r.Context(), acctSelect+` WHERE a.id = ?`, aid))
	}
	if err != nil {
		return nil, err
	}
	switch p.Role {
	case rbac.Root:
	case rbac.Reseller:
		// Owned directly, or through one of this master reseller's resellers.
		if a.OwnerID != p.UserID && a.grandOwnerID != p.UserID && a.UserID != p.UserID {
			return nil, store.ErrNotFound // do not reveal existence across tenants
		}
	default:
		if a.UserID != p.UserID {
			return nil, store.ErrNotFound
		}
	}
	return a, nil
}

func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	q := acctSelect
	var args []any
	if p.Role == rbac.Reseller {
		q += ` WHERE (u.owner_id = ? OR o.owner_id = ?)`
		args = append(args, p.UserID, p.UserID)
	}
	if search := strings.TrimSpace(r.URL.Query().Get("q")); search != "" {
		if len(args) == 0 {
			q += ` WHERE`
		} else {
			q += ` AND`
		}
		q += ` (a.primary_domain LIKE ? OR a.sys_user LIKE ? OR u.email LIKE ?)`
		like := "%" + strings.NewReplacer("%", "", "_", "").Replace(search) + "%"
		args = append(args, like, like, like)
	}
	rows, err := s.db.Query(r.Context(), q+` ORDER BY a.created_at DESC LIMIT 1000`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []*acct{}
	for rows.Next() {
		a, err := scanAcct(rows)
		if err != nil {
			return err
		}
		out = append(out, a)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) getAccount(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	writeJSON(w, 200, a)
	return nil
}

// CreateAccountParams drives account creation from the panel or a billing API.
type CreateAccountParams struct {
	Username    string `json:"username"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	Domain      string `json:"domain"`
	PackageID   string `json:"package_id"`
	ExternalRef string `json:"external_ref"`
}

type createdAccount struct {
	Account  *acct  `json:"account"`
	Password string `json:"password,omitempty"` // only populated when generated
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) error {
	var req CreateAccountParams
	if err := decode(r, &req); err != nil {
		return err
	}
	res, err := s.provisionAccount(r.Context(), principalFrom(r.Context()), req)
	if err != nil {
		s.audit(r, "account.create", req.Username, false, err.Error())
		return err
	}
	s.audit(r, "account.create", req.Username, true, req.Domain)
	writeJSON(w, 201, res)
	return nil
}

// provisionAccount is shared by the panel and the billing API. It writes the
// database rows first, then applies host changes; if the host step fails the
// rows are removed again so no half-created account remains.
func (s *Server) provisionAccount(ctx context.Context, owner *Principal, req CreateAccountParams) (*createdAccount, error) {
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	req.Domain = strings.ToLower(strings.TrimSpace(req.Domain))
	if err := system.SysUser(req.Username); err != nil {
		return nil, err
	}
	if err := system.Domain(req.Domain); err != nil {
		return nil, err
	}
	if !strings.Contains(req.Email, "@") || len(req.Email) > 254 {
		return nil, errBadRequest("enter a valid email address")
	}
	pk, err := s.ownedPackage(ctx, owner, req.PackageID)
	if err != nil {
		return nil, errBadRequest("unknown package")
	}
	if err := s.claimDomain(ctx, owner, "", req.Domain); err != nil {
		return nil, err
	}
	// An existing system user must stop this here: the rollback of a failed
	// useradd runs "userdel --remove" on that name.
	if !s.cfg.DryRun {
		if _, err := s.prov.Runner().Run(ctx, "id", "-u", "--", req.Username); err == nil {
			return nil, errConflict("that username already exists on the server")
		}
	}
	if err := s.checkResellerAllocation(ctx, owner, pk.DiskMB, true); err != nil {
		return nil, err
	}
	generated := ""
	if req.Password == "" {
		if generated, err = auth.RandomPassword(20); err != nil {
			return nil, err
		}
		req.Password = generated
	}
	if err := auth.CheckPasswordPolicy(req.Password); err != nil {
		return nil, errBadRequest(err.Error())
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	userID, acctID, now := ids.New(), ids.New(), time.Now().Unix()
	home := fmt.Sprintf("%s/%s", strings.TrimRight(s.cfg.HomeBase, "/"), req.Username)
	var uid int
	err = s.db.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(sys_uid), 19999) + 1 FROM accounts`).Scan(&uid); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO users (id, username, email, password_hash, role, owner_id, created_at, updated_at)
			VALUES (?,?,?,?,'user',?,?,?)`, userID, req.Username, req.Email, hash, owner.UserID, now, now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO accounts (id, user_id, package_id, sys_user, sys_uid, home_dir, primary_domain, external_ref, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`, acctID, userID, pk.ID, req.Username, uid, home, req.Domain, req.ExternalRef, now, now); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO domains (id, account_id, name, kind, doc_root, php_version, created_at) VALUES (?,?,?,'primary',?,?,?)`,
			ids.New(), acctID, req.Domain, home+"/public_html", firstPHP(pk.PHPVersions), now)
		return err
	})
	if err != nil {
		if store.IsUnique(err) {
			return nil, errConflict("username or domain is already in use")
		}
		return nil, err
	}

	pa := provision.Account{SysUser: req.Username, UID: uid, Home: home, PHP: firstPHP(pk.PHPVersions), Limits: limitsOf(pk)}
	if err := s.applyNewAccount(ctx, pa, req.Domain); err != nil {
		s.log.Error("provisioning failed, rolling back", "user", req.Username, "err", err)
		_ = s.prov.DeleteAccount(context.WithoutCancel(ctx), pa, []string{req.Domain})
		_, _ = s.db.Exec(context.WithoutCancel(ctx), `DELETE FROM users WHERE id = ?`, userID) // cascades to account + domain
		return nil, fmt.Errorf("provision account: %w", err)
	}
	a, err := scanAcct(s.db.QueryRow(ctx, acctSelect+` WHERE a.id = ?`, acctID))
	if err != nil {
		return nil, err
	}
	return &createdAccount{Account: a, Password: generated}, nil
}

func (s *Server) applyNewAccount(ctx context.Context, pa provision.Account, domain string) error {
	if err := s.prov.CreateAccount(ctx, pa); err != nil {
		return err
	}
	if err := s.prov.SetPassword(ctx, pa.SysUser, mustRandom()); err != nil { // system password is unusable; panel auth is separate
		return err
	}
	if err := s.prov.WriteVhost(ctx, provision.Vhost{Domain: domain, Aliases: []string{"www." + domain}}, pa); err != nil {
		return err
	}
	return s.prov.DNS().CreateZone(ctx, domain, s.serverIP(ctx), s.nameservers(ctx))
}

func mustRandom() string {
	p, err := auth.RandomPassword(32)
	if err != nil {
		panic(err)
	}
	return p
}

func firstPHP(list string) string { return strings.TrimSpace(strings.Split(list, ",")[0]) }

func (s *Server) updateAccount(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		PackageID *string `json:"package_id"`
		Email     *string `json:"email"`
		Password  *string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	p := principalFrom(r.Context())
	if req.PackageID != nil && *req.PackageID != a.PackageID {
		pk, err := s.ownedPackage(r.Context(), p, *req.PackageID)
		if err != nil {
			return errBadRequest("unknown package")
		}
		if err := s.checkResellerAllocation(r.Context(), p, pk.DiskMB-a.pkg.DiskMB, false); err != nil && pk.DiskMB > a.pkg.DiskMB {
			return err
		}
		if _, err := s.db.Exec(r.Context(), `UPDATE accounts SET package_id=?, updated_at=? WHERE id=?`, pk.ID, time.Now().Unix(), a.ID); err != nil {
			return err
		}
		a.pkg, a.PackageID = pk, pk.ID
		if err := s.prov.SetLimits(r.Context(), a.prov()); err != nil {
			return err
		}
		if err := s.prov.ApplyPHPPool(r.Context(), a.prov()); err != nil {
			return err
		}
	}
	if req.Email != nil {
		if !strings.Contains(*req.Email, "@") {
			return errBadRequest("enter a valid email address")
		}
		if _, err := s.db.Exec(r.Context(), `UPDATE users SET email=? WHERE id=?`, *req.Email, a.UserID); err != nil {
			return err
		}
	}
	if req.Password != nil {
		if err := s.setUserPassword(r.Context(), a.UserID, *req.Password); err != nil {
			return err
		}
	}
	s.audit(r, "account.update", a.SysUser, true, "")
	a, err = scanAcct(s.db.QueryRow(r.Context(), acctSelect+` WHERE a.id = ?`, a.ID))
	if err != nil {
		return err
	}
	writeJSON(w, 200, a)
	return nil
}

func (s *Server) setUserPassword(ctx context.Context, userID, password string) error {
	if err := auth.CheckPasswordPolicy(password); err != nil {
		return errBadRequest(err.Error())
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `UPDATE users SET password_hash=?, updated_at=? WHERE id=?`, hash, time.Now().Unix(), userID); err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (s *Server) accountDomains(ctx context.Context, accountID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT name FROM domains WHERE account_id = ?`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Server) suspendAccount(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Reason string `json:"reason"`
	}
	_ = decode(r, &req) // body optional
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if err := s.doSuspend(r.Context(), a, req.Reason); err != nil {
		return err
	}
	s.audit(r, "account.suspend", a.SysUser, true, req.Reason)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) doSuspend(ctx context.Context, a *acct, reason string) error {
	doms, err := s.accountDomains(ctx, a.ID)
	if err != nil {
		return err
	}
	if err := s.prov.Suspend(ctx, a.prov(), doms); err != nil {
		return err
	}
	if len(reason) > 200 {
		reason = reason[:200]
	}
	_, err = s.db.Exec(ctx, `UPDATE accounts SET status='suspended', suspend_reason=?, updated_at=? WHERE id=?`, reason, time.Now().Unix(), a.ID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `UPDATE users SET suspended=1 WHERE id=?`, a.UserID)
	if err == nil {
		_, err = s.db.Exec(ctx, `DELETE FROM sessions WHERE user_id=?`, a.UserID)
	}
	if err == nil {
		s.syncFTPLogged(ctx) // suspended accounts lose FTP access
	}
	return err
}

func (s *Server) unsuspendAccount(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if err := s.doUnsuspend(r.Context(), a); err != nil {
		return err
	}
	s.audit(r, "account.unsuspend", a.SysUser, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) doUnsuspend(ctx context.Context, a *acct) error {
	if err := s.prov.Unsuspend(ctx, a.prov()); err != nil {
		return err
	}
	if err := s.rewriteVhosts(ctx, a); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `UPDATE accounts SET status='active', suspend_reason='', updated_at=? WHERE id=?`, time.Now().Unix(), a.ID); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `UPDATE users SET suspended=0 WHERE id=?`, a.UserID)
	if err == nil {
		s.syncFTPLogged(ctx)
	}
	return err
}

func (s *Server) terminateAccount(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if err := s.doTerminate(r.Context(), a); err != nil {
		return err
	}
	s.audit(r, "account.terminate", a.SysUser, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) doTerminate(ctx context.Context, a *acct) error {
	doms, err := s.accountDomains(ctx, a.ID)
	if err != nil {
		return err
	}
	// Drop customer databases/users first; they live outside the home dir.
	if err := s.dropAccountDatabases(ctx, a.ID); err != nil {
		return err
	}
	for _, d := range doms {
		_ = s.prov.DNS().DeleteZone(ctx, d)
	}
	if err := s.prov.DeleteAccount(ctx, a.prov(), doms); err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `DELETE FROM users WHERE id = ?`, a.UserID) // cascades
	if err == nil {
		s.syncFTPLogged(ctx)
		_, _ = s.prov.Runner().Run(ctx, "rm", "-rf", "--", provision.HtpasswdDir+"/"+a.SysUser)
	}
	return err
}
