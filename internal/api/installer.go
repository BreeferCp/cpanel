package api

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/auth"
	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/provision"
	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/store"
)

func (s *Server) installerRoutes(mux *http.ServeMux) {
	const a = "/api/v1/accounts/{aid}"
	mux.HandleFunc("GET /api/v1/installer/catalog", s.require(rbac.HostingRead, func(w http.ResponseWriter, _ *http.Request) error {
		writeJSON(w, 200, provision.Catalog)
		return nil
	}))
	s.route(mux, "GET "+a+"/installations", rbac.HostingRead, s.listInstallations)
	s.route(mux, "POST "+a+"/installations", rbac.HostingWrite, s.createInstallation)
	s.route(mux, "DELETE "+a+"/installations/{id}", rbac.HostingWrite, s.deleteInstallation)
	s.route(mux, "GET "+a+"/installations/{id}/wordpress", rbac.HostingRead, s.wpStatus)
	s.route(mux, "POST "+a+"/installations/{id}/wordpress", rbac.HostingWrite, s.wpAction)
}

type installation struct {
	ID        string `json:"id"`
	App       string `json:"app"`
	AppName   string `json:"app_name"`
	DomainID  string `json:"domain_id"`
	Domain    string `json:"domain"`
	Path      string `json:"path"`
	URL       string `json:"url"`
	Version   string `json:"version"`
	DBName    string `json:"db_name"`
	AdminUser string `json:"admin_user"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

const installSelect = `SELECT i.id, i.app, i.domain_id, d.name, i.path, i.url, i.version, i.db_name, i.admin_user, i.status, i.created_at
	FROM installations i JOIN domains d ON d.id = i.domain_id`

func scanInstallation(row rowScanner) (installation, error) {
	var v installation
	err := row.Scan(&v.ID, &v.App, &v.DomainID, &v.Domain, &v.Path, &v.URL, &v.Version, &v.DBName, &v.AdminUser, &v.Status, &v.CreatedAt)
	if app, ok := provision.CatalogByID(v.App); ok {
		v.AppName = app.Name
	}
	return v, store.IsNoRows(err)
}

func (s *Server) listInstallations(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), installSelect+` WHERE i.account_id = ? ORDER BY i.created_at DESC`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []installation{}
	for rows.Next() {
		v, err := scanInstallation(rows)
		if err != nil {
			return err
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) installationOf(r *http.Request, a *acct) (installation, error) {
	return scanInstallation(s.db.QueryRow(r.Context(), installSelect+` WHERE i.id = ? AND i.account_id = ?`, r.PathValue("id"), a.ID))
}

func (s *Server) createInstallation(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		App           string `json:"app"`
		DomainID      string `json:"domain_id"`
		Directory     string `json:"directory"`
		Title         string `json:"title"`
		AdminUser     string `json:"admin_user"`
		AdminPassword string `json:"admin_password"`
		AdminEmail    string `json:"admin_email"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if a.Status != "active" {
		return errConflict("account is suspended")
	}
	app, ok := provision.CatalogByID(req.App)
	if !ok {
		return errBadRequest("unknown application")
	}
	d, err := s.domainOf(r.Context(), a.ID, req.DomainID)
	if err != nil {
		return errBadRequest("choose one of your domains")
	}
	sub := strings.Trim(path.Clean("/"+req.Directory), "/")
	if strings.Contains(sub, "..") || strings.ContainsAny(sub, "\x00\r\n") {
		return errBadRequest("invalid directory")
	}
	dir := d.DocRoot
	if sub != "" {
		dir = path.Join(d.DocRoot, sub)
	}
	url := "https://" + d.Name + "/" + sub
	url = strings.TrimSuffix(url, "/")
	if app.Method == "wpcli" {
		if req.Title == "" {
			req.Title = "My WordPress Site"
		}
		if req.AdminUser == "" || req.AdminEmail == "" {
			return errBadRequest("admin username and email are required")
		}
		if err := auth.CheckPasswordPolicy(req.AdminPassword); err != nil {
			return errBadRequest("admin password: " + err.Error())
		}
	}
	var dbName, dbUser, dbPass string
	if app.NeedsDB {
		if err := s.within(r.Context(), "db_instances", a.ID, a.pkg.MaxDatabases, "databases"); err != nil {
			return err
		}
		suffix := strings.ReplaceAll(ids.New()[:6], "-", "")
		dbName, err = prefixed(a.SysUser, app.ID[:min(len(app.ID), 8)]+suffix)
		if err != nil {
			return err
		}
		dbUser = dbName
		if dbPass, err = auth.RandomPassword(24); err != nil {
			return err
		}
	}
	inst := installation{ID: ids.New(), App: app.ID, AppName: app.Name, DomainID: d.ID, Domain: d.Name, Path: dir, URL: url,
		DBName: dbName, AdminUser: req.AdminUser, Status: "installing", CreatedAt: time.Now().Unix()}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO installations (id, account_id, domain_id, app, path, url, db_name, admin_user, status, created_at)
		VALUES (?,?,?,?,?,?,?,?,'installing',?)`, inst.ID, a.ID, d.ID, app.ID, dir, url, dbName, req.AdminUser, inst.CreatedAt); err != nil {
		if store.IsUnique(err) {
			return errConflict("an application is already installed in that directory")
		}
		return err
	}
	spec := provision.InstallSpec{App: app, Dir: dir, URL: url, Title: req.Title, AdminUser: req.AdminUser,
		AdminPass: req.AdminPassword, AdminEmail: req.AdminEmail, DBName: dbName, DBUser: dbUser, DBPass: dbPass}
	go s.runInstall(*a, inst.ID, spec)
	s.audit(r, "installer.install", app.ID, true, url)
	writeJSON(w, 202, inst)
	return nil
}

func (s *Server) runInstall(a acct, id string, spec provision.InstallSpec) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	fail := func(err error) {
		s.log.Error("install failed", "account", a.SysUser, "app", spec.App.ID, "err", err)
		_, _ = s.db.Exec(ctx, `UPDATE installations SET status = 'failed' WHERE id = ?`, id)
	}
	if spec.DBName != "" {
		now := time.Now().Unix()
		dbID, userID := ids.New(), ids.New()
		if err := s.mariadb.CreateDatabase(ctx, spec.DBName); err != nil {
			fail(err)
			return
		}
		if err := s.mariadb.CreateUser(ctx, spec.DBUser, spec.DBPass); err != nil {
			fail(err)
			return
		}
		if err := s.mariadb.Grant(ctx, spec.DBName, spec.DBUser, nil); err != nil {
			fail(err)
			return
		}
		_, _ = s.db.Exec(ctx, `INSERT INTO db_instances (id, account_id, name, created_at) VALUES (?,?,?,?)`, dbID, a.ID, spec.DBName, now)
		_, _ = s.db.Exec(ctx, `INSERT INTO db_users (id, account_id, name, created_at) VALUES (?,?,?,?)`, userID, a.ID, spec.DBUser, now)
		_, _ = s.db.Exec(ctx, `INSERT INTO db_grants (db_id, db_user_id, privileges) VALUES (?,?,'ALL')`, dbID, userID)
	}
	if err := s.prov.Install(ctx, a.prov(), spec); err != nil {
		fail(err)
		return
	}
	version := ""
	if spec.App.ID == "wordpress" {
		if info, err := s.prov.WPStatus(ctx, a.prov(), spec.Dir); err == nil {
			version = info.Version
		}
	}
	_, _ = s.db.Exec(ctx, `UPDATE installations SET status = 'installed', version = ? WHERE id = ?`, version, id)
}

func (s *Server) deleteInstallation(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		DeleteFiles    bool `json:"delete_files"`
		DeleteDatabase bool `json:"delete_database"`
	}
	_ = decode(r, &req)
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	inst, err := s.installationOf(r, a)
	if err != nil {
		return err
	}
	if inst.Status == "installing" {
		return errConflict("wait for the installation to finish")
	}
	if req.DeleteFiles {
		if err := s.prov.Uninstall(r.Context(), a.prov(), inst.Path); err != nil {
			return err
		}
	}
	if req.DeleteDatabase && inst.DBName != "" {
		if err := s.mariadb.DropDatabase(r.Context(), inst.DBName); err != nil {
			return err
		}
		_ = s.mariadb.DropUser(r.Context(), inst.DBName)
		_, _ = s.db.Exec(r.Context(), `DELETE FROM db_instances WHERE account_id = ? AND name = ?`, a.ID, inst.DBName)
		_, _ = s.db.Exec(r.Context(), `DELETE FROM db_users WHERE account_id = ? AND name = ?`, a.ID, inst.DBName)
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM installations WHERE id = ?`, inst.ID); err != nil {
		return err
	}
	s.audit(r, "installer.remove", inst.App, true, inst.URL)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) wordpressOf(r *http.Request) (*acct, installation, error) {
	a, err := s.loadAccount(r)
	if err != nil {
		return nil, installation{}, err
	}
	inst, err := s.installationOf(r, a)
	if err != nil {
		return nil, inst, err
	}
	if inst.App != "wordpress" {
		return nil, inst, errBadRequest("this installation is not WordPress")
	}
	if inst.Status != "installed" {
		return nil, inst, errConflict("installation is not ready")
	}
	return a, inst, nil
}

func (s *Server) wpStatus(w http.ResponseWriter, r *http.Request) error {
	a, inst, err := s.wordpressOf(r)
	if err != nil {
		return err
	}
	info, err := s.prov.WPStatus(r.Context(), a.prov(), inst.Path)
	if err != nil {
		return &apiError{502, "wpcli", "WP-CLI could not read this site: " + firstLine(err.Error())}
	}
	if info.Version != "" && info.Version != inst.Version {
		_, _ = s.db.Exec(r.Context(), `UPDATE installations SET version = ? WHERE id = ?`, info.Version, inst.ID)
	}
	writeJSON(w, 200, info)
	return nil
}

func (s *Server) wpAction(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Action   string `json:"action"`
		Target   string `json:"target"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, inst, err := s.wordpressOf(r)
	if err != nil {
		return err
	}
	if err := s.prov.WPAction(r.Context(), a.prov(), inst.Path, req.Action, req.Target, req.Password); err != nil {
		return wrapWP(err)
	}
	s.audit(r, "wordpress."+req.Action, inst.URL, true, req.Target)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func wrapWP(err error) error {
	if strings.Contains(err.Error(), "invalid value") {
		return err
	}
	return &apiError{502, "wpcli", fmt.Sprintf("WP-CLI failed: %s", firstLine(err.Error()))}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}
