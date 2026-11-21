package api

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/breefercp/breefercp/internal/rbac"
)

// Panel identifies which of the two dedicated panels a listener serves.
// WHM is for administrators and resellers, cPanel for account holders.
// They run on separate ports with separate session cookies, so a browser
// can hold an administrator session and a customer session at once and
// neither can be used on the other panel.
type Panel string

const (
	PanelWHM    Panel = "whm"
	PanelCPanel Panel = "cpanel"
	// PanelBoth serves both from one listener (cpanel_listen empty).
	PanelBoth Panel = "both"
)

const panelKey ctxKey = 2

func panelFrom(ctx context.Context) Panel {
	if p, ok := ctx.Value(panelKey).(Panel); ok {
		return p
	}
	return PanelBoth
}

// cookieName keeps the two panels' sessions apart: cookies are scoped by
// host, not port, so the same name would be shared by :2083 and :2087.
func cookieName(p Panel) string {
	switch p {
	case PanelWHM:
		return "breefer_whm"
	case PanelCPanel:
		return "breefer_cpanel"
	}
	return sessionCookie
}

// panelAccepts reports whether a user of role may use panel p.
func panelAccepts(p Panel, role rbac.Role) bool {
	switch p {
	case PanelWHM:
		return role == rbac.Root || role == rbac.Reseller
	case PanelCPanel:
		return role == rbac.User
	}
	return true
}

// panelFor is the panel a role signs in to.
func panelFor(role rbac.Role) Panel {
	if role == rbac.User {
		return PanelCPanel
	}
	return PanelWHM
}

// panelURL builds the absolute base URL of panel p as seen by the caller:
// same scheme and host name as the current request, with that panel's port.
func (s *Server) panelURL(r *http.Request, p Panel) string {
	if s.cfg.CpanelListen == "" || p == PanelBoth {
		return ""
	}
	addr := s.cfg.Listen
	if p == PanelCPanel {
		addr = s.cfg.CpanelListen
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if strings.Contains(host, ":") { // IPv6 literal
		host = "[" + host + "]"
	}
	scheme := "http"
	if s.isSecure(r) || s.cfg.TLSCert != "" {
		scheme = "https"
	}
	return scheme + "://" + host + ":" + port
}

func withPanel(p Panel, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), panelKey, p)))
	})
}

// wrongPanel tells a user who signed in on the other panel where to go.
func wrongPanel(role rbac.Role) error {
	target := panelFor(role)
	msg := "administrators and resellers sign in to WHM"
	if target == PanelCPanel {
		msg = "account holders sign in to cPanel"
	}
	return &apiError{403, "wrong_panel", msg}
}

// panelInfo is public: the UI calls it before sign-in to pick its layout.
func (s *Server) panelInfo(w http.ResponseWriter, r *http.Request) error {
	p := panelFrom(r.Context())
	hostname := s.setting(r.Context(), "hostname")
	out := map[string]any{
		"panel":       p,
		"whm_url":     s.panelURL(r, PanelWHM),
		"cpanel_url":  s.panelURL(r, PanelCPanel),
		"hostname":    hostname,
		"webmail_url": s.toolsURL(r.Context()) + "/webmail/",
	}
	if p != PanelWHM {
		out["announcement"] = s.setting(r.Context(), "announcement")
	}
	writeJSON(w, 200, out)
	return nil
}
