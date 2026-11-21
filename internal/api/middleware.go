package api

import (
	"context"
	"crypto/subtle"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/breefercp/breefercp/internal/auth"
	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/store"
)

const (
	sessionCookie = "breefer_session"
	csrfHeader    = "X-CSRF-Token"
)

// Principal is the authenticated caller of a request.
type Principal struct {
	UserID    string
	Username  string
	Role      rbac.Role
	OwnerID   string
	SessionID string
	CSRF      string
	// Scopes is non-nil only for API-token callers.
	Scopes   []string
	ViaToken bool
}

type ctxKey int

const principalKey ctxKey = 1

func principalFrom(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey).(*Principal)
	return p
}

// securityHeaders sets defensive headers on every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// recoverer turns panics into 500s without crashing the daemon.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.log.Error("panic", "path", r.URL.Path, "panic", v, "stack", string(debug.Stack()))
				writeJSON(w, 500, map[string]any{"error": map[string]string{"code": "internal", "message": "internal error"}})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// authenticate resolves a session cookie or bearer token into a Principal.
// Unauthenticated requests continue without one; routes opt in via require.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var p *Principal
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			p = s.principalFromToken(ctx, strings.TrimPrefix(h, "Bearer "), s.clientIP(r))
		} else if c, err := r.Cookie(sessionCookie); err == nil {
			p = s.principalFromSession(ctx, c.Value)
		}
		if p != nil {
			ctx = context.WithValue(ctx, principalKey, p)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) principalFromSession(ctx context.Context, token string) *Principal {
	id := auth.HashToken(token)
	now := time.Now().Unix()
	var p Principal
	var pending, suspended int
	var role string
	var owner *string
	err := s.db.QueryRow(ctx, `
		SELECT u.id, u.username, u.role, u.owner_id, u.suspended, s.id, s.csrf_token, s.pending_2fa
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id = ? AND s.expires_at > ?`, id, now).
		Scan(&p.UserID, &p.Username, &role, &owner, &suspended, &p.SessionID, &p.CSRF, &pending)
	if err != nil || suspended == 1 || pending == 1 {
		return nil
	}
	p.Role = rbac.Role(role)
	if owner != nil {
		p.OwnerID = *owner
	}
	// Sliding expiry, written at most once a minute to spare the database.
	_, _ = s.db.Exec(ctx, `UPDATE sessions SET last_seen = ?, expires_at = ? WHERE id = ? AND last_seen < ?`,
		now, now+int64(s.cfg.SessionTTL.Seconds()), id, now-60)
	return &p
}

func (s *Server) principalFromToken(ctx context.Context, token, ip string) *Principal {
	hash := auth.HashToken(strings.TrimPrefix(token, "bcp_"))
	now := time.Now().Unix()
	var p Principal
	var role, scopes, allowed string
	var owner *string
	var suspended int
	var expires int64
	err := s.db.QueryRow(ctx, `
		SELECT u.id, u.username, u.role, u.owner_id, u.suspended, t.scopes, t.allowed_ips, t.expires_at
		FROM api_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = ?`, hash).Scan(&p.UserID, &p.Username, &role, &owner, &suspended, &scopes, &allowed, &expires)
	if err != nil || suspended == 1 || (expires != 0 && expires < now) {
		return nil
	}
	if allowed != "" && !ipAllowed(allowed, ip) {
		return nil
	}
	p.Role = rbac.Role(role)
	if owner != nil {
		p.OwnerID = *owner
	}
	p.Scopes = strings.Fields(scopes)
	p.ViaToken = true
	_, _ = s.db.Exec(ctx, `UPDATE api_tokens SET last_used = ? WHERE token_hash = ?`, now, hash)
	return &p
}

// require rejects requests without a principal or without the permission.
// Cookie sessions must also present a matching CSRF token on unsafe methods.
func (s *Server) require(perm rbac.Permission, h handler) http.HandlerFunc {
	return s.wrap(func(w http.ResponseWriter, r *http.Request) error {
		p := principalFrom(r.Context())
		if p == nil {
			return errUnauthorized()
		}
		if !p.ViaToken && isUnsafe(r.Method) {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get(csrfHeader)), []byte(p.CSRF)) != 1 {
				return &apiError{403, "csrf", "missing or invalid CSRF token"}
			}
		}
		if p.ViaToken && !scopeAllows(p.Scopes, perm) {
			return errForbidden()
		}
		if !rbac.Can(p.Role, perm) {
			return errForbidden()
		}
		return h(w, r)
	})
}

func isUnsafe(m string) bool {
	return m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions
}

// scopeAllows maps token scopes to permissions. A scope is either "*", a
// resource prefix ("accounts") or a full permission ("accounts:write").
func scopeAllows(scopes []string, perm rbac.Permission) bool {
	res, _, _ := strings.Cut(string(perm), ":")
	for _, sc := range scopes {
		if sc == "*" || sc == string(perm) || sc == res {
			return true
		}
	}
	return false
}

// ---- rate limiting --------------------------------------------------------

type bucket struct {
	count int
	reset time.Time
}

// rateLimiter is a fixed-window limiter keyed by an arbitrary string.
type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]*bucket
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{limit: limit, window: window, buckets: map[string]*bucket{}}
	go func() {
		for range time.Tick(window) {
			rl.mu.Lock()
			now := time.Now()
			for k, b := range rl.buckets {
				if now.After(b.reset) {
					delete(rl.buckets, k)
				}
			}
			rl.mu.Unlock()
		}
	}()
	return rl
}

// allow records one attempt and reports whether it is within the limit.
func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok || now.After(b.reset) {
		b = &bucket{reset: now.Add(rl.window)}
		rl.buckets[key] = b
	}
	b.count++
	return b.count <= rl.limit
}

// ensure store is referenced even when only used via Server.
var _ = store.ErrNotFound
