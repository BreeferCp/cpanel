package api

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/auth"
	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/provision"
	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/store"
)

func (s *Server) wpToolkitRoutes(mux *http.ServeMux) {
	a := "/api/v1/accounts/{aid}"
	i := a + "/installations/{id}"
	s.route(mux, "POST "+a+"/wordpress/scan", rbac.HostingWrite, s.wpScan)
	s.route(mux, "GET "+i+"/wordpress/site", rbac.HostingRead, s.wpSite)
	s.route(mux, "PUT "+i+"/wordpress/settings", rbac.HostingWrite, s.wpSettings)
	s.route(mux, "POST "+i+"/wordpress/install-item", rbac.HostingWrite, s.wpInstallItem)
	s.route(mux, "GET "+i+"/wordpress/security", rbac.HostingRead, s.wpSecurity)
	s.route(mux, "POST "+i+"/wordpress/security/{check}", rbac.HostingWrite, s.wpFix)
	s.route(mux, "PUT "+i+"/autoupdate", rbac.HostingWrite, s.wpAutoUpdatePolicy)
	s.route(mux, "POST "+i+"/update-now", rbac.HostingWrite, s.wpUpdateNow)
	s.route(mux, "GET "+i+"/backups", rbac.HostingRead, s.listInstallBackups)
	s.route(mux, "POST "+i+"/backups", rbac.HostingWrite, s.createInstallBackup)
	s.route(mux, "POST "+i+"/backups/{bid}/restore", rbac.HostingWrite, s.restoreInstallBackup)
	s.route(mux, "DELETE "+i+"/backups/{bid}", rbac.HostingWrite, s.deleteInstallBackup)
	s.route(mux, "POST "+i+"/clone", rbac.HostingWrite, s.cloneInstallation)
	s.route(mux, "POST "+i+"/push", rbac.HostingWrite, s.pushStaging)
}

type wpPolicy struct {
	AutoCore    string `json:"auto_core"`
	AutoPlugins bool   `json:"auto_plugins"`
	AutoThemes  bool   `json:"auto_themes"`
	StagingOf   string `json:"staging_of"`
	LastAt      int64  `json:"last_update_at"`
	LastResult  string `json:"last_update_result"`
}

func (s *Server) wpPolicyOf(ctx context.Context, id string) wpPolicy {
	var p wpPolicy
	var pl, th int
	_ = s.db.QueryRow(ctx, `SELECT auto_core, auto_plugins, auto_themes, staging_of, last_update_at, last_update_result FROM installations WHERE id = ?`, id).
		Scan(&p.AutoCore, &pl, &th, &p.StagingOf, &p.LastAt, &p.LastResult)
	p.AutoPlugins, p.AutoThemes = pl == 1, th == 1
	return p
}

func (s *Server) wpSite(w http.ResponseWriter, r *http.Request) error {
	a, inst, err := s.wordpressOf(r)
	if err != nil {
		return err
	}
	site := s.prov.WPSiteInfo(r.Context(), a.prov(), inst.Path)
	writeJSON(w, 200, map[string]any{"site": site, "policy": s.wpPolicyOf(r.Context(), inst.ID)})
	return nil
}

func (s *Server) wpSettings(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name string `json:"name"`
		On   bool   `json:"on"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, inst, err := s.wordpressOf(r)
	if err != nil {
		return err
	}
	if err := s.prov.WPSetting(r.Context(), a.prov(), inst.Path, req.Name, req.On); err != nil {
		return wrapWP(err)
	}
	s.audit(r, "wordpress.setting."+req.Name, inst.URL, true, fmt.Sprint(req.On))
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) wpInstallItem(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Kind     string `json:"kind"`
		Slug     string `json:"slug"`
		Activate bool   `json:"activate"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, inst, err := s.wordpressOf(r)
	if err != nil {
		return err
	}
	if err := s.prov.WPInstallItem(r.Context(), a.prov(), inst.Path, req.Kind, strings.TrimSpace(req.Slug), req.Activate); err != nil {
		return wrapWP(err)
	}
	s.audit(r, "wordpress.install."+req.Kind, inst.URL, true, req.Slug)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) wpSecurity(w http.ResponseWriter, r *http.Request) error {
	a, inst, err := s.wordpressOf(r)
	if err != nil {
		return err
	}
	writeJSON(w, 200, s.prov.WPSecurity(r.Context(), a.prov(), inst.Path))
	return nil
}

func (s *Server) wpFix(w http.ResponseWriter, r *http.Request) error {
	a, inst, err := s.wordpressOf(r)
	if err != nil {
		return err
	}
	check := r.PathValue("check")
	if err := s.prov.WPFix(r.Context(), a.prov(), inst.Path, check); err != nil {
		return wrapWP(err)
	}
	s.audit(r, "wordpress.harden."+check, inst.URL, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// wpScan finds WordPress sites the panel does not know yet (installed by
// hand or migrated in) and adds them to the toolkit.
func (s *Server) wpScan(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	dirs, err := s.prov.WPDetect(r.Context(), a.prov())
	if err != nil {
		return err
	}
	doms, err := s.domainsOf(r.Context(), a.ID)
	if err != nil {
		return err
	}
	added := []string{}
	for _, dir := range dirs {
		var n int
		_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM installations WHERE account_id = ? AND path = ?`, a.ID, dir).Scan(&n)
		if n > 0 {
			continue
		}
		// Attach it to the domain whose document root contains it.
		var best *domain
		for i := range doms {
			if dir == doms[i].DocRoot || strings.HasPrefix(dir, doms[i].DocRoot+"/") {
				if best == nil || len(doms[i].DocRoot) > len(best.DocRoot) {
					best = &doms[i]
				}
			}
		}
		if best == nil {
			continue
		}
		sub := strings.TrimPrefix(strings.TrimPrefix(dir, best.DocRoot), "/")
		url := strings.TrimSuffix(siteScheme(best.HasSSL)+best.Name+"/"+sub, "/")
		site := s.prov.WPSiteInfo(r.Context(), a.prov(), dir)
		if site.SiteURL != "" && provision.SiteURL(site.SiteURL) == nil {
			url = site.SiteURL
		}
		version := ""
		if info, err := s.prov.WPStatus(r.Context(), a.prov(), dir); err == nil {
			version = info.Version
		}
		if _, err := s.db.Exec(r.Context(), `INSERT INTO installations (id, account_id, domain_id, app, path, url, version, db_name, admin_user, status, created_at)
			VALUES (?,?,?,'wordpress',?,?,?,?,?,'installed',?)`, ids.New(), a.ID, best.ID, dir, url, version, site.DBName,
			strings.Join(site.Admins, ","), time.Now().Unix()); err != nil {
			continue
		}
		added = append(added, url)
	}
	s.audit(r, "wordpress.scan", a.SysUser, true, fmt.Sprintf("%d added", len(added)))
	writeJSON(w, 200, map[string]any{"found": len(dirs), "added": added})
	return nil
}

func siteScheme(ssl bool) string {
	if ssl {
		return "https://"
	}
	return "http://"
}

// ---- auto-update policy and smart updates ----------------------------------------------

func (s *Server) wpAutoUpdatePolicy(w http.ResponseWriter, r *http.Request) error {
	var req wpPolicy
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.AutoCore != "off" && req.AutoCore != "minor" && req.AutoCore != "major" {
		return errBadRequest("core updates must be off, minor or major")
	}
	_, inst, err := s.wordpressOf(r)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE installations SET auto_core = ?, auto_plugins = ?, auto_themes = ? WHERE id = ?`,
		req.AutoCore, b2i(req.AutoPlugins), b2i(req.AutoThemes), inst.ID); err != nil {
		return err
	}
	s.audit(r, "wordpress.autoupdate", inst.URL, true, req.AutoCore)
	writeJSON(w, 200, s.wpPolicyOf(r.Context(), inst.ID))
	return nil
}

func (s *Server) wpUpdateNow(w http.ResponseWriter, r *http.Request) error {
	a, inst, err := s.wordpressOf(r)
	if err != nil {
		return err
	}
	p := s.wpPolicyOf(r.Context(), inst.ID)
	core := p.AutoCore
	if core == "off" {
		core = "major"
	}
	go s.smartUpdate(*a, inst, core, true, true)
	s.audit(r, "wordpress.smartupdate", inst.URL, true, "")
	w.WriteHeader(http.StatusAccepted)
	return nil
}

// siteHealthy fetches the site's home page and reports whether it answers
// below 500. Certificates are not checked: this probes the local server.
func siteHealthy(ctx context.Context, url string) (int, bool) {
	c := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/", nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("User-Agent", "BreeferCP-SmartUpdate")
	resp, err := c.Do(req)
	if err != nil {
		return 0, false
	}
	resp.Body.Close()
	return resp.StatusCode, resp.StatusCode < 500
}

// smartUpdate backs the site up, applies updates, and restores the backup
// automatically if the home page stops working (WP Toolkit "Smart Updates").
func (s *Server) smartUpdate(a acct, inst installation, core string, plugins, themes bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	record := func(result string) {
		_, _ = s.db.Exec(ctx, `UPDATE installations SET last_update_at = ?, last_update_result = ? WHERE id = ?`, time.Now().Unix(), result, inst.ID)
	}
	if s.cfg.DryRun {
		record("Updated core (minor), plugins. The site answered normally afterwards.")
		return
	}
	_, before := siteHealthy(ctx, inst.URL)
	bid, dir, err := s.backupInstallation(ctx, &a, inst, "pre-update")
	if err != nil {
		record("Skipped: the pre-update backup failed.")
		return
	}
	done, uerr := s.prov.WPAutoUpdate(ctx, a.prov(), inst.Path, core, plugins, themes)
	code, after := siteHealthy(ctx, inst.URL)
	if uerr != nil || (before && !after) {
		if rerr := s.prov.WPRestore(ctx, a.prov(), inst.Path, inst.DBName, dir, false); rerr != nil {
			record("Update broke the site and the automatic restore failed; restore backup " + bid + " manually.")
			s.log.Error("smart update rollback failed", "site", inst.URL, "err", rerr)
			return
		}
		record(fmt.Sprintf("Rolled back: after updating the site answered %d. Your site was restored from the pre-update backup.", code))
		return
	}
	if done == "" {
		done = "nothing"
	}
	if v, err := s.prov.WPStatus(ctx, a.prov(), inst.Path); err == nil {
		_, _ = s.db.Exec(ctx, `UPDATE installations SET version = ? WHERE id = ?`, v.Version, inst.ID)
	}
	record("Updated " + done + ". The site answered normally afterwards.")
	s.pruneInstallBackups(ctx, inst.ID, "pre-update", 3)
}

// wpAutoUpdateTick runs daily smart updates between 01:00 and 02:00.
func (s *Server) wpAutoUpdateTick(ctx context.Context) {
	if time.Now().Hour() != 1 {
		return
	}
	cutoff := time.Now().Add(-20 * time.Hour).Unix()
	rows, err := s.db.Query(ctx, installSelect+` WHERE i.app = 'wordpress' AND i.status = 'installed'
		AND (i.auto_core <> 'off' OR i.auto_plugins = 1 OR i.auto_themes = 1) AND i.last_update_at < ? AND i.staging_of = ''`, cutoff)
	if err != nil {
		return
	}
	var list []installation
	for rows.Next() {
		if v, err := scanInstallation(rows); err == nil {
			list = append(list, v)
		}
	}
	rows.Close()
	go func() {
		for _, inst := range list {
			var accountID string
			if s.db.QueryRow(ctx, `SELECT account_id FROM installations WHERE id = ?`, inst.ID).Scan(&accountID) != nil {
				continue
			}
			a, err := scanAcct(s.db.QueryRow(ctx, acctSelect+` WHERE a.id = ? AND a.status = 'active'`, accountID))
			if err != nil {
				continue
			}
			p := s.wpPolicyOf(ctx, inst.ID)
			s.smartUpdate(*a, inst, p.AutoCore, p.AutoPlugins, p.AutoThemes)
		}
	}()
}

// ---- backups -------------------------------------------------------------------------------

type installBackup struct {
	ID        string `json:"id"`
	Reason    string `json:"reason"`
	SizeBytes int64  `json:"size_bytes"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

func (s *Server) installBackupDir(a *acct, instID, bid string) string {
	return filepath.Join(s.cfg.DataDir, "backups", a.SysUser, "apps", instID, bid)
}

// backupInstallation synchronously backs up one installation.
func (s *Server) backupInstallation(ctx context.Context, a *acct, inst installation, reason string) (string, string, error) {
	bid := ids.New()
	dir := s.installBackupDir(a, inst.ID, bid)
	if _, err := s.db.Exec(ctx, `INSERT INTO install_backups (id, installation_id, path, reason, status, created_at) VALUES (?,?,?,?,'running',?)`,
		bid, inst.ID, dir, reason, time.Now().Unix()); err != nil {
		return "", "", err
	}
	err := s.prov.WPBackup(ctx, a.prov(), inst.Path, inst.DBName, dir)
	status, size := "complete", dirSize(dir)
	if err != nil {
		status = "failed"
		s.log.Error("installation backup", "site", inst.URL, "err", err)
	}
	_, _ = s.db.Exec(ctx, `UPDATE install_backups SET status = ?, size_bytes = ? WHERE id = ?`, status, size, bid)
	return bid, dir, err
}

func dirSize(dir string) int64 {
	var n int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n += info.Size()
		}
		return nil
	})
	return n
}

func (s *Server) pruneInstallBackups(ctx context.Context, instID, reason string, keep int) {
	rows, err := s.db.Query(ctx, `SELECT id, path FROM install_backups WHERE installation_id = ? AND reason = ? ORDER BY created_at DESC`, instID, reason)
	if err != nil {
		return
	}
	type b struct{ id, p string }
	var old []b
	n := 0
	for rows.Next() {
		var x b
		if rows.Scan(&x.id, &x.p) == nil {
			if n++; n > keep {
				old = append(old, x)
			}
		}
	}
	rows.Close()
	for _, x := range old {
		_, _ = s.prov.Runner().Run(ctx, "rm", "-rf", "--", x.p)
		_, _ = s.db.Exec(ctx, `DELETE FROM install_backups WHERE id = ?`, x.id)
	}
}

// installationAny loads an installation of any app (backups work for all).
func (s *Server) installationAny(r *http.Request) (*acct, installation, error) {
	a, err := s.loadAccount(r)
	if err != nil {
		return nil, installation{}, err
	}
	inst, err := s.installationOf(r, a)
	if err != nil {
		return nil, inst, err
	}
	if inst.Status != "installed" {
		return nil, inst, errConflict("installation is not ready")
	}
	return a, inst, nil
}

func (s *Server) listInstallBackups(w http.ResponseWriter, r *http.Request) error {
	_, inst, err := s.installationAny(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, reason, size_bytes, status, created_at FROM install_backups WHERE installation_id = ? ORDER BY created_at DESC`, inst.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []installBackup{}
	for rows.Next() {
		var b installBackup
		if err := rows.Scan(&b.ID, &b.Reason, &b.SizeBytes, &b.Status, &b.CreatedAt); err != nil {
			return err
		}
		out = append(out, b)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) createInstallBackup(w http.ResponseWriter, r *http.Request) error {
	a, inst, err := s.installationAny(r)
	if err != nil {
		return err
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()
		_, _, _ = s.backupInstallation(ctx, a, inst, "manual")
	}()
	s.audit(r, "installer.backup", inst.URL, true, "")
	w.WriteHeader(http.StatusAccepted)
	return nil
}

func (s *Server) installBackupOf(ctx context.Context, instID, bid string) (string, error) {
	var p, status string
	if err := s.db.QueryRow(ctx, `SELECT path, status FROM install_backups WHERE id = ? AND installation_id = ?`, bid, instID).Scan(&p, &status); err != nil {
		return "", store.ErrNotFound
	}
	if status != "complete" {
		return "", errConflict("this backup is not complete")
	}
	return p, nil
}

func (s *Server) restoreInstallBackup(w http.ResponseWriter, r *http.Request) error {
	a, inst, err := s.installationAny(r)
	if err != nil {
		return err
	}
	dir, err := s.installBackupOf(r.Context(), inst.ID, r.PathValue("bid"))
	if err != nil {
		return err
	}
	ctx := context.WithoutCancel(r.Context())
	if _, _, err := s.backupInstallation(ctx, a, inst, "pre-restore"); err != nil {
		return errConflict("could not take a safety backup before restoring; nothing was changed")
	}
	s.pruneInstallBackups(ctx, inst.ID, "pre-restore", 2)
	if err := s.prov.WPRestore(ctx, a.prov(), inst.Path, inst.DBName, dir, false); err != nil {
		return wrapWP(err)
	}
	s.audit(r, "installer.restore", inst.URL, true, r.PathValue("bid"))
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) deleteInstallBackup(w http.ResponseWriter, r *http.Request) error {
	_, inst, err := s.installationAny(r)
	if err != nil {
		return err
	}
	var p string
	if err := s.db.QueryRow(r.Context(), `SELECT path FROM install_backups WHERE id = ? AND installation_id = ?`, r.PathValue("bid"), inst.ID).Scan(&p); err != nil {
		return store.ErrNotFound
	}
	_, _ = s.prov.Runner().Run(r.Context(), "rm", "-rf", "--", p)
	if _, err := s.db.Exec(r.Context(), `DELETE FROM install_backups WHERE id = ?`, r.PathValue("bid")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- clone, staging and push ---------------------------------------------------------------

func (s *Server) cloneInstallation(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		DomainID  string `json:"domain_id"`
		Directory string `json:"directory"`
		Staging   bool   `json:"staging"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, src, err := s.wordpressOf(r)
	if err != nil {
		return err
	}
	if src.DBName == "" {
		return errConflict("the panel does not know this site's database; run a scan first")
	}
	d, err := s.domainOf(r.Context(), a.ID, req.DomainID)
	if err != nil {
		return errBadRequest("choose one of your domains")
	}
	sub, err := provision.CleanRelDir(req.Directory)
	if err != nil {
		return err
	}
	dir := d.DocRoot
	if sub != "" {
		dir = path.Join(d.DocRoot, sub)
	}
	if dir == src.Path {
		return errBadRequest("choose a different location for the copy")
	}
	if err := s.within(r.Context(), "db_instances", a.ID, a.pkg.MaxDatabases, "databases"); err != nil {
		return err
	}
	var hasSSL int
	_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM ssl_certs WHERE domain_id = ?`, d.ID).Scan(&hasSSL)
	url := strings.TrimSuffix(siteScheme(hasSSL > 0)+d.Name+"/"+sub, "/")
	dbName, err := prefixed(a.SysUser, "wp"+ids.New()[:8])
	if err != nil {
		return err
	}
	dbPass, err := auth.RandomPassword(24)
	if err != nil {
		return err
	}
	staging := ""
	if req.Staging {
		staging = src.ID
	}
	inst := installation{ID: ids.New(), App: "wordpress", AppName: "WordPress", DomainID: d.ID, Domain: d.Name, Path: dir, URL: url,
		DBName: dbName, AdminUser: src.AdminUser, Status: "installing", Version: src.Version, CreatedAt: time.Now().Unix()}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO installations (id, account_id, domain_id, app, path, url, version, db_name, admin_user, status, staging_of, auto_core, created_at)
		VALUES (?,?,?,'wordpress',?,?,?,?,?,'installing',?,'off',?)`, inst.ID, a.ID, d.ID, dir, url, src.Version, dbName, src.AdminUser, staging, inst.CreatedAt); err != nil {
		if store.IsUnique(err) {
			return errConflict("an application is already installed in that directory")
		}
		return err
	}
	go func(a acct) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		fail := func(err error) {
			s.log.Error("clone failed", "src", src.URL, "dst", url, "err", err)
			_, _ = s.db.Exec(ctx, `UPDATE installations SET status = 'failed' WHERE id = ?`, inst.ID)
		}
		if err := s.createAppDatabase(ctx, &a, dbName, dbPass); err != nil {
			fail(err)
			return
		}
		spec := provision.WPCloneSpec{SrcDir: src.Path, SrcDB: src.DBName, SrcURL: src.URL, DstDir: dir, DstURL: url,
			DstDB: dbName, DstUser: dbName, DstPass: dbPass, NoIndex: req.Staging,
			Stage: filepath.Join(s.cfg.DataDir, "staging", "clone-"+inst.ID)}
		if !s.cfg.DryRun {
			if err := s.prov.WPClone(ctx, a.prov(), spec); err != nil {
				fail(err)
				return
			}
		}
		_, _ = s.db.Exec(ctx, `UPDATE installations SET status = 'installed' WHERE id = ?`, inst.ID)
	}(*a)
	s.audit(r, map[bool]string{true: "wordpress.staging", false: "wordpress.clone"}[req.Staging], src.URL, true, url)
	writeJSON(w, 202, inst)
	return nil
}

// createAppDatabase creates a database and same-named user with ALL on it
// and records both in the panel.
func (s *Server) createAppDatabase(ctx context.Context, a *acct, name, pass string) error {
	if err := s.mariadb.CreateDatabase(ctx, name); err != nil {
		return err
	}
	if err := s.mariadb.CreateUser(ctx, name, pass); err != nil {
		return err
	}
	if err := s.mariadb.Grant(ctx, name, name, nil); err != nil {
		return err
	}
	now := time.Now().Unix()
	dbID, userID := ids.New(), ids.New()
	_, _ = s.db.Exec(ctx, `INSERT INTO db_instances (id, account_id, name, created_at) VALUES (?,?,?,?)`, dbID, a.ID, name, now)
	_, _ = s.db.Exec(ctx, `INSERT INTO db_users (id, account_id, name, created_at) VALUES (?,?,?,?)`, userID, a.ID, name, now)
	_, _ = s.db.Exec(ctx, `INSERT INTO db_grants (db_id, db_user_id, privileges) VALUES (?,?,'ALL')`, dbID, userID)
	return nil
}

// pushStaging copies a staging site over the live site it was made from,
// after a safety backup of the live site.
func (s *Server) pushStaging(w http.ResponseWriter, r *http.Request) error {
	a, stg, err := s.wordpressOf(r)
	if err != nil {
		return err
	}
	pol := s.wpPolicyOf(r.Context(), stg.ID)
	if pol.StagingOf == "" {
		return errBadRequest("this site is not a staging copy")
	}
	live, err := scanInstallation(s.db.QueryRow(r.Context(), installSelect+` WHERE i.id = ? AND i.account_id = ?`, pol.StagingOf, a.ID))
	if err != nil {
		return errConflict("the live site of this staging copy no longer exists")
	}
	go func(a acct) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		if s.cfg.DryRun {
			return
		}
		if _, _, err := s.backupInstallation(ctx, &a, live, "pre-push"); err != nil {
			s.log.Error("push: safety backup failed", "site", live.URL, "err", err)
			return
		}
		s.pruneInstallBackups(ctx, live.ID, "pre-push", 2)
		spec := provision.WPCloneSpec{SrcDir: stg.Path, SrcDB: stg.DBName, SrcURL: stg.URL, DstDir: live.Path, DstDB: live.DBName,
			DstURL: live.URL, Stage: filepath.Join(s.cfg.DataDir, "staging", "push-"+stg.ID)}
		if err := s.prov.WPPush(ctx, a.prov(), spec); err != nil {
			s.log.Error("push to live failed", "site", live.URL, "err", err)
			_, _ = s.db.Exec(ctx, `UPDATE installations SET last_update_result = ? WHERE id = ?`,
				"Push from staging failed; restore the pre-push backup if the site is broken.", live.ID)
		}
	}(*a)
	s.audit(r, "wordpress.push", stg.URL, true, live.URL)
	w.WriteHeader(http.StatusAccepted)
	return nil
}
