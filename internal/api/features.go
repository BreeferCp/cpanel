package api

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/rbac"
)

// Feature is one cPanel tool a feature list can switch off.
type Feature struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Group string `json:"group"`
}

// Features is the catalog shown in WHM's Feature Manager. The UI hides
// disabled tools; featureGate enforces the same list on the API.
var Features = []Feature{
	{"file_manager", "File Manager", "Files"},
	{"disk_usage", "Disk Usage", "Files"},
	{"backups", "Backups", "Files"},
	{"ftp", "FTP Accounts", "Files"},
	{"git", "Git Version Control", "Files"},
	{"domains", "Domains", "Domains"},
	{"redirects", "Redirects", "Domains"},
	{"dns", "Zone Editor and DNSSEC", "Domains"},
	{"ssl", "SSL/TLS", "Security"},
	{"directory_privacy", "Directory Privacy", "Security"},
	{"ip_blocker", "IP Blocker", "Security"},
	{"hotlink", "Hotlink Protection", "Security"},
	{"email", "Email Accounts and Forwarders", "Email"},
	{"autoresponders", "Autoresponders", "Email"},
	{"spam_filter", "Spam Filters", "Email"},
	{"forwarders", "Forwarders and Domain Forwarders", "Email"},
	{"email_routing", "Email Routing", "Email"},
	{"default_address", "Default Address", "Email"},
	{"mailing_lists", "Mailing Lists", "Email"},
	{"email_filters", "Email Filters", "Email"},
	{"track_delivery", "Track Delivery", "Email"},
	{"deliverability", "Email Deliverability", "Email"},
	{"databases", "MySQL Databases", "Databases"},
	{"phpmyadmin", "phpMyAdmin", "Databases"},
	{"remote_mysql", "Remote MySQL", "Databases"},
	{"installer", "App Installer", "Software"},
	{"wordpress", "WordPress Manager", "Software"},
	{"php", "PHP Manager", "Software"},
	{"apps", "Node.js, Python and Ruby Apps", "Software"},
	{"error_pages", "Error Pages", "Advanced"},
	{"cron", "Cron Jobs", "Advanced"},
	{"ssh", "SSH Access", "Advanced"},
	{"terminal", "Terminal", "Advanced"},
	{"logs", "Errors and Raw Access", "Metrics"},
	{"visitors", "Visitors", "Metrics"},
	{"bandwidth", "Bandwidth", "Metrics"},
	{"dynamic_dns", "Dynamic DNS", "Domains"},
	{"indexes", "Indexes", "Advanced"},
	{"mime_types", "MIME Types", "Advanced"},
	{"track_dns", "Track DNS", "Advanced"},
	{"optimize", "Optimize Website", "Software"},
}

func validFeature(k string) bool {
	for _, f := range Features {
		if f.Key == k {
			return true
		}
	}
	return false
}

// featureFor maps the path below /api/v1/accounts/{aid}/ to a feature key.
func featureFor(segs []string) string {
	if len(segs) == 0 {
		return ""
	}
	switch segs[0] {
	case "files":
		return "file_manager"
	case "disk-usage":
		return "disk_usage"
	case "backups":
		return "backups"
	case "ftp":
		return "ftp"
	case "git":
		return "git"
	case "domains":
		if len(segs) >= 3 {
			switch segs[2] {
			case "redirects":
				return "redirects"
			case "error-pages":
				return "error_pages"
			case "ssl":
				return "ssl"
			}
		}
		return "domains"
	case "ssl":
		return "ssl"
	case "dns", "dnssec":
		return "dns"
	case "protected-dirs":
		return "directory_privacy"
	case "ip-blocks":
		return "ip_blocker"
	case "hotlink":
		return "hotlink"
	case "mailboxes", "mail", "mail-usage":
		return "email"
	case "mail-aliases", "domain-forwards":
		return "forwarders"
	case "email-routing":
		return "email_routing"
	case "default-address":
		return "default_address"
	case "mailing-lists":
		return "mailing_lists"
	case "mail-filters":
		return "email_filters"
	case "mail-delivery":
		return "track_delivery"
	case "email-deliverability":
		return "deliverability"
	case "visitors":
		return "visitors"
	case "bandwidth":
		return "bandwidth"
	case "ddns":
		return "dynamic_dns"
	case "indexes":
		return "indexes"
	case "mime-types":
		return "mime_types"
	case "dns-lookup", "traceroute":
		return "track_dns"
	case "optimize":
		return "optimize"
	case "autoresponders":
		return "autoresponders"
	case "spam":
		return "spam_filter"
	case "databases", "db-users":
		return "databases"
	case "phpmyadmin":
		return "phpmyadmin"
	case "remote-mysql":
		return "remote_mysql"
	case "installations":
		if len(segs) >= 3 {
			switch segs[2] {
			case "wordpress", "clone", "push", "autoupdate", "update-now":
				return "wordpress"
			}
		}
		return "installer"
	case "wordpress":
		return "wordpress"
	case "php":
		return "php"
	case "apps":
		return "apps"
	case "cron", "cron-email":
		return "cron"
	case "ssh-keys":
		return "ssh"
	case "terminal":
		return "terminal"
	case "logs":
		return "logs"
	}
	return ""
}

// disabledFeatures returns the switched-off features of a user's account.
func (s *Server) disabledFeatures(ctx context.Context, userID string) map[string]bool {
	var list string
	_ = s.db.QueryRow(ctx, `SELECT fl.disabled FROM accounts a JOIN packages k ON k.id = a.package_id
		JOIN feature_lists fl ON fl.id = k.feature_list_id WHERE a.user_id = ?`, userID).Scan(&list)
	out := map[string]bool{}
	for _, k := range strings.Fields(list) {
		out[k] = true
	}
	return out
}

var nginxOnly = map[string]bool{"protected-dirs": true, "ip-blocks": true, "hotlink": true, "indexes": true, "mime-types": true, "optimize": true}

// featureGate refuses account holders the tools their package's feature
// list switches off. Administrators acting on an account are not limited.
func (s *Server) featureGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := principalFrom(r.Context())
		rest, ok := strings.CutPrefix(r.URL.Path, "/api/v1/accounts/")
		if ok && s.cfg.WebServer == "openlitespeed" && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
			// The OpenLiteSpeed renderer ignores these. Say so rather than
			// report a folder as protected when it is not.
			if segs := strings.Split(rest, "/"); len(segs) > 1 && nginxOnly[segs[1]] {
				writeJSON(w, 400, map[string]any{"error": map[string]string{"code": "unsupported",
					"message": "this tool is not supported with OpenLiteSpeed yet"}})
				return
			}
		}
		if ok && p != nil && p.Role == rbac.User {
			segs := strings.Split(rest, "/")
			if f := featureFor(segs[1:]); f != "" && s.disabledFeatures(r.Context(), p.UserID)[f] {
				writeJSON(w, 403, map[string]any{"error": map[string]string{"code": "feature_disabled",
					"message": "this feature is not included in your hosting plan"}})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ---- feature lists (WHM Feature Manager) ------------------------------------

type featureList struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Disabled []string `json:"disabled"`
	Packages int      `json:"packages"`
}

func (s *Server) featureRoutes(mux *http.ServeMux) {
	s.route(mux, "GET /api/v1/features", rbac.HostingRead, func(w http.ResponseWriter, _ *http.Request) error {
		writeJSON(w, 200, Features)
		return nil
	})
	s.route(mux, "GET /api/v1/feature-lists", rbac.PackagesRead, s.listFeatureLists)
	s.route(mux, "POST /api/v1/feature-lists", rbac.PackagesWrite, s.saveFeatureList)
	s.route(mux, "PUT /api/v1/feature-lists/{id}", rbac.PackagesWrite, s.saveFeatureList)
	s.route(mux, "DELETE /api/v1/feature-lists/{id}", rbac.PackagesWrite, s.deleteFeatureList)
}

func (s *Server) listFeatureLists(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	q := `SELECT f.id, f.name, f.disabled, (SELECT COUNT(*) FROM packages k WHERE k.feature_list_id = f.id) FROM feature_lists f`
	var args []any
	if p.Role != rbac.Root {
		q += ` WHERE f.owner_id = ?`
		args = append(args, p.UserID)
	}
	rows, err := s.db.Query(r.Context(), q+` ORDER BY f.name`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []featureList{}
	for rows.Next() {
		var f featureList
		var dis string
		if err := rows.Scan(&f.ID, &f.Name, &dis, &f.Packages); err != nil {
			return err
		}
		f.Disabled = strings.Fields(dis)
		out = append(out, f)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

// ownedFeatureList checks that the caller may use or edit list id.
func (s *Server) ownedFeatureList(ctx context.Context, p *Principal, id string) error {
	q := `SELECT COUNT(*) FROM feature_lists WHERE id = ?`
	args := []any{id}
	if p.Role != rbac.Root {
		q += ` AND owner_id = ?`
		args = append(args, p.UserID)
	}
	var n int
	if err := s.db.QueryRow(ctx, q, args...).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return errNotFound()
	}
	return nil
}

func (s *Server) saveFeatureList(w http.ResponseWriter, r *http.Request) error {
	var req featureList
	if err := decode(r, &req); err != nil {
		return err
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 64 {
		return errBadRequest("the feature list needs a name (max 64 characters)")
	}
	seen := map[string]bool{}
	clean := []string{}
	for _, k := range req.Disabled {
		if !validFeature(k) {
			return errBadRequest("unknown feature " + k)
		}
		if !seen[k] {
			seen[k] = true
			clean = append(clean, k)
		}
	}
	sort.Strings(clean)
	p := principalFrom(r.Context())
	if id := r.PathValue("id"); id != "" {
		if err := s.ownedFeatureList(r.Context(), p, id); err != nil {
			return err
		}
		if _, err := s.db.Exec(r.Context(), `UPDATE feature_lists SET name = ?, disabled = ? WHERE id = ?`,
			req.Name, strings.Join(clean, " "), id); err != nil {
			return err
		}
		req.ID = id
	} else {
		req.ID = ids.New()
		if _, err := s.db.Exec(r.Context(), `INSERT INTO feature_lists (id, owner_id, name, disabled, created_at) VALUES (?,?,?,?,?)`,
			req.ID, p.UserID, req.Name, strings.Join(clean, " "), time.Now().Unix()); err != nil {
			return err
		}
	}
	req.Disabled = clean
	s.audit(r, "featurelist.save", req.Name, true, strings.Join(clean, " "))
	writeJSON(w, 200, req)
	return nil
}

func (s *Server) deleteFeatureList(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	id := r.PathValue("id")
	if err := s.ownedFeatureList(r.Context(), p, id); err != nil {
		return err
	}
	var n int
	if err := s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM packages WHERE feature_list_id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return errConflict("packages still use this feature list")
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM feature_lists WHERE id = ?`, id); err != nil {
		return err
	}
	s.audit(r, "featurelist.delete", id, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}
