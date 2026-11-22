package api

import (
	"context"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/auth"
	"github.com/breefercp/breefercp/internal/files"
	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/provision"
	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/system"
)

func (s *Server) moreHostingRoutes(mux *http.ServeMux) {
	a := "/api/v1/accounts/{aid}"
	r, w := rbac.HostingRead, rbac.HostingWrite

	s.route(mux, "GET "+a+"/ftp", r, s.listFTP)
	s.route(mux, "POST "+a+"/ftp", w, s.createFTP)
	s.route(mux, "PATCH "+a+"/ftp/{id}", w, s.updateFTP)
	s.route(mux, "DELETE "+a+"/ftp/{id}", w, s.deleteFTP)

	s.route(mux, "GET "+a+"/protected-dirs", r, s.listProtected)
	s.route(mux, "POST "+a+"/protected-dirs", w, s.createProtected)
	s.route(mux, "DELETE "+a+"/protected-dirs/{id}", w, s.deleteProtected)
	s.route(mux, "PUT "+a+"/protected-dirs/{id}/users/{user}", w, s.putProtectedUser)
	s.route(mux, "DELETE "+a+"/protected-dirs/{id}/users/{user}", w, s.deleteProtectedUser)

	s.route(mux, "GET "+a+"/ip-blocks", r, s.listIPBlocks)
	s.route(mux, "POST "+a+"/ip-blocks", w, s.addIPBlock)
	s.route(mux, "DELETE "+a+"/ip-blocks/{id}", w, s.deleteIPBlock)

	s.route(mux, "GET "+a+"/hotlink", r, s.getHotlink)
	s.route(mux, "PUT "+a+"/hotlink", w, s.putHotlink)

	s.route(mux, "GET "+a+"/remote-mysql", r, s.listRemoteMySQL)
	s.route(mux, "POST "+a+"/remote-mysql", w, s.addRemoteMySQL)
	s.route(mux, "DELETE "+a+"/remote-mysql/{id}", w, s.deleteRemoteMySQL)

	s.route(mux, "GET "+a+"/git", r, s.listGit)
	s.route(mux, "POST "+a+"/git", w, s.createGit)
	s.route(mux, "POST "+a+"/git/{id}/pull", w, s.pullGit)
	s.route(mux, "DELETE "+a+"/git/{id}", w, s.deleteGit)

	s.route(mux, "GET "+a+"/dnssec/{zone}", r, s.getDNSSEC)
	s.route(mux, "PUT "+a+"/dnssec/{zone}", w, s.putDNSSEC)
}

// ---- FTP --------------------------------------------------------------------

type ftpView struct {
	ID        string `json:"id"`
	Login     string `json:"login"`
	Directory string `json:"directory"`
	QuotaMB   int    `json:"quota_mb"`
	CreatedAt int64  `json:"created_at"`
}

func (s *Server) listFTP(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, login, directory, quota_mb, created_at FROM ftp_accounts WHERE account_id = ? ORDER BY login`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []ftpView{}
	for rows.Next() {
		var v ftpView
		if err := rows.Scan(&v.ID, &v.Login, &v.Directory, &v.QuotaMB, &v.CreatedAt); err != nil {
			return err
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

// syncFTP regenerates the server-wide Pure-FTPd database. Suspended
// accounts are left out, which disables their FTP logins.
func (s *Server) syncFTP(ctx context.Context) error {
	if s.cfg.DryRun && runtime.GOOS != "linux" {
		// Development homes on Windows are "C:/..." paths, which the
		// colon-separated Pure-FTPd format cannot hold; nothing is written in
		// dry-run anyway.
		return nil
	}
	rows, err := s.db.Query(ctx, `SELECT f.login, f.password_hash, a.sys_uid, a.home_dir, f.directory, f.quota_mb
		FROM ftp_accounts f JOIN accounts a ON a.id = f.account_id WHERE a.status = 'active'`)
	if err != nil {
		return err
	}
	var users []provision.FTPUser
	for rows.Next() {
		var u provision.FTPUser
		if err := rows.Scan(&u.Login, &u.Hash, &u.UID, &u.Home, &u.Dir, &u.QuotaMB); err != nil {
			rows.Close()
			return err
		}
		users = append(users, u)
	}
	rows.Close()
	return s.prov.SyncFTP(ctx, users)
}

func (s *Server) createFTP(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name      string `json:"name"`
		Domain    string `json:"domain"`
		Password  string `json:"password"`
		Directory string `json:"directory"`
		QuotaMB   int    `json:"quota_mb"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if err := s.within(r.Context(), "ftp_accounts", a.ID, a.pkg.MaxFTP, "FTP accounts"); err != nil {
		return err
	}
	req.Domain = strings.ToLower(strings.TrimSpace(req.Domain))
	var n int
	if err := s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM domains WHERE account_id = ? AND name = ?`, a.ID, req.Domain).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return errBadRequest("choose one of your domains")
	}
	login := strings.ToLower(strings.TrimSpace(req.Name)) + "@" + req.Domain
	if err := provision.FTPLogin(login); err != nil {
		return err
	}
	dir, err := provision.CleanRelDir(req.Directory)
	if err != nil {
		return err
	}
	if req.QuotaMB < 0 || req.QuotaMB > 1<<20 {
		return errBadRequest("invalid quota")
	}
	if err := auth.CheckPasswordPolicy(req.Password); err != nil {
		return errBadRequest(err.Error())
	}
	hash, err := auth.SHA512Crypt(req.Password)
	if err != nil {
		return err
	}
	if dir != "" && !s.cfg.DryRun { // dry-run creates no home directories
		jail, err := files.Open(a.HomeDir, a.SysUID, a.SysUID)
		if err != nil {
			return fsErr(err)
		}
		err = jail.Mkdir(dir)
		jail.Close()
		if err != nil {
			return fsErr(err)
		}
	}
	v := ftpView{ID: ids.New(), Login: login, Directory: dir, QuotaMB: req.QuotaMB, CreatedAt: time.Now().Unix()}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO ftp_accounts (id, account_id, login, password_hash, directory, quota_mb, created_at)
		VALUES (?,?,?,?,?,?,?)`, v.ID, a.ID, login, hash, dir, req.QuotaMB, v.CreatedAt); err != nil {
		return err
	}
	if err := s.syncFTP(r.Context()); err != nil {
		_, _ = s.db.Exec(context.WithoutCancel(r.Context()), `DELETE FROM ftp_accounts WHERE id = ?`, v.ID)
		return err
	}
	s.audit(r, "ftp.create", login, true, "")
	writeJSON(w, 201, v)
	return nil
}

func (s *Server) updateFTP(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Password *string `json:"password"`
		QuotaMB  *int    `json:"quota_mb"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	var login string
	if err := s.db.QueryRow(r.Context(), `SELECT login FROM ftp_accounts WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID).Scan(&login); err != nil {
		return errNotFound()
	}
	if req.Password != nil {
		if err := auth.CheckPasswordPolicy(*req.Password); err != nil {
			return errBadRequest(err.Error())
		}
		hash, err := auth.SHA512Crypt(*req.Password)
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(r.Context(), `UPDATE ftp_accounts SET password_hash = ? WHERE id = ?`, hash, r.PathValue("id")); err != nil {
			return err
		}
	}
	if req.QuotaMB != nil {
		if *req.QuotaMB < 0 || *req.QuotaMB > 1<<20 {
			return errBadRequest("invalid quota")
		}
		if _, err := s.db.Exec(r.Context(), `UPDATE ftp_accounts SET quota_mb = ? WHERE id = ?`, *req.QuotaMB, r.PathValue("id")); err != nil {
			return err
		}
	}
	if err := s.syncFTP(r.Context()); err != nil {
		return err
	}
	s.audit(r, "ftp.update", login, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) deleteFTP(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(r.Context(), `DELETE FROM ftp_accounts WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound()
	}
	if err := s.syncFTP(r.Context()); err != nil {
		return err
	}
	s.audit(r, "ftp.delete", r.PathValue("id"), true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- directory privacy -------------------------------------------------------

type protectedView struct {
	ID       string   `json:"id"`
	DomainID string   `json:"domain_id"`
	Domain   string   `json:"domain"`
	Path     string   `json:"path"`
	Realm    string   `json:"realm"`
	Users    []string `json:"users"`
	Leech    int      `json:"leech_limit"`
	Blocked  []string `json:"blocked_users"`
}

func (s *Server) listProtected(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT p.id, p.domain_id, d.name, p.path, p.realm, p.leech_limit
		FROM protected_dirs p JOIN domains d ON d.id = p.domain_id WHERE d.account_id = ? ORDER BY d.name, p.path`, a.ID)
	if err != nil {
		return err
	}
	out := []protectedView{}
	index := map[string]int{}
	for rows.Next() {
		v := protectedView{Users: []string{}, Blocked: []string{}}
		if err := rows.Scan(&v.ID, &v.DomainID, &v.Domain, &v.Path, &v.Realm, &v.Leech); err != nil {
			rows.Close()
			return err
		}
		index[v.ID] = len(out)
		out = append(out, v)
	}
	rows.Close()
	urows, err := s.db.Query(r.Context(), `SELECT u.dir_id, u.username, u.disabled FROM protected_dir_users u
		JOIN protected_dirs p ON p.id = u.dir_id JOIN domains d ON d.id = p.domain_id WHERE d.account_id = ? ORDER BY u.username`, a.ID)
	if err != nil {
		return err
	}
	defer urows.Close()
	for urows.Next() {
		var id, u string
		var off int
		if err := urows.Scan(&id, &u, &off); err != nil {
			return err
		}
		if i, ok := index[id]; ok {
			out[i].Users = append(out[i].Users, u)
			if off == 1 {
				out[i].Blocked = append(out[i].Blocked, u)
			}
		}
	}
	writeJSON(w, 200, out)
	return urows.Err()
}

func (s *Server) createProtected(w http.ResponseWriter, r *http.Request) error {
	var req protectedView
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	d, err := s.domainOf(r.Context(), a.ID, req.DomainID)
	if err != nil {
		return err
	}
	req.Path = "/" + strings.Trim(strings.TrimSpace(req.Path), "/")
	if strings.TrimSpace(req.Realm) == "" {
		req.Realm = "Restricted area"
	}
	if err := provision.ValidateProtectedDir(provision.ProtectedDir{Path: req.Path, Realm: req.Realm}); err != nil {
		return err
	}
	req.ID, req.Domain, req.Users, req.Blocked, req.Leech = ids.New(), d.Name, []string{}, []string{}, 0
	if _, err := s.db.Exec(r.Context(), `INSERT INTO protected_dirs (id, domain_id, path, realm, created_at) VALUES (?,?,?,?,?)`,
		req.ID, d.ID, req.Path, req.Realm, time.Now().Unix()); err != nil {
		return err
	}
	// The password file must exist before nginx is told to use it.
	if err := s.prov.WriteHtpasswd(r.Context(), a.prov(), req.ID, nil); err != nil {
		return err
	}
	if err := s.rewriteVhosts(r.Context(), a); err != nil {
		_, _ = s.db.Exec(context.WithoutCancel(r.Context()), `DELETE FROM protected_dirs WHERE id = ?`, req.ID)
		return err
	}
	s.audit(r, "web.protect.create", d.Name+req.Path, true, "")
	writeJSON(w, 201, req)
	return nil
}

func (s *Server) protectedOf(ctx context.Context, accountID, id string) error {
	var n int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM protected_dirs p JOIN domains d ON d.id = p.domain_id WHERE p.id = ? AND d.account_id = ?`,
		id, accountID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return errNotFound()
	}
	return nil
}

func (s *Server) writeProtectedUsers(ctx context.Context, a *acct, id string) error {
	rows, err := s.db.Query(ctx, `SELECT username, password_hash FROM protected_dir_users WHERE dir_id = ? AND disabled = 0`, id)
	if err != nil {
		return err
	}
	users := map[string]string{}
	for rows.Next() {
		var u, h string
		if err := rows.Scan(&u, &h); err != nil {
			rows.Close()
			return err
		}
		users[u] = h
	}
	rows.Close()
	return s.prov.WriteHtpasswd(ctx, a.prov(), id, users)
}

func (s *Server) putProtectedUser(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	id, user := r.PathValue("id"), r.PathValue("user")
	if err := s.protectedOf(r.Context(), a.ID, id); err != nil {
		return err
	}
	if err := provision.HtUser(user); err != nil {
		return err
	}
	if len(req.Password) < 8 || len(req.Password) > 128 {
		return errBadRequest("the password must be 8 to 128 characters")
	}
	hash, err := auth.SHA512Crypt(req.Password)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM protected_dir_users WHERE dir_id = ? AND username = ?`, id, user); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO protected_dir_users (dir_id, username, password_hash) VALUES (?,?,?)`, id, user, hash); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM leech_hits WHERE dir_id = ? AND username = ?`, id, user); err != nil {
		return err
	}
	if err := s.writeProtectedUsers(r.Context(), a, id); err != nil {
		return err
	}
	s.audit(r, "web.protect.user", user, true, id)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) deleteProtectedUser(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	id := r.PathValue("id")
	if err := s.protectedOf(r.Context(), a.ID, id); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM protected_dir_users WHERE dir_id = ? AND username = ?`, id, r.PathValue("user")); err != nil {
		return err
	}
	if err := s.writeProtectedUsers(r.Context(), a, id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) deleteProtected(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	id := r.PathValue("id")
	if err := s.protectedOf(r.Context(), a.ID, id); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM protected_dirs WHERE id = ?`, id); err != nil {
		return err
	}
	if err := s.rewriteVhosts(r.Context(), a); err != nil {
		return err
	}
	_ = s.prov.RemoveHtpasswd(a.prov(), id)
	s.audit(r, "web.protect.delete", id, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- IP blocker and hotlink protection ----------------------------------------

func (s *Server) listIPBlocks(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, cidr, created_at FROM ip_blocks WHERE account_id = ? ORDER BY created_at DESC`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type v struct {
		ID        string `json:"id"`
		CIDR      string `json:"cidr"`
		CreatedAt int64  `json:"created_at"`
	}
	out := []v{}
	for rows.Next() {
		var x v
		if err := rows.Scan(&x.ID, &x.CIDR, &x.CreatedAt); err != nil {
			return err
		}
		out = append(out, x)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

const maxIPBlocks = 500

func (s *Server) addIPBlock(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		CIDR string `json:"cidr"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	cidr, err := provision.BlockedCIDR(req.CIDR)
	if err != nil {
		return err
	}
	if err := s.within(r.Context(), "ip_blocks", a.ID, maxIPBlocks, "blocked addresses"); err != nil {
		return err
	}
	id := ids.New()
	if _, err := s.db.Exec(r.Context(), `INSERT INTO ip_blocks (id, account_id, cidr, created_at) VALUES (?,?,?,?)`, id, a.ID, cidr, time.Now().Unix()); err != nil {
		return err
	}
	if err := s.rewriteVhosts(r.Context(), a); err != nil {
		_, _ = s.db.Exec(context.WithoutCancel(r.Context()), `DELETE FROM ip_blocks WHERE id = ?`, id)
		return err
	}
	s.audit(r, "web.ipblock.add", cidr, true, a.SysUser)
	writeJSON(w, 201, map[string]any{"id": id, "cidr": cidr})
	return nil
}

func (s *Server) deleteIPBlock(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM ip_blocks WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID); err != nil {
		return err
	}
	if err := s.rewriteVhosts(r.Context(), a); err != nil {
		return err
	}
	s.audit(r, "web.ipblock.delete", r.PathValue("id"), true, a.SysUser)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type hotlinkView struct {
	Enabled    bool     `json:"enabled"`
	Allowed    []string `json:"allowed"`
	Extensions []string `json:"extensions"`
	AllowEmpty bool     `json:"allow_empty"`
}

func (s *Server) hotlinkOf(ctx context.Context, accountID string) hotlinkView {
	v := hotlinkView{Allowed: []string{}, Extensions: strings.Fields("jpg jpeg gif png webp svg bmp mp4 mp3"), AllowEmpty: true}
	var en, empty int
	var allowed, ext string
	if err := s.db.QueryRow(ctx, `SELECT enabled, allowed, extensions, allow_empty FROM hotlink_settings WHERE account_id = ?`, accountID).
		Scan(&en, &allowed, &ext, &empty); err == nil {
		v = hotlinkView{Enabled: en == 1, Allowed: strings.Fields(allowed), Extensions: strings.Fields(ext), AllowEmpty: empty == 1}
	}
	return v
}

func (s *Server) getHotlink(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	writeJSON(w, 200, s.hotlinkOf(r.Context(), a.ID))
	return nil
}

func (s *Server) putHotlink(w http.ResponseWriter, r *http.Request) error {
	var req hotlinkView
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	for i := range req.Allowed {
		req.Allowed[i] = strings.ToLower(strings.TrimSpace(req.Allowed[i]))
	}
	for i := range req.Extensions {
		req.Extensions[i] = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(req.Extensions[i]), "."))
	}
	if len(req.Allowed) > 50 || len(req.Extensions) > 40 {
		return errBadRequest("too many entries")
	}
	if err := provision.ValidateHotlink(provision.Hotlink{Allowed: req.Allowed, Extensions: req.Extensions}); err != nil {
		return err
	}
	prev := s.hotlinkOf(r.Context(), a.ID)
	if _, err := s.db.Exec(r.Context(), `DELETE FROM hotlink_settings WHERE account_id = ?`, a.ID); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO hotlink_settings (account_id, enabled, allowed, extensions, allow_empty) VALUES (?,?,?,?,?)`,
		a.ID, b2i(req.Enabled), strings.Join(req.Allowed, " "), strings.Join(req.Extensions, " "), b2i(req.AllowEmpty)); err != nil {
		return err
	}
	if err := s.rewriteVhosts(r.Context(), a); err != nil {
		ctx := context.WithoutCancel(r.Context())
		_, _ = s.db.Exec(ctx, `UPDATE hotlink_settings SET enabled = ?, allowed = ?, extensions = ?, allow_empty = ? WHERE account_id = ?`,
			b2i(prev.Enabled), strings.Join(prev.Allowed, " "), strings.Join(prev.Extensions, " "), b2i(prev.AllowEmpty), a.ID)
		return err
	}
	s.audit(r, "web.hotlink.set", a.SysUser, true, "")
	writeJSON(w, 200, req)
	return nil
}

// webProtection adds the account-wide rules and the domain's protected
// directories to a vhost.
func (s *Server) webProtection(ctx context.Context, accountID, domainID string, v *provision.Vhost) error {
	rows, err := s.db.Query(ctx, `SELECT cidr FROM ip_blocks WHERE account_id = ? ORDER BY cidr`, accountID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return err
		}
		v.DenyIPs = append(v.DenyIPs, c)
	}
	rows.Close()
	if h := s.hotlinkOf(ctx, accountID); h.Enabled {
		v.Hotlink = &provision.Hotlink{Allowed: h.Allowed, Extensions: h.Extensions, AllowEmpty: h.AllowEmpty}
	}
	rows, err = s.db.Query(ctx, `SELECT id, path, realm FROM protected_dirs WHERE domain_id = ? ORDER BY length(path) DESC`, domainID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var d provision.ProtectedDir
		if err := rows.Scan(&d.ID, &d.Path, &d.Realm); err != nil {
			return err
		}
		v.Protected = append(v.Protected, d)
	}
	return rows.Err()
}

// ---- remote MySQL -------------------------------------------------------------

func (s *Server) listRemoteMySQL(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, host, note, created_at FROM remote_mysql_hosts WHERE account_id = ? ORDER BY host`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type v struct {
		ID        string `json:"id"`
		Host      string `json:"host"`
		Note      string `json:"note"`
		CreatedAt int64  `json:"created_at"`
	}
	out := []v{}
	for rows.Next() {
		var x v
		if err := rows.Scan(&x.ID, &x.Host, &x.Note, &x.CreatedAt); err != nil {
			return err
		}
		out = append(out, x)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

// syncRemoteMySQL gives every database user of the account the account's
// remote hosts, with access to the databases it is granted locally.
func (s *Server) syncRemoteMySQL(ctx context.Context, a *acct) error {
	hosts := []string{}
	rows, err := s.db.Query(ctx, `SELECT host FROM remote_mysql_hosts WHERE account_id = ?`, a.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			rows.Close()
			return err
		}
		hosts = append(hosts, h)
	}
	rows.Close()
	rows, err = s.db.Query(ctx, `SELECT u.name, COALESCE(i.name, '') FROM db_users u
		LEFT JOIN db_grants g ON g.db_user_id = u.id LEFT JOIN db_instances i ON i.id = g.db_id WHERE u.account_id = ?`, a.ID)
	if err != nil {
		return err
	}
	grants := map[string][]string{}
	for rows.Next() {
		var u, d string
		if err := rows.Scan(&u, &d); err != nil {
			rows.Close()
			return err
		}
		if _, ok := grants[u]; !ok {
			grants[u] = []string{}
		}
		if d != "" {
			grants[u] = append(grants[u], d)
		}
	}
	rows.Close()
	for u, dbs := range grants {
		if err := s.mariadb.SyncRemoteUser(ctx, u, hosts, dbs); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) addRemoteMySQL(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Host string `json:"host"`
		Note string `json:"note"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	req.Host = strings.TrimSpace(req.Host)
	if err := provision.RemoteHost(req.Host); err != nil {
		return err
	}
	if err := system.OneLine(req.Note, 120); err != nil {
		return err
	}
	if err := s.within(r.Context(), "remote_mysql_hosts", a.ID, 50, "remote hosts"); err != nil {
		return err
	}
	id := ids.New()
	if _, err := s.db.Exec(r.Context(), `INSERT INTO remote_mysql_hosts (id, account_id, host, note, created_at) VALUES (?,?,?,?,?)`,
		id, a.ID, req.Host, req.Note, time.Now().Unix()); err != nil {
		return err
	}
	if err := s.syncRemoteMySQL(r.Context(), a); err != nil {
		_, _ = s.db.Exec(context.WithoutCancel(r.Context()), `DELETE FROM remote_mysql_hosts WHERE id = ?`, id)
		return err
	}
	s.audit(r, "mysql.remote.add", req.Host, true, a.SysUser)
	writeJSON(w, 201, map[string]any{"id": id, "host": req.Host, "note": req.Note})
	return nil
}

func (s *Server) deleteRemoteMySQL(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM remote_mysql_hosts WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID); err != nil {
		return err
	}
	if err := s.syncRemoteMySQL(r.Context(), a); err != nil {
		return err
	}
	s.audit(r, "mysql.remote.delete", r.PathValue("id"), true, a.SysUser)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- Git ----------------------------------------------------------------------

type gitView struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Path      string            `json:"path"`
	CloneURL  string            `json:"clone_url"`
	CreatedAt int64             `json:"created_at"`
	Info      provision.GitInfo `json:"info"`
	SSHURL    string            `json:"ssh_url"`
}

func (s *Server) listGit(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, name, path, clone_url, created_at FROM git_repos WHERE account_id = ? ORDER BY name`, a.ID)
	if err != nil {
		return err
	}
	out := []gitView{}
	for rows.Next() {
		var v gitView
		if err := rows.Scan(&v.ID, &v.Name, &v.Path, &v.CloneURL, &v.CreatedAt); err != nil {
			rows.Close()
			return err
		}
		out = append(out, v)
	}
	rows.Close()
	host := s.setting(r.Context(), "hostname")
	if host == "" {
		host = a.PrimaryDomain
	}
	for i := range out {
		if s.cfg.DryRun {
			out[i].Info = provision.GitInfo{Branch: "main", Head: "3f2a9c1", Commit: "Update landing page copy", When: "2 hours ago"}
		} else {
			out[i].Info = s.prov.GitStatus(r.Context(), a.prov(), out[i].Path)
		}
		out[i].SSHURL = "ssh://" + a.SysUser + "@" + host + "/~/" + out[i].Path
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) createGit(w http.ResponseWriter, r *http.Request) error {
	var req gitView
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := system.OneLine(req.Name, 64); err != nil || req.Name == "" {
		return errBadRequest("the repository needs a name (max 64 characters)")
	}
	dir, err := provision.CleanRelDir(req.Path)
	if err != nil || dir == "" {
		return errBadRequest("choose a folder for the repository, such as repositories/site")
	}
	req.CloneURL = strings.TrimSpace(req.CloneURL)
	if req.CloneURL != "" {
		if err := provision.GitCloneURL(req.CloneURL); err != nil {
			return err
		}
	}
	if err := s.within(r.Context(), "git_repos", a.ID, 100, "repositories"); err != nil {
		return err
	}
	jail, err := files.Open(a.HomeDir, a.SysUID, a.SysUID)
	if err != nil {
		return fsErr(err)
	}
	if _, err := jail.Stat(dir); err == nil {
		jail.Close()
		return errConflict("that folder already exists; choose a new one")
	}
	jail.Close()
	if err := s.prov.GitCreate(r.Context(), a.prov(), provision.GitRepo{Dir: dir, CloneURL: req.CloneURL}); err != nil {
		return err
	}
	req.ID, req.Path, req.CreatedAt = ids.New(), dir, time.Now().Unix()
	if _, err := s.db.Exec(r.Context(), `INSERT INTO git_repos (id, account_id, name, path, clone_url, created_at) VALUES (?,?,?,?,?,?)`,
		req.ID, a.ID, req.Name, dir, req.CloneURL, req.CreatedAt); err != nil {
		return err
	}
	s.audit(r, "git.create", dir, true, req.CloneURL)
	writeJSON(w, 201, req)
	return nil
}

func (s *Server) gitOf(ctx context.Context, accountID, id string) (string, string, error) {
	var p, url string
	if err := s.db.QueryRow(ctx, `SELECT path, clone_url FROM git_repos WHERE id = ? AND account_id = ?`, id, accountID).Scan(&p, &url); err != nil {
		return "", "", errNotFound()
	}
	return p, url, nil
}

func (s *Server) pullGit(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	dir, url, err := s.gitOf(r.Context(), a.ID, r.PathValue("id"))
	if err != nil {
		return err
	}
	if url == "" {
		return errBadRequest("this repository has no remote to pull from")
	}
	out, err := s.prov.GitPull(r.Context(), a.prov(), dir)
	if err != nil {
		return &apiError{422, "git_failed", "git pull failed; the branch may have diverged from the remote"}
	}
	s.audit(r, "git.pull", dir, true, "")
	writeJSON(w, 200, map[string]string{"output": out})
	return nil
}

// deleteGit forgets a repository; its files stay in the home directory.
func (s *Server) deleteGit(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if _, _, err := s.gitOf(r.Context(), a.ID, r.PathValue("id")); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM git_repos WHERE id = ?`, r.PathValue("id")); err != nil {
		return err
	}
	s.audit(r, "git.delete", r.PathValue("id"), true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- DNSSEC -------------------------------------------------------------------

func (s *Server) zoneOf(ctx context.Context, accountID, zone string) error {
	var n int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM domains WHERE account_id = ? AND name = ? AND kind IN ('primary','addon')`,
		accountID, strings.ToLower(zone)).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return errNotFound()
	}
	return nil
}

func (s *Server) getDNSSEC(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	zone := r.PathValue("zone")
	if err := s.zoneOf(r.Context(), a.ID, zone); err != nil {
		return err
	}
	var on int
	_ = s.db.QueryRow(r.Context(), `SELECT dnssec FROM dns_zones WHERE name = ?`, zone).Scan(&on)
	keys, err := s.prov.DNS().DNSSECKeys(r.Context(), zone)
	if err != nil {
		return err
	}
	if keys == nil {
		keys = []provision.DNSSECKey{}
	}
	if s.cfg.DryRun && on == 1 && len(keys) == 0 {
		keys = []provision.DNSSECKey{{ID: 1, KeyType: "csk", Active: true, Algorithm: "ECDSAP256SHA256",
			DS: []string{zone + ". IN DS 41263 13 2 6c3d8f0b2c7a4f9e1d5b3a8c7e6f4d2b1a9c8e7f6d5b4a3c2e1f0d9c8b7a6e5f"}}}
	}
	writeJSON(w, 200, map[string]any{"enabled": on == 1, "keys": keys})
	return nil
}

func (s *Server) putDNSSEC(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	zone := strings.ToLower(r.PathValue("zone"))
	if err := s.zoneOf(r.Context(), a.ID, zone); err != nil {
		return err
	}
	if err := s.prov.DNS().SetDNSSEC(r.Context(), zone, req.Enabled); err != nil {
		return err
	}
	res, err := s.db.Exec(r.Context(), `UPDATE dns_zones SET dnssec = ? WHERE name = ?`, b2i(req.Enabled), zone)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		_, _ = s.db.Exec(r.Context(), `INSERT INTO dns_zones (id, account_id, name, dnssec, created_at) VALUES (?,?,?,?,?)`,
			ids.New(), a.ID, zone, b2i(req.Enabled), time.Now().Unix())
	}
	s.audit(r, "dns.dnssec", zone, true, map[bool]string{true: "on", false: "off"}[req.Enabled])
	return s.getDNSSEC(w, r)
}

// syncFTPLogged is syncFTP for callers that must not fail on it.
func (s *Server) syncFTPLogged(ctx context.Context) {
	if err := s.syncFTP(ctx); err != nil {
		s.log.Error("ftp sync", "err", err)
	}
}
