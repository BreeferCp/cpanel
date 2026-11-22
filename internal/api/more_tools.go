package api

import (
	"net/http"
	"strings"

	"github.com/breefercp/breefercp/internal/provision"
	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/store"
)

func (s *Server) moreToolRoutes(mux *http.ServeMux) {
	a := "/api/v1/accounts/{aid}"
	r, w := rbac.HostingRead, rbac.HostingWrite

	s.route(mux, "GET "+a+"/mail-usage", r, s.mailUsage)
	s.route(mux, "POST "+a+"/databases/{id}/maintenance", w, s.databaseMaintenance)
	s.route(mux, "GET "+a+"/cron-email", r, s.getCronEmail)
	s.route(mux, "PUT "+a+"/cron-email", w, s.putCronEmail)
}

// ---- email disk usage -------------------------------------------------------

func (s *Server) mailUsage(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, address, quota_mb FROM mailboxes WHERE account_id = ? ORDER BY address`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type usage struct {
		ID       string `json:"id"`
		Address  string `json:"address"`
		QuotaMB  int64  `json:"quota_mb"`
		Bytes    int64  `json:"bytes"`
		Messages int    `json:"messages"`
	}
	out := []usage{}
	for rows.Next() {
		var u usage
		if err := rows.Scan(&u.ID, &u.Address, &u.QuotaMB); err != nil {
			return err
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(out) > 0 {
		used, err := s.prov.MailUsage(r.Context(), a.prov())
		if err != nil {
			return err
		}
		for i := range out {
			out[i].Bytes, out[i].Messages = used[out[i].Address].Bytes, used[out[i].Address].Messages
		}
	}
	writeJSON(w, 200, out)
	return nil
}

// ---- database check and repair ------------------------------------------------

func (s *Server) databaseMaintenance(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Action string `json:"action"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Action != "check" && req.Action != "repair" {
		return errBadRequest("action must be check or repair")
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	var name string
	if err := s.db.QueryRow(r.Context(), `SELECT name FROM db_instances WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID).Scan(&name); err != nil {
		return store.IsNoRows(err)
	}
	res, err := s.prov.CheckDatabase(r.Context(), name, req.Action == "repair")
	if err != nil {
		return err
	}
	s.audit(r, "database."+req.Action, name, true, "")
	writeJSON(w, 200, map[string]any{"tables": res})
	return nil
}

// ---- cron email -----------------------------------------------------------------

func (s *Server) getCronEmail(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	var v string
	if err := s.db.QueryRow(r.Context(), `SELECT cron_email FROM accounts WHERE id = ?`, a.ID).Scan(&v); err != nil {
		return err
	}
	writeJSON(w, 200, map[string]string{"email": v})
	return nil
}

func (s *Server) putCronEmail(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Email string `json:"email"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	v := strings.ToLower(strings.TrimSpace(req.Email))
	if v != "" && !provision.CronMailTo(v) {
		return errBadRequest("enter one email address, or leave it empty to discard the output")
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE accounts SET cron_email = ? WHERE id = ?`, v, a.ID); err != nil {
		return err
	}
	if err := s.syncCron(r.Context(), a); err != nil {
		return err
	}
	s.audit(r, "cron.email", a.SysUser, true, v)
	w.WriteHeader(http.StatusNoContent)
	return nil
}
