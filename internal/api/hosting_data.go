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
	"github.com/breefercp/breefercp/internal/store"
	"github.com/breefercp/breefercp/internal/system"
)

// ---- databases ----------------------------------------------------------------

// prefixed returns "<sysuser>_<name>" and validates the combined identifier.
func prefixed(sysUser, name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	full := sysUser + "_" + name
	if err := system.DBName(full); err != nil {
		return "", err
	}
	if len(full) > 32 {
		return "", errBadRequest("name is too long once the account prefix is added (max 32 characters in total)")
	}
	return full, nil
}

func (s *Server) listDatabases(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	type dv struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		SizeMB int64    `json:"size_mb"`
		Users  []string `json:"users"`
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, name, size_mb FROM db_instances WHERE account_id = ? ORDER BY name`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []dv{}
	for rows.Next() {
		var d dv
		if err := rows.Scan(&d.ID, &d.Name, &d.SizeMB); err != nil {
			return err
		}
		d.Users = []string{}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range out {
		urows, err := s.db.Query(r.Context(), `SELECT u.id FROM db_grants g JOIN db_users u ON u.id = g.db_user_id WHERE g.db_id = ?`, out[i].ID)
		if err != nil {
			return err
		}
		for urows.Next() {
			var uid string
			if err := urows.Scan(&uid); err != nil {
				urows.Close()
				return err
			}
			out[i].Users = append(out[i].Users, uid)
		}
		urows.Close()
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) createDatabase(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	full, err := prefixed(a.SysUser, req.Name)
	if err != nil {
		return err
	}
	if err := s.within(r.Context(), "db_instances", a.ID, a.pkg.MaxDatabases, "databases"); err != nil {
		return err
	}
	id := ids.New()
	if _, err := s.db.Exec(r.Context(), `INSERT INTO db_instances (id, account_id, name, created_at) VALUES (?,?,?,?)`, id, a.ID, full, time.Now().Unix()); err != nil {
		return err
	}
	if err := s.mariadb.CreateDatabase(r.Context(), full); err != nil {
		_, _ = s.db.Exec(context.WithoutCancel(r.Context()), `DELETE FROM db_instances WHERE id = ?`, id)
		return err
	}
	s.audit(r, "database.create", full, true, "")
	writeJSON(w, 201, map[string]string{"id": id, "name": full})
	return nil
}

func (s *Server) deleteDatabase(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	var name string
	if err := s.db.QueryRow(r.Context(), `SELECT name FROM db_instances WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID).Scan(&name); err != nil {
		return store.IsNoRows(err)
	}
	if err := s.mariadb.DropDatabase(r.Context(), name); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM db_instances WHERE id = ?`, r.PathValue("id")); err != nil {
		return err
	}
	s.audit(r, "database.delete", name, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) listDBUsers(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, name FROM db_users WHERE account_id = ? ORDER BY name`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type uv struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	out := []uv{}
	for rows.Next() {
		var u uv
		if err := rows.Scan(&u.ID, &u.Name); err != nil {
			return err
		}
		out = append(out, u)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) createDBUser(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	full, err := prefixed(a.SysUser, req.Name)
	if err != nil {
		return err
	}
	if err := auth.CheckPasswordPolicy(req.Password); err != nil {
		return errBadRequest(err.Error())
	}
	id := ids.New()
	if _, err := s.db.Exec(r.Context(), `INSERT INTO db_users (id, account_id, name, created_at) VALUES (?,?,?,?)`, id, a.ID, full, time.Now().Unix()); err != nil {
		return err
	}
	if err := s.mariadb.CreateUser(r.Context(), full, req.Password); err != nil {
		_, _ = s.db.Exec(context.WithoutCancel(r.Context()), `DELETE FROM db_users WHERE id = ?`, id)
		return err
	}
	s.audit(r, "dbuser.create", full, true, "")
	writeJSON(w, 201, map[string]string{"id": id, "name": full})
	return nil
}

func (s *Server) dbUserName(ctx context.Context, accountID, id string) (string, error) {
	var name string
	err := s.db.QueryRow(ctx, `SELECT name FROM db_users WHERE id = ? AND account_id = ?`, id, accountID).Scan(&name)
	return name, store.IsNoRows(err)
}

func (s *Server) setDBUserPassword(w http.ResponseWriter, r *http.Request) error {
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
	name, err := s.dbUserName(r.Context(), a.ID, r.PathValue("id"))
	if err != nil {
		return err
	}
	if err := auth.CheckPasswordPolicy(req.Password); err != nil {
		return errBadRequest(err.Error())
	}
	if err := s.mariadb.SetUserPassword(r.Context(), name, req.Password); err != nil {
		return err
	}
	s.audit(r, "dbuser.password", name, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) deleteDBUser(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	name, err := s.dbUserName(r.Context(), a.ID, r.PathValue("id"))
	if err != nil {
		return err
	}
	if err := s.mariadb.DropUser(r.Context(), name); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM db_users WHERE id = ?`, r.PathValue("id")); err != nil {
		return err
	}
	s.audit(r, "dbuser.delete", name, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) grantDB(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Privileges []string `json:"privileges"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	var dbName string
	if err := s.db.QueryRow(r.Context(), `SELECT name FROM db_instances WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID).Scan(&dbName); err != nil {
		return store.IsNoRows(err)
	}
	user, err := s.dbUserName(r.Context(), a.ID, r.PathValue("uid"))
	if err != nil {
		return err
	}
	if err := s.mariadb.Grant(r.Context(), dbName, user, req.Privileges); err != nil {
		return err
	}
	privs := strings.Join(req.Privileges, ",")
	if privs == "" {
		privs = "ALL"
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM db_grants WHERE db_id = ? AND db_user_id = ?`, r.PathValue("id"), r.PathValue("uid")); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO db_grants (db_id, db_user_id, privileges) VALUES (?,?,?)`, r.PathValue("id"), r.PathValue("uid"), privs); err != nil {
		return err
	}
	s.audit(r, "database.grant", dbName, true, user)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) revokeDB(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	var dbName string
	if err := s.db.QueryRow(r.Context(), `SELECT name FROM db_instances WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID).Scan(&dbName); err != nil {
		return store.IsNoRows(err)
	}
	user, err := s.dbUserName(r.Context(), a.ID, r.PathValue("uid"))
	if err != nil {
		return err
	}
	if err := s.mariadb.Revoke(r.Context(), dbName, user); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM db_grants WHERE db_id = ? AND db_user_id = ?`, r.PathValue("id"), r.PathValue("uid")); err != nil {
		return err
	}
	s.audit(r, "database.revoke", dbName, true, user)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) dropAccountDatabases(ctx context.Context, accountID string) error {
	for _, q := range []struct {
		sel  string
		drop func(context.Context, string) error
	}{
		{`SELECT name FROM db_instances WHERE account_id = ?`, s.mariadb.DropDatabase},
		{`SELECT name FROM db_users WHERE account_id = ?`, s.mariadb.DropUser},
	} {
		rows, err := s.db.Query(ctx, q.sel, accountID)
		if err != nil {
			return err
		}
		var names []string
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				rows.Close()
				return err
			}
			names = append(names, n)
		}
		rows.Close()
		for _, n := range names {
			if err := q.drop(ctx, n); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---- mail ---------------------------------------------------------------------

type mailbox struct {
	ID      string `json:"id"`
	Address string `json:"address"`
	QuotaMB int    `json:"quota_mb"`
	UsedMB  int    `json:"used_mb"`
	Active  bool   `json:"active"`
}

func (s *Server) listMailboxes(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, address, quota_mb, used_mb, active FROM mailboxes WHERE account_id = ? ORDER BY address`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []mailbox{}
	for rows.Next() {
		var m mailbox
		var act int
		if err := rows.Scan(&m.ID, &m.Address, &m.QuotaMB, &m.UsedMB, &act); err != nil {
			return err
		}
		m.Active = act == 1
		out = append(out, m)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) createMailbox(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		LocalPart string `json:"local_part"`
		DomainID  string `json:"domain_id"`
		Password  string `json:"password"`
		QuotaMB   int    `json:"quota_mb"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	req.LocalPart = strings.ToLower(strings.TrimSpace(req.LocalPart))
	if err := system.LocalPart(req.LocalPart); err != nil {
		return err
	}
	d, err := s.domainOf(r.Context(), a.ID, req.DomainID)
	if err != nil {
		return errBadRequest("choose one of your domains")
	}
	if err := auth.CheckPasswordPolicy(req.Password); err != nil {
		return errBadRequest(err.Error())
	}
	if req.QuotaMB <= 0 || req.QuotaMB > 102400 {
		req.QuotaMB = 1024
	}
	if err := s.within(r.Context(), "mailboxes", a.ID, a.pkg.MaxMailboxes, "mailboxes"); err != nil {
		return err
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return err
	}
	m := mailbox{ID: ids.New(), Address: req.LocalPart + "@" + d.Name, QuotaMB: req.QuotaMB, Active: true}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO mailboxes (id, account_id, domain_id, local_part, address, password_hash, quota_mb, created_at)
		VALUES (?,?,?,?,?,?,?,?)`, m.ID, a.ID, d.ID, req.LocalPart, m.Address, "{ARGON2ID}"+hash, m.QuotaMB, time.Now().Unix()); err != nil {
		return err
	}
	if err := s.prov.EnsureMaildir(r.Context(), a.prov(), d.Name, req.LocalPart); err != nil {
		_, _ = s.db.Exec(context.WithoutCancel(r.Context()), `DELETE FROM mailboxes WHERE id = ?`, m.ID)
		return err
	}
	s.audit(r, "mailbox.create", m.Address, true, "")
	writeJSON(w, 201, m)
	return nil
}

func (s *Server) setMailboxPassword(w http.ResponseWriter, r *http.Request) error {
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
	if err := auth.CheckPasswordPolicy(req.Password); err != nil {
		return errBadRequest(err.Error())
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(r.Context(), `UPDATE mailboxes SET password_hash = ? WHERE id = ? AND account_id = ?`, "{ARGON2ID}"+hash, r.PathValue("id"), a.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	s.audit(r, "mailbox.password", r.PathValue("id"), true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) deleteMailbox(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	var local, dom string
	if err := s.db.QueryRow(r.Context(), `SELECT m.local_part, d.name FROM mailboxes m JOIN domains d ON d.id = m.domain_id
		WHERE m.id = ? AND m.account_id = ?`, r.PathValue("id"), a.ID).Scan(&local, &dom); err != nil {
		return store.IsNoRows(err)
	}
	if err := s.prov.RemoveMaildir(r.Context(), a.prov(), dom, local); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM mailboxes WHERE id = ?`, r.PathValue("id")); err != nil {
		return err
	}
	s.audit(r, "mailbox.delete", local+"@"+dom, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type mailAlias struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

func (s *Server) listAliases(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, source, destination FROM mail_aliases WHERE account_id = ? ORDER BY source`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []mailAlias{}
	for rows.Next() {
		var m mailAlias
		if err := rows.Scan(&m.ID, &m.Source, &m.Destination); err != nil {
			return err
		}
		out = append(out, m)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) createAlias(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		LocalPart   string `json:"local_part"`
		DomainID    string `json:"domain_id"`
		Destination string `json:"destination"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	req.LocalPart = strings.ToLower(strings.TrimSpace(req.LocalPart))
	if req.LocalPart != "*" { // catch-all
		if err := system.LocalPart(req.LocalPart); err != nil {
			return err
		}
	}
	d, err := s.domainOf(r.Context(), a.ID, req.DomainID)
	if err != nil {
		return errBadRequest("choose one of your domains")
	}
	dest := strings.ToLower(strings.TrimSpace(req.Destination))
	if !strings.Contains(dest, "@") || strings.ContainsAny(dest, " ,;\r\n") {
		return errBadRequest("enter one valid destination address")
	}
	m := mailAlias{ID: ids.New(), Source: req.LocalPart + "@" + d.Name, Destination: dest}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO mail_aliases (id, account_id, source, destination, created_at) VALUES (?,?,?,?,?)`,
		m.ID, a.ID, m.Source, m.Destination, time.Now().Unix()); err != nil {
		return err
	}
	s.audit(r, "alias.create", m.Source, true, dest)
	writeJSON(w, 201, m)
	return nil
}

func (s *Server) deleteAlias(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM mail_aliases WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) enableDKIM(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Domain string `json:"domain"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if err := s.zoneOwned(r.Context(), a, req.Domain); err != nil {
		return err
	}
	k, err := s.prov.GenerateDKIM(r.Context(), req.Domain)
	if err != nil {
		return err
	}
	if err := s.prov.DNS().UpsertRecord(r.Context(), req.Domain, provision.Record{Name: k.DNSName, Type: "TXT", TTL: 3600, Content: k.DNSValue}); err != nil {
		return err
	}
	dmarc := provision.Record{Name: "_dmarc", Type: "TXT", TTL: 3600, Content: fmt.Sprintf(`"v=DMARC1; p=quarantine; rua=mailto:postmaster@%s"`, req.Domain)}
	if err := s.prov.DNS().UpsertRecord(r.Context(), req.Domain, dmarc); err != nil {
		return err
	}
	s.audit(r, "mail.dkim", req.Domain, true, "")
	writeJSON(w, 200, map[string]string{"selector": k.Selector})
	return nil
}

// ---- apps ---------------------------------------------------------------------

type appRow struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Runtime  string            `json:"runtime"`
	Image    string            `json:"image,omitempty"`
	Command  string            `json:"command,omitempty"`
	Port     int               `json:"port"`
	DomainID string            `json:"domain_id,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	Status   string            `json:"status"`
}

func (s *Server) listApps(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, name, runtime, image, port, COALESCE(domain_id,''), status FROM apps WHERE account_id = ? ORDER BY name`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []appRow{}
	for rows.Next() {
		var v appRow
		if err := rows.Scan(&v.ID, &v.Name, &v.Runtime, &v.Image, &v.Port, &v.DomainID, &v.Status); err != nil {
			return err
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) createApp(w http.ResponseWriter, r *http.Request) error {
	var req appRow
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if err := s.within(r.Context(), "apps", a.ID, a.pkg.MaxApps, "applications"); err != nil {
		return err
	}
	if req.Port == 0 {
		if err := s.db.QueryRow(r.Context(), `SELECT COALESCE(MAX(port), 29999) + 1 FROM apps`).Scan(&req.Port); err != nil {
			return err
		}
	}
	dir := path.Join(a.HomeDir, "apps", req.Name)
	papp := provision.App{Name: req.Name, Runtime: req.Runtime, Command: req.Command, Dir: dir, Image: req.Image, Port: req.Port, Env: req.Env}
	// Dry validation before touching anything.
	if req.Runtime != "docker" {
		if _, err := provision.RenderAppUnit(a.prov(), papp); err != nil {
			return err
		}
	}
	req.ID, req.Status = ids.New(), "running"
	if _, err := s.db.Exec(r.Context(), `INSERT INTO apps (id, account_id, domain_id, name, runtime, image, port, env, status, created_at)
		VALUES (?,?,NULLIF(?, ''),?,?,?,?,?,?,?)`, req.ID, a.ID, req.DomainID, req.Name, req.Runtime, req.Image, req.Port, "{}", req.Status, time.Now().Unix()); err != nil {
		return err
	}
	if _, err := s.prov.Runner().Run(r.Context(), "install", "-d", "-m", "0750", "-o", a.SysUser, "-g", a.SysUser, dir); err != nil {
		_, _ = s.db.Exec(context.WithoutCancel(r.Context()), `DELETE FROM apps WHERE id = ?`, req.ID)
		return err
	}
	if err := s.prov.DeployApp(r.Context(), a.prov(), papp); err != nil {
		_, _ = s.db.Exec(context.WithoutCancel(r.Context()), `DELETE FROM apps WHERE id = ?`, req.ID)
		return err
	}
	if req.DomainID != "" {
		if d, err := s.domainOf(r.Context(), a.ID, req.DomainID); err == nil {
			v, _ := s.vhostFor(r.Context(), d)
			v.ProxyPort = req.Port
			if err := s.prov.WriteVhost(r.Context(), v, a.prov()); err != nil {
				return err
			}
		}
	}
	s.audit(r, "app.create", req.Name, true, req.Runtime)
	req.Env = nil
	writeJSON(w, 201, req)
	return nil
}

func (s *Server) appByID(ctx context.Context, a *acct, id string) (appRow, error) {
	var v appRow
	err := s.db.QueryRow(ctx, `SELECT id, name, runtime, image, port, status FROM apps WHERE id = ? AND account_id = ?`, id, a.ID).
		Scan(&v.ID, &v.Name, &v.Runtime, &v.Image, &v.Port, &v.Status)
	return v, store.IsNoRows(err)
}

func (s *Server) appAction(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	v, err := s.appByID(r.Context(), a, r.PathValue("id"))
	if err != nil {
		return err
	}
	action := r.PathValue("action")
	if err := s.prov.AppAction(r.Context(), a.prov(), provision.App{Name: v.Name, Runtime: v.Runtime}, action); err != nil {
		return err
	}
	status := map[string]string{"start": "running", "restart": "running", "stop": "stopped"}[action]
	if _, err := s.db.Exec(r.Context(), `UPDATE apps SET status = ? WHERE id = ?`, status, v.ID); err != nil {
		return err
	}
	s.audit(r, "app."+action, v.Name, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) deleteApp(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	v, err := s.appByID(r.Context(), a, r.PathValue("id"))
	if err != nil {
		return err
	}
	if err := s.prov.RemoveApp(r.Context(), a.prov(), provision.App{Name: v.Name, Runtime: v.Runtime}); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM apps WHERE id = ?`, v.ID); err != nil {
		return err
	}
	s.audit(r, "app.delete", v.Name, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}
