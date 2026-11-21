package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/breefercp/breefercp/internal/ids"
)

// audit records a security-relevant action. Failures to write are logged but
// never block the user action.
func (s *Server) audit(r *http.Request, action, target string, ok bool, detail string) {
	p := principalFrom(r.Context())
	var uid, uname string
	if p != nil {
		uid, uname = p.UserID, p.Username
	}
	s.auditAs(r.Context(), uid, uname, s.clientIP(r), action, target, ok, detail)
}

func (s *Server) auditAs(ctx context.Context, uid, uname, ip, action, target string, ok bool, detail string) {
	okInt := 0
	if ok {
		okInt = 1
	}
	if len(detail) > 500 {
		detail = detail[:500]
	}
	// Detached context: the audit trail must survive client disconnects.
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if _, err := s.db.Exec(cctx,
		`INSERT INTO audit_log (id, at, actor_id, actor_name, action, target, ip, detail, ok) VALUES (?,?,?,?,?,?,?,?,?)`,
		ids.New(), time.Now().Unix(), uid, uname, action, target, ip, detail, okInt); err != nil {
		s.log.Error("audit write failed", "err", err, "action", action)
	}
}

type auditEntry struct {
	ID     string `json:"id"`
	At     int64  `json:"at"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Target string `json:"target"`
	IP     string `json:"ip"`
	Detail string `json:"detail"`
	OK     bool   `json:"ok"`
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) error {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.Query(r.Context(),
		`SELECT id, at, actor_name, action, target, ip, detail, ok FROM audit_log ORDER BY at DESC, id LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []auditEntry{}
	for rows.Next() {
		var e auditEntry
		var ok int
		if err := rows.Scan(&e.ID, &e.At, &e.Actor, &e.Action, &e.Target, &e.IP, &e.Detail, &ok); err != nil {
			return err
		}
		e.OK = ok == 1
		out = append(out, e)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}
