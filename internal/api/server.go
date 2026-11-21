// Package api implements the BreeferCP HTTP API and serves the web UI.
package api

import (
	"context"
	"database/sql"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/config"
	"github.com/breefercp/breefercp/internal/provision"
	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/store"
)

// Server wires the HTTP surface to the store and provisioner.
type Server struct {
	cfg     config.Config
	db      *store.Store
	prov    *provision.Provisioner
	mariadb *provision.MariaDB
	log     *slog.Logger
	trusted []*net.IPNet

	loginLimit *rateLimiter
	apiLimit   *rateLimiter
	started    time.Time
	metrics    *metricsRing
	cgroups    *cgroupSampler
	version    string
}

// New builds the server. mdb may be nil in dry-run mode.
func New(cfg config.Config, db *store.Store, prov *provision.Provisioner, mdb *sql.DB, log *slog.Logger, version string) *Server {
	s := &Server{
		cfg: cfg, db: db, prov: prov, log: log, version: version,
		mariadb:    provision.NewMariaDB(mdb, cfg.DryRun),
		loginLimit: newRateLimiter(10, 5*time.Minute),
		apiLimit:   newRateLimiter(600, time.Minute),
		started:    time.Now(),
		metrics:    &metricsRing{},
		cgroups:    newCgroupSampler(),
	}
	for _, c := range cfg.TrustedProxies {
		if _, n, err := net.ParseCIDR(strings.TrimSpace(c)); err == nil {
			s.trusted = append(s.trusted, n)
		}
	}
	return s
}

// Handler returns the fully wrapped root handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.routes(mux)
	mux.Handle("/", s.spa())
	var h http.Handler = mux
	h = s.authenticate(h)
	h = s.limitAPI(h)
	h = securityHeaders(h)
	h = s.recoverer(h)
	return h
}

func (s *Server) limitAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && !s.apiLimit.allow(s.clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, 429, map[string]any{"error": map[string]string{"code": "rate_limited", "message": "too many requests"}})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// P is shorthand for building a permission-guarded route.
func (s *Server) route(mux *http.ServeMux, pattern string, perm rbac.Permission, h handler) {
	mux.HandleFunc(pattern, s.require(perm, h))
}

func (s *Server) routes(mux *http.ServeMux) {
	// Public.
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok", "version": s.version})
	})
	mux.HandleFunc("POST /api/v1/auth/login", s.wrap(s.login))
	mux.HandleFunc("POST /api/v1/auth/login/2fa", s.wrap(s.login2FA))
	mux.HandleFunc("POST /api/v1/auth/sso", s.wrap(s.ssoRedeem))

	// Session & profile.
	mux.HandleFunc("POST /api/v1/auth/logout", s.wrap(s.logout))
	s.route(mux, "GET /api/v1/me", rbac.ProfileWrite, s.me)
	s.route(mux, "PATCH /api/v1/me", rbac.ProfileWrite, s.updateMe)
	s.route(mux, "POST /api/v1/me/password", rbac.ProfileWrite, s.changePassword)
	s.route(mux, "POST /api/v1/me/2fa/setup", rbac.ProfileWrite, s.totpSetup)
	s.route(mux, "POST /api/v1/me/2fa/enable", rbac.ProfileWrite, s.totpEnable)
	s.route(mux, "POST /api/v1/me/2fa/disable", rbac.ProfileWrite, s.totpDisable)
	s.route(mux, "GET /api/v1/me/sessions", rbac.ProfileWrite, s.listSessions)
	s.route(mux, "DELETE /api/v1/me/sessions/{id}", rbac.ProfileWrite, s.revokeSession)
	s.route(mux, "GET /api/v1/me/tokens", rbac.TokensWrite, s.listTokens)
	s.route(mux, "POST /api/v1/me/tokens", rbac.TokensWrite, s.createToken)
	s.route(mux, "DELETE /api/v1/me/tokens/{id}", rbac.TokensWrite, s.deleteToken)

	// WHM: server administration.
	s.route(mux, "GET /api/v1/server/status", rbac.ServerRead, s.serverStatus)
	s.route(mux, "GET /api/v1/server/services", rbac.ServerRead, s.listServices)
	s.route(mux, "POST /api/v1/server/services/{name}/{action}", rbac.ServerWrite, s.serviceAction)
	s.route(mux, "GET /api/v1/server/firewall", rbac.FirewallRead, s.listFirewall)
	s.route(mux, "POST /api/v1/server/firewall", rbac.FirewallWrite, s.addFirewall)
	s.route(mux, "DELETE /api/v1/server/firewall/{id}", rbac.FirewallWrite, s.deleteFirewall)
	s.route(mux, "GET /api/v1/server/audit", rbac.AuditRead, s.listAudit)
	s.route(mux, "GET /api/v1/server/settings", rbac.ServerRead, s.getSettings)
	s.route(mux, "PUT /api/v1/server/settings", rbac.SettingsWrite, s.putSettings)

	// WHM: customers.
	s.route(mux, "GET /api/v1/packages", rbac.PackagesRead, s.listPackages)
	s.route(mux, "POST /api/v1/packages", rbac.PackagesWrite, s.createPackage)
	s.route(mux, "PUT /api/v1/packages/{id}", rbac.PackagesWrite, s.updatePackage)
	s.route(mux, "DELETE /api/v1/packages/{id}", rbac.PackagesWrite, s.deletePackage)
	s.route(mux, "GET /api/v1/resellers", rbac.ResellersWrite, s.listResellers)
	s.route(mux, "POST /api/v1/resellers", rbac.ResellersWrite, s.createReseller)
	s.route(mux, "PATCH /api/v1/resellers/{id}", rbac.ResellersWrite, s.updateReseller)
	s.route(mux, "DELETE /api/v1/resellers/{id}", rbac.ResellersWrite, s.deleteReseller)
	s.route(mux, "GET /api/v1/accounts", rbac.AccountsRead, s.listAccounts)
	s.route(mux, "POST /api/v1/accounts", rbac.AccountsWrite, s.createAccount)
	s.route(mux, "GET /api/v1/accounts/{aid}", rbac.HostingRead, s.getAccount)
	s.route(mux, "PATCH /api/v1/accounts/{aid}", rbac.AccountsWrite, s.updateAccount)
	s.route(mux, "POST /api/v1/accounts/{aid}/suspend", rbac.AccountsWrite, s.suspendAccount)
	s.route(mux, "POST /api/v1/accounts/{aid}/unsuspend", rbac.AccountsWrite, s.unsuspendAccount)
	s.route(mux, "DELETE /api/v1/accounts/{aid}", rbac.AccountsWrite, s.terminateAccount)
	s.route(mux, "POST /api/v1/accounts/{aid}/sso", rbac.AccountsWrite, s.createSSO)

	// cPanel: hosting features, scoped to an account ("me" for account holders).
	s.hostingRoutes(mux)

	// File manager.
	s.fileRoutes(mux)

	// Billing integrations (WHMCS, Paymenter, generic).
	s.billingRoutes(mux)

	s.extraRoutes(mux)
	s.ticketRoutes(mux)
	s.installerRoutes(mux)
	s.toolRoutes(mux)
	s.terminalRoutes(mux)
	s.metricsRoutes(mux)
}

// spa serves the built React app, falling back to index.html for client routes.
func (s *Server) spa() http.Handler {
	root := s.cfg.WebDir
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, 404, map[string]any{"error": map[string]string{"code": "not_found", "message": "unknown endpoint"}})
			return
		}
		if _, err := os.Stat(root); err != nil {
			http.Error(w, "BreeferCP web UI is not installed (web_dir missing)", http.StatusServiceUnavailable)
			return
		}
		clean := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		full := filepath.Join(root, clean)
		if info, err := os.Stat(full); err != nil || info.IsDir() {
			w.Header().Set("Cache-Control", "no-store")
			http.ServeFile(w, r, filepath.Join(root, "index.html"))
			return
		}
		if strings.HasPrefix(clean, "assets") { // content-hashed by the bundler
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeFile(w, r, full)
	})
}

// ipAllowed reports whether ip falls inside any entry of a comma/space
// separated allow list of IPs and CIDRs.
func ipAllowed(list, ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, e := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == ' ' }) {
		if _, n, err := net.ParseCIDR(e); err == nil {
			if n.Contains(parsed) {
				return true
			}
		} else if net.ParseIP(e) != nil && net.ParseIP(e).Equal(parsed) {
			return true
		}
	}
	return false
}

// Shutdown is a hook for graceful cleanup.
func (s *Server) Shutdown(_ context.Context) error { return nil }

var _ fs.FS // keep io/fs import for future embedded UI support
