package api

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/breefercp/breefercp/internal/auth"
	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/store"
)

const (
	maxFailedLogins = 5
	lockDuration    = 15 * time.Minute
	pendingTTL      = 5 * time.Minute
)

// totpUsed remembers the last accepted TOTP step per user to block replays.
var totpUsed sync.Map // userID -> uint64

type userRow struct {
	ID           string
	Username     string
	Email        string
	PasswordHash string
	Role         rbac.Role
	OwnerID      string
	TOTPSecret   string
	TOTPEnabled  bool
	Recovery     string
	Suspended    bool
	Failed       int
	LockedUntil  int64
	Locale       string
	Theme        string
}

func (s *Server) userByUsername(ctx context.Context, name string) (*userRow, error) {
	return s.scanUser(s.db.QueryRow(ctx, userSelect+` WHERE username = ?`, name))
}

func (s *Server) userByID(ctx context.Context, id string) (*userRow, error) {
	return s.scanUser(s.db.QueryRow(ctx, userSelect+` WHERE id = ?`, id))
}

const userSelect = `SELECT id, username, email, password_hash, role, COALESCE(owner_id,''), totp_secret, totp_enabled,
	recovery_codes, suspended, failed_logins, locked_until, locale, theme FROM users`

type rowScanner interface{ Scan(dest ...any) error }

func (s *Server) scanUser(row rowScanner) (*userRow, error) {
	var u userRow
	var role string
	var totp, susp int
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &role, &u.OwnerID, &u.TOTPSecret, &totp,
		&u.Recovery, &susp, &u.Failed, &u.LockedUntil, &u.Locale, &u.Theme)
	if err != nil {
		return nil, store.IsNoRows(err)
	}
	u.Role, u.TOTPEnabled, u.Suspended = rbac.Role(role), totp == 1, susp == 1
	return &u, nil
}

type userView struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	Role        rbac.Role `json:"role"`
	TOTPEnabled bool      `json:"totp_enabled"`
	Locale      string    `json:"locale"`
	Theme       string    `json:"theme"`
	AccountID   string    `json:"account_id,omitempty"`
	IsMaster    bool      `json:"is_master"`
}

func (s *Server) view(ctx context.Context, u *userRow) userView {
	v := userView{ID: u.ID, Username: u.Username, Email: u.Email, Role: u.Role,
		TOTPEnabled: u.TOTPEnabled, Locale: u.Locale, Theme: u.Theme}
	_ = s.db.QueryRow(ctx, `SELECT id FROM accounts WHERE user_id = ?`, u.ID).Scan(&v.AccountID)
	var master int
	_ = s.db.QueryRow(ctx, `SELECT is_master FROM users WHERE id = ?`, u.ID).Scan(&master)
	v.IsMaster = master == 1
	return v
}

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	var req loginReq
	if err := decode(r, &req); err != nil {
		return err
	}
	ip := s.clientIP(r)
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	// Only failures count, so many legitimate sign-ins from one office IP
	// never trip the limiter.
	rule := s.loginRuleFor(r.Context(), ip)
	if rule == "deny" {
		s.auditAs(r.Context(), "", req.Username, ip, "auth.login", "", false, "address denied by host access rule")
		return &apiError{403, "access_denied", "sign-in from your address is not allowed"}
	}
	// Allow-listed addresses (office, monitoring) are never rate limited.
	if rule != "allow" && (s.loginLimit.blocked("ip:"+ip) || s.loginLimit.blocked("user:"+req.Username)) {
		return errTooMany()
	}
	failedAttempt := func() {
		s.loginLimit.fail("ip:" + ip)
		s.loginLimit.fail("user:" + req.Username)
	}
	invalid := &apiError{401, "invalid_credentials", "invalid username or password"}
	u, err := s.userByUsername(r.Context(), req.Username)
	if err != nil {
		auth.DummyVerify(req.Password) // equalise timing with the found-user path
		failedAttempt()
		s.auditAs(r.Context(), "", req.Username, ip, "auth.login", "", false, "unknown user")
		return invalid
	}
	now := time.Now().Unix()
	if u.LockedUntil > now {
		s.auditAs(r.Context(), u.ID, u.Username, ip, "auth.login", "", false, "account locked")
		return &apiError{423, "locked", "account temporarily locked after repeated failures"}
	}
	if !auth.VerifyPassword(req.Password, u.PasswordHash) {
		failed := u.Failed + 1
		lock := int64(0)
		if failed >= maxFailedLogins {
			lock, failed = now+int64(lockDuration.Seconds()), 0
		}
		_, _ = s.db.Exec(r.Context(), `UPDATE users SET failed_logins = ?, locked_until = ? WHERE id = ?`, failed, lock, u.ID)
		failedAttempt()
		s.auditAs(r.Context(), u.ID, u.Username, ip, "auth.login", "", false, "bad password")
		return invalid
	}
	if u.Suspended {
		return &apiError{403, "suspended", "this account is suspended"}
	}
	// Checked only after the password, so it reveals nothing to a guesser.
	if !panelAccepts(panelFrom(r.Context()), u.Role) {
		s.auditAs(r.Context(), u.ID, u.Username, ip, "auth.login", "", false, "wrong panel")
		return wrongPanel(u.Role)
	}
	if u.Role != rbac.User && !s.whmAllowed(r.Context(), ip) {
		s.auditAs(r.Context(), u.ID, u.Username, ip, "auth.login", "", false, "not on WHM allow list")
		return &apiError{403, "access_denied", "WHM sign-in from your address is not allowed"}
	}
	_, _ = s.db.Exec(r.Context(), `UPDATE users SET failed_logins = 0, locked_until = 0 WHERE id = ?`, u.ID)

	if u.TOTPEnabled {
		challenge, err := s.newSession(r, u.ID, true)
		if err != nil {
			return err
		}
		writeJSON(w, 200, map[string]any{"twofa_required": true, "challenge": challenge})
		return nil
	}
	return s.finishLogin(w, r, u)
}

type login2FAReq struct {
	Challenge string `json:"challenge"`
	Code      string `json:"code"`
}

func (s *Server) login2FA(w http.ResponseWriter, r *http.Request) error {
	var req login2FAReq
	if err := decode(r, &req); err != nil {
		return err
	}
	ip := s.clientIP(r)
	if s.loginLimit.blocked("2fa:" + ip) {
		return errTooMany()
	}
	id := auth.HashToken(req.Challenge)
	var uid string
	var created int64
	err := s.db.QueryRow(r.Context(), `SELECT user_id, created_at FROM sessions WHERE id = ? AND pending_2fa = 1`, id).Scan(&uid, &created)
	if err != nil || time.Now().Unix()-created > int64(pendingTTL.Seconds()) {
		return &apiError{401, "invalid_challenge", "the sign-in attempt expired, start again"}
	}
	u, err := s.userByID(r.Context(), uid)
	if err != nil {
		return errUnauthorized()
	}
	ok := false
	code := strings.TrimSpace(req.Code)
	if step, valid := auth.VerifyTOTP(u.TOTPSecret, code, time.Now()); valid {
		if last, found := totpUsed.Load(u.ID); !found || last.(uint64) < step {
			totpUsed.Store(u.ID, step)
			ok = true
		}
	} else if s.consumeRecoveryCode(r.Context(), u, code) {
		ok = true
	}
	if !ok {
		s.loginLimit.fail("2fa:" + ip)
		s.auditAs(r.Context(), u.ID, u.Username, ip, "auth.2fa", "", false, "bad code")
		return &apiError{401, "invalid_code", "invalid verification code"}
	}
	_, _ = s.db.Exec(r.Context(), `DELETE FROM sessions WHERE id = ?`, id)
	return s.finishLogin(w, r, u)
}

func (s *Server) consumeRecoveryCode(ctx context.Context, u *userRow, code string) bool {
	if u.Recovery == "" || len(code) != 9 {
		return false
	}
	h := auth.HashToken(strings.ToLower(code))
	codes := strings.Split(u.Recovery, ",")
	for i, c := range codes {
		if subtle.ConstantTimeCompare([]byte(c), []byte(h)) == 1 {
			codes = append(codes[:i], codes[i+1:]...)
			_, _ = s.db.Exec(ctx, `UPDATE users SET recovery_codes = ? WHERE id = ?`, strings.Join(codes, ","), u.ID)
			return true
		}
	}
	return false
}

func (s *Server) finishLogin(w http.ResponseWriter, r *http.Request, u *userRow) error {
	token, err := s.newSession(r, u.ID, false)
	if err != nil {
		return err
	}
	var csrf string
	if err := s.db.QueryRow(r.Context(), `SELECT csrf_token FROM sessions WHERE id = ?`, auth.HashToken(token)).Scan(&csrf); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName(panelFrom(r.Context())), Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: s.isSecure(r), MaxAge: int(s.cfg.SessionTTL.Seconds()),
	})
	s.auditAs(r.Context(), u.ID, u.Username, s.clientIP(r), "auth.login", "", true, "")
	writeJSON(w, 200, map[string]any{"user": s.view(r.Context(), u), "csrf_token": csrf})
	return nil
}

func (s *Server) isSecure(r *http.Request) bool {
	return r.TLS != nil || (s.isTrustedProxy(strings.Split(r.RemoteAddr, ":")[0]) && r.Header.Get("X-Forwarded-Proto") == "https")
}

// newSession creates a session row and returns the opaque cookie token.
func (s *Server) newSession(r *http.Request, userID string, pending bool) (string, error) {
	token, hash, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	csrf, _, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()
	exp := now + int64(s.cfg.SessionTTL.Seconds())
	p := 0
	if pending {
		p, exp = 1, now+int64(pendingTTL.Seconds())
	}
	ua := r.UserAgent()
	if len(ua) > 200 {
		ua = ua[:200]
	}
	_, err = s.db.Exec(r.Context(),
		`INSERT INTO sessions (id, user_id, csrf_token, ip, user_agent, pending_2fa, created_at, last_seen, expires_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		hash, userID, csrf, s.clientIP(r), ua, p, now, now, exp)
	return token, err
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) error {
	if p := principalFrom(r.Context()); p != nil && !p.ViaToken {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get(csrfHeader)), []byte(p.CSRF)) != 1 {
			return &apiError{403, "csrf", "missing or invalid CSRF token"}
		}
		_, _ = s.db.Exec(r.Context(), `DELETE FROM sessions WHERE id = ?`, p.SessionID)
		s.audit(r, "auth.logout", "", true, "")
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName(panelFrom(r.Context())), Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	u, err := s.userByID(r.Context(), p.UserID)
	if err != nil {
		return err
	}
	writeJSON(w, 200, map[string]any{"user": s.view(r.Context(), u), "csrf_token": p.CSRF})
	return nil
}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Email  *string `json:"email"`
		Locale *string `json:"locale"`
		Theme  *string `json:"theme"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	p := principalFrom(r.Context())
	u, err := s.userByID(r.Context(), p.UserID)
	if err != nil {
		return err
	}
	if req.Email != nil {
		if !strings.Contains(*req.Email, "@") || len(*req.Email) > 254 {
			return errBadRequest("enter a valid email address")
		}
		u.Email = *req.Email
	}
	if req.Locale != nil {
		if len(*req.Locale) < 2 || len(*req.Locale) > 8 {
			return errBadRequest("invalid locale")
		}
		u.Locale = *req.Locale
	}
	if req.Theme != nil {
		if *req.Theme != "light" && *req.Theme != "dark" && *req.Theme != "system" {
			return errBadRequest("theme must be light, dark or system")
		}
		u.Theme = *req.Theme
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE users SET email=?, locale=?, theme=?, updated_at=? WHERE id=?`,
		u.Email, u.Locale, u.Theme, time.Now().Unix(), u.ID); err != nil {
		return err
	}
	writeJSON(w, 200, s.view(r.Context(), u))
	return nil
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	p := principalFrom(r.Context())
	u, err := s.userByID(r.Context(), p.UserID)
	if err != nil {
		return err
	}
	if !auth.VerifyPassword(req.Current, u.PasswordHash) {
		s.audit(r, "auth.password", u.Username, false, "wrong current password")
		return &apiError{403, "invalid_credentials", "current password is incorrect"}
	}
	if err := auth.CheckPasswordPolicy(req.New); err != nil {
		return errBadRequest(err.Error())
	}
	hash, err := auth.HashPassword(req.New)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE users SET password_hash=?, updated_at=? WHERE id=?`, hash, time.Now().Unix(), u.ID); err != nil {
		return err
	}
	// Invalidate every other session.
	_, _ = s.db.Exec(r.Context(), `DELETE FROM sessions WHERE user_id = ? AND id <> ?`, u.ID, p.SessionID)
	s.audit(r, "auth.password", u.Username, true, "")
	s.securityNotice(r.Context(), u.ID, "password_changed", "The password of "+u.Username+" was changed", "The password of the hosting login "+u.Username+" was just changed.")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- two-factor ------------------------------------------------------------

func (s *Server) totpSetup(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	u, err := s.userByID(r.Context(), p.UserID)
	if err != nil {
		return err
	}
	if u.TOTPEnabled {
		return errConflict("two-factor authentication is already enabled")
	}
	secret, err := auth.NewTOTPSecret()
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE users SET totp_secret = ? WHERE id = ?`, secret, u.ID); err != nil {
		return err
	}
	writeJSON(w, 200, map[string]string{"secret": secret, "uri": auth.TOTPURI("BreeferCP", u.Username, secret)})
	return nil
}

func (s *Server) totpEnable(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Code string `json:"code"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	p := principalFrom(r.Context())
	u, err := s.userByID(r.Context(), p.UserID)
	if err != nil {
		return err
	}
	if u.TOTPSecret == "" || u.TOTPEnabled {
		return errBadRequest("start two-factor setup first")
	}
	step, ok := auth.VerifyTOTP(u.TOTPSecret, req.Code, time.Now())
	if !ok {
		return &apiError{400, "invalid_code", "invalid verification code"}
	}
	totpUsed.Store(u.ID, step)
	codes, err := auth.NewRecoveryCodes(8)
	if err != nil {
		return err
	}
	hashes := make([]string, len(codes))
	for i, c := range codes {
		hashes[i] = auth.HashToken(c)
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE users SET totp_enabled = 1, recovery_codes = ? WHERE id = ?`,
		strings.Join(hashes, ","), u.ID); err != nil {
		return err
	}
	s.audit(r, "auth.2fa.enable", u.Username, true, "")
	writeJSON(w, 200, map[string]any{"recovery_codes": codes})
	return nil
}

func (s *Server) totpDisable(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	p := principalFrom(r.Context())
	u, err := s.userByID(r.Context(), p.UserID)
	if err != nil {
		return err
	}
	if !auth.VerifyPassword(req.Password, u.PasswordHash) {
		return &apiError{403, "invalid_credentials", "password is incorrect"}
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE users SET totp_enabled = 0, totp_secret = '', recovery_codes = '' WHERE id = ?`, u.ID); err != nil {
		return err
	}
	s.audit(r, "auth.2fa.disable", u.Username, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- sessions & API tokens -------------------------------------------------

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	rows, err := s.db.Query(r.Context(),
		`SELECT id, ip, user_agent, created_at, last_seen FROM sessions WHERE user_id = ? AND pending_2fa = 0 AND expires_at > ? ORDER BY last_seen DESC`,
		p.UserID, time.Now().Unix())
	if err != nil {
		return err
	}
	defer rows.Close()
	type sv struct {
		ID        string `json:"id"`
		IP        string `json:"ip"`
		UserAgent string `json:"user_agent"`
		CreatedAt int64  `json:"created_at"`
		LastSeen  int64  `json:"last_seen"`
		Current   bool   `json:"current"`
	}
	out := []sv{}
	for rows.Next() {
		var v sv
		if err := rows.Scan(&v.ID, &v.IP, &v.UserAgent, &v.CreatedAt, &v.LastSeen); err != nil {
			return err
		}
		v.Current = v.ID == p.SessionID
		out = append(out, v)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	if _, err := s.db.Exec(r.Context(), `DELETE FROM sessions WHERE id = ? AND user_id = ?`, r.PathValue("id"), p.UserID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	rows, err := s.db.Query(r.Context(),
		`SELECT id, name, scopes, allowed_ips, last_used, expires_at, created_at FROM api_tokens WHERE user_id = ? ORDER BY created_at DESC`, p.UserID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type tv struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Scopes     string `json:"scopes"`
		AllowedIPs string `json:"allowed_ips"`
		LastUsed   int64  `json:"last_used"`
		ExpiresAt  int64  `json:"expires_at"`
		CreatedAt  int64  `json:"created_at"`
	}
	out := []tv{}
	for rows.Next() {
		var v tv
		if err := rows.Scan(&v.ID, &v.Name, &v.Scopes, &v.AllowedIPs, &v.LastUsed, &v.ExpiresAt, &v.CreatedAt); err != nil {
			return err
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name       string   `json:"name"`
		Scopes     []string `json:"scopes"`
		AllowedIPs string   `json:"allowed_ips"`
		ExpiresIn  int      `json:"expires_in_days"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 80 {
		return errBadRequest("token name is required")
	}
	if len(req.Scopes) == 0 {
		return errBadRequest("select at least one scope")
	}
	for _, sc := range req.Scopes {
		if strings.ContainsAny(sc, " ,") || len(sc) > 40 {
			return errBadRequest("invalid scope")
		}
	}
	p := principalFrom(r.Context())
	if p.ViaToken {
		return errForbidden() // tokens cannot mint tokens
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		return err
	}
	var exp int64
	if req.ExpiresIn > 0 {
		exp = time.Now().AddDate(0, 0, req.ExpiresIn).Unix()
	}
	id := ids.New()
	if _, err := s.db.Exec(r.Context(),
		`INSERT INTO api_tokens (id, user_id, name, token_hash, scopes, allowed_ips, expires_at, created_at) VALUES (?,?,?,?,?,?,?,?)`,
		id, p.UserID, req.Name, hash, strings.Join(req.Scopes, " "), req.AllowedIPs, exp, time.Now().Unix()); err != nil {
		return err
	}
	s.audit(r, "token.create", req.Name, true, "")
	writeJSON(w, 201, map[string]any{"id": id, "token": "bcp_" + token})
	return nil
}

func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	if _, err := s.db.Exec(r.Context(), `DELETE FROM api_tokens WHERE id = ? AND user_id = ?`, r.PathValue("id"), p.UserID); err != nil {
		return err
	}
	s.audit(r, "token.delete", r.PathValue("id"), true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}
