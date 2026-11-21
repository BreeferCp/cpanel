package api

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/auth"
	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/store"
)

// Bundled web tools (phpMyAdmin, Roundcube) are served by the system web
// server under tools_url (default https://<hostname>:8443).
//
// phpMyAdmin single sign-on: the panel creates a temporary MariaDB user that
// can reach only the account's databases, and hands phpMyAdmin a one-time
// token. phpMyAdmin's signon script (deploy/phpmyadmin/breefercp-signon.php)
// redeems the token over loopback for the temporary credentials. The
// customer's real database passwords are never involved, and the temporary
// user is dropped when it expires.

const (
	pmaRedeemWindow = 2 * time.Minute
	pmaUserTTL      = 2 * time.Hour
)

func (s *Server) toolRoutes(mux *http.ServeMux) {
	s.route(mux, "GET /api/v1/tools", rbac.HostingRead, s.toolsInfo)
	s.route(mux, "POST /api/v1/accounts/{aid}/phpmyadmin", rbac.HostingWrite, s.pmaLogin)
	mux.HandleFunc("POST /api/v1/internal/pma/redeem", s.wrap(s.pmaRedeem))
}

func (s *Server) toolsURL(ctx context.Context) string {
	if v := s.setting(ctx, "tools_url"); v != "" {
		return strings.TrimRight(v, "/")
	}
	host := s.setting(ctx, "hostname")
	if host == "" {
		host = "localhost"
	}
	return "https://" + host + ":8443"
}

func (s *Server) toolsInfo(w http.ResponseWriter, r *http.Request) error {
	base := s.toolsURL(r.Context())
	writeJSON(w, 200, map[string]string{"phpmyadmin": base + "/phpmyadmin/", "webmail": base + "/webmail/"})
	return nil
}

func (s *Server) pmaLogin(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if a.Status != "active" {
		return errConflict("account is suspended")
	}
	rows, err := s.db.Query(r.Context(), `SELECT name FROM db_instances WHERE account_id = ?`, a.ID)
	if err != nil {
		return err
	}
	var dbs []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return err
		}
		dbs = append(dbs, n)
	}
	rows.Close()
	if len(dbs) == 0 {
		return errConflict("create a database first")
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		return err
	}
	user := "pma_" + strings.ToLower(hash[:10])
	pass, err := auth.RandomPassword(32)
	if err != nil {
		return err
	}
	if err := s.mariadb.CreateUser(r.Context(), user, pass); err != nil {
		return err
	}
	for _, d := range dbs {
		if err := s.mariadb.Grant(r.Context(), d, user, nil); err != nil {
			_ = s.mariadb.DropUser(context.WithoutCancel(r.Context()), user)
			return err
		}
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO pma_sessions (token_hash, account_id, db_user, password, expires_at) VALUES (?,?,?,?,?)`,
		hash, a.ID, user, pass, time.Now().Add(pmaUserTTL).Unix()); err != nil {
		return err
	}
	s.audit(r, "phpmyadmin.login", a.SysUser, true, "")
	writeJSON(w, 200, map[string]string{"url": s.toolsURL(r.Context()) + "/phpmyadmin/breefercp-signon.php?token=" + token})
	return nil
}

// pmaRedeem is called by phpMyAdmin's signon script on the same host. It is
// only reachable from loopback and each token works once, within minutes.
func (s *Server) pmaRedeem(w http.ResponseWriter, r *http.Request) error {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return errForbidden()
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	hash := auth.HashToken(req.Token)
	var user, pass string
	var expires int64
	var redeemed int
	err := s.db.QueryRow(r.Context(), `SELECT db_user, password, expires_at, redeemed FROM pma_sessions WHERE token_hash = ?`, hash).
		Scan(&user, &pass, &expires, &redeemed)
	if err != nil {
		return &apiError{401, "invalid_token", "invalid or expired token"}
	}
	issued := time.Unix(expires, 0).Add(-pmaUserTTL)
	if redeemed == 1 || time.Since(issued) > pmaRedeemWindow {
		return &apiError{401, "invalid_token", "invalid or expired token"}
	}
	// Mark used and forget the password; only phpMyAdmin's session holds it now.
	if _, err := s.db.Exec(r.Context(), `UPDATE pma_sessions SET redeemed = 1, password = '' WHERE token_hash = ?`, hash); err != nil {
		return err
	}
	writeJSON(w, 200, map[string]string{"user": user, "password": pass, "host": "localhost"})
	return nil
}

// purgePMAUsers drops temporary phpMyAdmin users whose lifetime has ended.
func (s *Server) purgePMAUsers(ctx context.Context) {
	rows, err := s.db.Query(ctx, `SELECT token_hash, db_user FROM pma_sessions WHERE expires_at < ?`, time.Now().Unix())
	if err != nil {
		return
	}
	type row struct{ hash, user string }
	var list []row
	for rows.Next() {
		var x row
		if rows.Scan(&x.hash, &x.user) == nil {
			list = append(list, x)
		}
	}
	rows.Close()
	for _, x := range list {
		if err := s.mariadb.DropUser(ctx, x.user); err != nil {
			s.log.Error("drop phpMyAdmin temp user", "user", x.user, "err", err)
			continue
		}
		_, _ = s.db.Exec(ctx, `DELETE FROM pma_sessions WHERE token_hash = ?`, x.hash)
	}
}

var _ = store.ErrNotFound
