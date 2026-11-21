package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/provision"
	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/store"
	"github.com/breefercp/breefercp/internal/system"
)

func (s *Server) hostingRoutes(mux *http.ServeMux) {
	const a = "/api/v1/accounts/{aid}"
	r, w := rbac.HostingRead, rbac.HostingWrite

	s.route(mux, "GET "+a+"/usage", r, s.usage)

	s.route(mux, "GET "+a+"/domains", r, s.listDomains)
	s.route(mux, "POST "+a+"/domains", w, s.addDomain)
	s.route(mux, "PATCH "+a+"/domains/{id}", w, s.updateDomain)
	s.route(mux, "DELETE "+a+"/domains/{id}", w, s.deleteDomain)

	s.route(mux, "GET "+a+"/ssl", r, s.listCerts)
	s.route(mux, "POST "+a+"/domains/{id}/ssl/letsencrypt", w, s.issueLE)
	s.route(mux, "POST "+a+"/domains/{id}/ssl/custom", w, s.installCustomCert)
	s.route(mux, "DELETE "+a+"/domains/{id}/ssl", w, s.removeCert)

	s.route(mux, "GET "+a+"/dns/{zone}", r, s.listDNS)
	s.route(mux, "POST "+a+"/dns/{zone}", w, s.upsertDNS)
	s.route(mux, "DELETE "+a+"/dns/{zone}", w, s.deleteDNS)

	s.route(mux, "GET "+a+"/cron", r, s.listCron)
	s.route(mux, "POST "+a+"/cron", w, s.addCron)
	s.route(mux, "PATCH "+a+"/cron/{id}", w, s.toggleCron)
	s.route(mux, "DELETE "+a+"/cron/{id}", w, s.deleteCron)

	s.route(mux, "GET "+a+"/ssh-keys", r, s.listSSHKeys)
	s.route(mux, "POST "+a+"/ssh-keys", w, s.addSSHKey)
	s.route(mux, "DELETE "+a+"/ssh-keys/{id}", w, s.deleteSSHKey)

	s.route(mux, "GET "+a+"/databases", r, s.listDatabases)
	s.route(mux, "POST "+a+"/databases", w, s.createDatabase)
	s.route(mux, "DELETE "+a+"/databases/{id}", w, s.deleteDatabase)
	s.route(mux, "GET "+a+"/db-users", r, s.listDBUsers)
	s.route(mux, "POST "+a+"/db-users", w, s.createDBUser)
	s.route(mux, "PUT "+a+"/db-users/{id}/password", w, s.setDBUserPassword)
	s.route(mux, "DELETE "+a+"/db-users/{id}", w, s.deleteDBUser)
	s.route(mux, "PUT "+a+"/databases/{id}/grants/{uid}", w, s.grantDB)
	s.route(mux, "DELETE "+a+"/databases/{id}/grants/{uid}", w, s.revokeDB)

	s.route(mux, "GET "+a+"/mailboxes", r, s.listMailboxes)
	s.route(mux, "POST "+a+"/mailboxes", w, s.createMailbox)
	s.route(mux, "PUT "+a+"/mailboxes/{id}/password", w, s.setMailboxPassword)
	s.route(mux, "DELETE "+a+"/mailboxes/{id}", w, s.deleteMailbox)
	s.route(mux, "GET "+a+"/mail-aliases", r, s.listAliases)
	s.route(mux, "POST "+a+"/mail-aliases", w, s.createAlias)
	s.route(mux, "DELETE "+a+"/mail-aliases/{id}", w, s.deleteAlias)
	s.route(mux, "POST "+a+"/mail/dkim", w, s.enableDKIM)

	s.route(mux, "GET "+a+"/apps", r, s.listApps)
	s.route(mux, "POST "+a+"/apps", w, s.createApp)
	s.route(mux, "POST "+a+"/apps/{id}/{action}", w, s.appAction)
	s.route(mux, "DELETE "+a+"/apps/{id}", w, s.deleteApp)

	s.route(mux, "GET "+a+"/backups", r, s.listBackups)
	s.route(mux, "POST "+a+"/backups", w, s.createBackup)
	s.route(mux, "POST "+a+"/backups/{id}/restore", w, s.restoreBackup)
	s.route(mux, "DELETE "+a+"/backups/{id}", w, s.deleteBackup)
}

// within enforces a package quota. limit == 0 means unlimited.
func (s *Server) within(ctx context.Context, table, accountID string, limit int, what string) error {
	if limit == 0 {
		return nil
	}
	switch table { // table names are constants from this file, never user input
	case "domains", "db_instances", "mailboxes", "cron_jobs", "apps", "ftp_accounts", "ip_blocks", "remote_mysql_hosts", "git_repos":
	default:
		return fmt.Errorf("within: unsupported table %q", table)
	}
	var n int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM `+table+` WHERE account_id = ?`, accountID).Scan(&n); err != nil {
		return err
	}
	if n >= limit {
		return errLimit(what)
	}
	return nil
}

func (s *Server) usage(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	count := func(table string) int {
		var n int
		_ = s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM `+table+` WHERE account_id = ?`, a.ID).Scan(&n)
		return n
	}
	writeJSON(w, 200, map[string]any{
		"disk":      map[string]int{"used": a.DiskUsedMB, "limit": a.pkg.DiskMB},
		"bandwidth": map[string]int{"used": a.BWUsedMB, "limit": a.pkg.BandwidthMB},
		"domains":   map[string]int{"used": count("domains"), "limit": a.pkg.MaxDomains},
		"databases": map[string]int{"used": count("db_instances"), "limit": a.pkg.MaxDatabases},
		"mailboxes": map[string]int{"used": count("mailboxes"), "limit": a.pkg.MaxMailboxes},
		"cron":      map[string]int{"used": count("cron_jobs"), "limit": a.pkg.MaxCron},
		"apps":      map[string]int{"used": count("apps"), "limit": a.pkg.MaxApps},
		"package":   a.PackageName,
		"status":    a.Status,
	})
	return nil
}

// ---- domains ----------------------------------------------------------------

type domain struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	DocRoot    string `json:"doc_root"`
	PHP        string `json:"php_version"`
	ForceHTTPS bool   `json:"force_https"`
	HasSSL     bool   `json:"has_ssl"`
	CreatedAt  int64  `json:"created_at"`
}

func (s *Server) domainsOf(ctx context.Context, accountID string) ([]domain, error) {
	rows, err := s.db.Query(ctx, `SELECT d.id, d.name, d.kind, d.doc_root, d.php_version, d.force_https, d.created_at,
		(SELECT COUNT(*) FROM ssl_certs c WHERE c.domain_id = d.id) FROM domains d WHERE d.account_id = ? ORDER BY d.name`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain{}
	for rows.Next() {
		var d domain
		var f, c int
		if err := rows.Scan(&d.ID, &d.Name, &d.Kind, &d.DocRoot, &d.PHP, &f, &d.CreatedAt, &c); err != nil {
			return nil, err
		}
		d.ForceHTTPS, d.HasSSL = f == 1, c > 0
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Server) listDomains(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	out, err := s.domainsOf(r.Context(), a.ID)
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) domainOf(ctx context.Context, accountID, id string) (domain, error) {
	var d domain
	var f int
	err := s.db.QueryRow(ctx, `SELECT id, name, kind, doc_root, php_version, force_https, created_at FROM domains WHERE id = ? AND account_id = ?`,
		id, accountID).Scan(&d.ID, &d.Name, &d.Kind, &d.DocRoot, &d.PHP, &f, &d.CreatedAt)
	d.ForceHTTPS = f == 1
	return d, store.IsNoRows(err)
}

// vhostFor builds the nginx description of a stored domain, attaching its
// certificate when one is installed.
func (s *Server) vhostFor(ctx context.Context, d domain) (provision.Vhost, error) {
	v := provision.Vhost{Domain: d.Name, DocRoot: d.DocRoot, PHP: d.PHP, ForceHTTPS: d.ForceHTTPS}
	if d.Kind == "primary" || d.Kind == "addon" {
		v.Aliases = []string{"www." + d.Name}
	}
	err := s.db.QueryRow(ctx, `SELECT cert_path, key_path FROM ssl_certs WHERE domain_id = ? ORDER BY created_at DESC LIMIT 1`, d.ID).
		Scan(&v.CertPath, &v.KeyPath)
	if err != nil && store.IsNoRows(err) != store.ErrNotFound {
		return v, err
	}
	if v.Redirects, err = s.redirectsOf(ctx, d.ID); err != nil {
		return v, err
	}
	var pages string
	if err := s.db.QueryRow(ctx, `SELECT error_pages FROM domains WHERE id = ?`, d.ID).Scan(&pages); err != nil {
		return v, err
	}
	v.ErrorPages = parseCodes(pages)
	var accountID string
	if err := s.db.QueryRow(ctx, `SELECT account_id FROM domains WHERE id = ?`, d.ID).Scan(&accountID); err != nil {
		return v, err
	}
	if err := s.webProtection(ctx, accountID, d.ID, &v); err != nil {
		return v, err
	}
	// An app attached to this domain keeps it proxied across rewrites.
	_ = s.db.QueryRow(ctx, `SELECT port FROM apps WHERE domain_id = ? ORDER BY created_at LIMIT 1`, d.ID).Scan(&v.ProxyPort)
	return v, nil
}

func (s *Server) rewriteVhosts(ctx context.Context, a *acct) error {
	doms, err := s.domainsOf(ctx, a.ID)
	if err != nil {
		return err
	}
	for _, d := range doms {
		v, err := s.vhostFor(ctx, d)
		if err != nil {
			return err
		}
		if err := s.prov.WriteVhost(ctx, v, a.prov()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) addDomain(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name    string `json:"name"`
		Kind    string `json:"kind"`
		DocRoot string `json:"doc_root"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if a.Status != "active" {
		return errConflict("account is suspended")
	}
	req.Name = strings.ToLower(strings.TrimSpace(req.Name))
	if err := system.Domain(req.Name); err != nil {
		return err
	}
	switch req.Kind {
	case "addon", "subdomain", "alias":
	default:
		return errBadRequest("kind must be addon, subdomain or alias")
	}
	if req.Kind == "subdomain" {
		doms, _ := s.accountDomains(r.Context(), a.ID)
		ok := false
		for _, d := range doms {
			if strings.HasSuffix(req.Name, "."+d) {
				ok = true
			}
		}
		if !ok {
			return errBadRequest("a subdomain must belong to one of your domains")
		}
	}
	if err := s.within(r.Context(), "domains", a.ID, a.pkg.MaxDomains, "domains"); err != nil {
		return err
	}
	root, err := safeDocRoot(a.HomeDir, req.DocRoot, req.Name)
	if err != nil {
		return err
	}
	d := domain{ID: ids.New(), Name: req.Name, Kind: req.Kind, DocRoot: root, PHP: firstPHP(a.pkg.PHPVersions), ForceHTTPS: true}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO domains (id, account_id, name, kind, doc_root, php_version, force_https, created_at) VALUES (?,?,?,?,?,?,1,?)`,
		d.ID, a.ID, d.Name, d.Kind, d.DocRoot, d.PHP, time.Now().Unix()); err != nil {
		return err
	}
	pa := a.prov()
	rollback := func() { _, _ = s.db.Exec(context.WithoutCancel(r.Context()), `DELETE FROM domains WHERE id = ?`, d.ID) }
	if _, err := s.prov.Runner().Run(r.Context(), "install", "-d", "-m", "0755", "-o", a.SysUser, "-g", a.SysUser, d.DocRoot); err != nil {
		rollback()
		return err
	}
	v, _ := s.vhostFor(r.Context(), d)
	if err := s.prov.WriteVhost(r.Context(), v, pa); err != nil {
		rollback()
		return err
	}
	if req.Kind == "addon" {
		if err := s.prov.DNS().CreateZone(r.Context(), d.Name, s.serverIP(r.Context()), s.nameservers(r.Context())); err != nil {
			s.log.Warn("dns zone create failed", "domain", d.Name, "err", err)
		}
	}
	s.audit(r, "domain.add", d.Name, true, "")
	writeJSON(w, 201, d)
	return nil
}

// safeDocRoot returns an absolute document root that is guaranteed to sit
// inside the account's home directory.
func safeDocRoot(home, requested, name string) (string, error) {
	rel := requested
	if rel == "" {
		rel = "public_html/" + name
	}
	if strings.ContainsAny(rel, "\x00\r\n") || path.IsAbs(rel) {
		return "", errBadRequest("document root must be a relative path inside your home directory")
	}
	clean := path.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, "../") || clean == "." {
		return "", errBadRequest("document root must stay inside your home directory")
	}
	return path.Join(home, clean), nil
}

func (s *Server) updateDomain(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		PHP        *string `json:"php_version"`
		ForceHTTPS *bool   `json:"force_https"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	d, err := s.domainOf(r.Context(), a.ID, r.PathValue("id"))
	if err != nil {
		return err
	}
	if req.PHP != nil {
		if err := system.PHPVersion(*req.PHP); err != nil {
			return err
		}
		allowed := false
		for _, v := range strings.Split(a.pkg.PHPVersions, ",") {
			if strings.TrimSpace(v) == *req.PHP {
				allowed = true
			}
		}
		if !allowed {
			return errBadRequest("that PHP version is not included in your package")
		}
		d.PHP = *req.PHP
	}
	if req.ForceHTTPS != nil {
		d.ForceHTTPS = *req.ForceHTTPS
	}
	f := 0
	if d.ForceHTTPS {
		f = 1
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE domains SET php_version=?, force_https=? WHERE id=?`, d.PHP, f, d.ID); err != nil {
		return err
	}
	pa := a.prov()
	pa.PHP = d.PHP
	if err := s.prov.ApplyPHPPool(r.Context(), pa); err != nil {
		return err
	}
	v, err := s.vhostFor(r.Context(), d)
	if err != nil {
		return err
	}
	if err := s.prov.WriteVhost(r.Context(), v, pa); err != nil {
		return err
	}
	s.audit(r, "domain.update", d.Name, true, "")
	writeJSON(w, 200, d)
	return nil
}

func (s *Server) deleteDomain(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	d, err := s.domainOf(r.Context(), a.ID, r.PathValue("id"))
	if err != nil {
		return err
	}
	if d.Kind == "primary" {
		return errConflict("the primary domain cannot be removed")
	}
	if err := s.prov.RemoveVhost(r.Context(), d.Name); err != nil {
		return err
	}
	_ = s.prov.DNS().DeleteZone(r.Context(), d.Name)
	if _, err := s.db.Exec(r.Context(), `DELETE FROM domains WHERE id = ?`, d.ID); err != nil {
		return err
	}
	_ = s.prov.ReloadWeb(r.Context())
	s.audit(r, "domain.delete", d.Name, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- SSL --------------------------------------------------------------------

func (s *Server) listCerts(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT c.id, d.id, d.name, c.issuer, c.names, c.not_after, c.auto_renew FROM ssl_certs c
		JOIN domains d ON d.id = c.domain_id WHERE d.account_id = ? ORDER BY d.name`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type cv struct {
		ID        string `json:"id"`
		DomainID  string `json:"domain_id"`
		Domain    string `json:"domain"`
		Issuer    string `json:"issuer"`
		Names     string `json:"names"`
		NotAfter  int64  `json:"not_after"`
		AutoRenew bool   `json:"auto_renew"`
	}
	out := []cv{}
	for rows.Next() {
		var v cv
		var ar int
		if err := rows.Scan(&v.ID, &v.DomainID, &v.Domain, &v.Issuer, &v.Names, &v.NotAfter, &ar); err != nil {
			return err
		}
		v.AutoRenew = ar == 1
		out = append(out, v)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) saveCert(ctx context.Context, a *acct, d domain, issuer string, names []string, c *provision.Certificate) error {
	certPath, keyPath, err := s.prov.InstallCertificate(d.Name, c)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `DELETE FROM ssl_certs WHERE domain_id = ?`, d.ID); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO ssl_certs (id, domain_id, issuer, names, not_after, cert_path, key_path, auto_renew, created_at)
		VALUES (?,?,?,?,?,?,?,1,?)`, ids.New(), d.ID, issuer, strings.Join(names, ","), c.NotAfter.Unix(), certPath, keyPath, time.Now().Unix()); err != nil {
		return err
	}
	v, err := s.vhostFor(ctx, d)
	if err != nil {
		return err
	}
	return s.prov.WriteVhost(ctx, v, a.prov())
}

func (s *Server) issueLE(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	d, err := s.domainOf(r.Context(), a.ID, r.PathValue("id"))
	if err != nil {
		return err
	}
	names := []string{d.Name}
	if d.Kind == "primary" || d.Kind == "addon" {
		names = append(names, "www."+d.Name)
	}
	cert, err := s.prov.IssueLetsEncrypt(r.Context(), a.Email, names, false)
	if err != nil {
		s.audit(r, "ssl.issue", d.Name, false, err.Error())
		return &apiError{502, "acme_failed", "certificate could not be issued: " + err.Error()}
	}
	if err := s.saveCert(r.Context(), a, d, "letsencrypt", names, cert); err != nil {
		return err
	}
	s.audit(r, "ssl.issue", d.Name, true, "letsencrypt")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) installCustomCert(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Certificate string `json:"certificate"`
		Key         string `json:"key"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	d, err := s.domainOf(r.Context(), a.ID, r.PathValue("id"))
	if err != nil {
		return err
	}
	pair, err := tls.X509KeyPair([]byte(req.Certificate), []byte(req.Key))
	if err != nil {
		return errBadRequest("certificate and private key do not match or are not valid PEM")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return errBadRequest("certificate could not be parsed")
	}
	if err := leaf.VerifyHostname(d.Name); err != nil {
		return errBadRequest("certificate does not cover " + d.Name)
	}
	if time.Now().After(leaf.NotAfter) {
		return errBadRequest("certificate has expired")
	}
	// Re-encode so only well-formed PEM blocks reach disk.
	var chain []byte
	for _, der := range pair.Certificate {
		chain = append(chain, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	keyBlock, _ := pem.Decode([]byte(req.Key))
	if keyBlock == nil {
		return errBadRequest("private key is not valid PEM")
	}
	cert := &provision.Certificate{CertPEM: chain, KeyPEM: pem.EncodeToMemory(keyBlock), NotAfter: leaf.NotAfter}
	if err := s.saveCert(r.Context(), a, d, "custom", leaf.DNSNames, cert); err != nil {
		return err
	}
	s.audit(r, "ssl.install", d.Name, true, "custom")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) removeCert(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	d, err := s.domainOf(r.Context(), a.ID, r.PathValue("id"))
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM ssl_certs WHERE domain_id = ?`, d.ID); err != nil {
		return err
	}
	v, err := s.vhostFor(r.Context(), d)
	if err != nil {
		return err
	}
	if err := s.prov.WriteVhost(r.Context(), v, a.prov()); err != nil {
		return err
	}
	s.audit(r, "ssl.remove", d.Name, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- DNS --------------------------------------------------------------------

// zoneOwned verifies the zone is a domain of the account, so one customer can
// never edit another customer's DNS.
func (s *Server) zoneOwned(ctx context.Context, a *acct, zone string) error {
	var n int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM domains WHERE account_id = ? AND name = ? AND kind IN ('primary','addon')`, a.ID, zone).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Server) listDNS(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	zone := r.PathValue("zone")
	if err := s.zoneOwned(r.Context(), a, zone); err != nil {
		return err
	}
	recs, err := s.prov.DNS().ListRecords(r.Context(), zone)
	if err != nil {
		return err
	}
	if recs == nil {
		recs = []provision.Record{}
	}
	writeJSON(w, 200, recs)
	return nil
}

func (s *Server) upsertDNS(w http.ResponseWriter, r *http.Request) error {
	var rec provision.Record
	if err := decode(r, &rec); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	zone := r.PathValue("zone")
	if err := s.zoneOwned(r.Context(), a, zone); err != nil {
		return err
	}
	if rec.Type == "SOA" || strings.EqualFold(rec.Type, "SOA") {
		return errBadRequest("SOA records are managed by the server")
	}
	if err := s.prov.DNS().UpsertRecord(r.Context(), zone, rec); err != nil {
		return err
	}
	s.audit(r, "dns.upsert", zone, true, rec.Type+" "+rec.Name)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) deleteDNS(w http.ResponseWriter, r *http.Request) error {
	var rec provision.Record
	if err := decode(r, &rec); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	zone := r.PathValue("zone")
	if err := s.zoneOwned(r.Context(), a, zone); err != nil {
		return err
	}
	if err := s.prov.DNS().DeleteRecord(r.Context(), zone, rec); err != nil {
		return err
	}
	s.audit(r, "dns.delete", zone, true, rec.Type+" "+rec.Name)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- cron -------------------------------------------------------------------

type cronJob struct {
	ID       string `json:"id"`
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
	Enabled  bool   `json:"enabled"`
}

func (s *Server) cronJobs(ctx context.Context, accountID string) ([]cronJob, error) {
	rows, err := s.db.Query(ctx, `SELECT id, schedule, command, enabled FROM cron_jobs WHERE account_id = ? ORDER BY created_at`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []cronJob{}
	for rows.Next() {
		var j cronJob
		var e int
		if err := rows.Scan(&j.ID, &j.Schedule, &j.Command, &e); err != nil {
			return nil, err
		}
		j.Enabled = e == 1
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Server) syncCron(ctx context.Context, a *acct) error {
	jobs, err := s.cronJobs(ctx, a.ID)
	if err != nil {
		return err
	}
	pj := make([]provision.CronJob, len(jobs))
	for i, j := range jobs {
		pj[i] = provision.CronJob{Schedule: j.Schedule, Command: j.Command, Enabled: j.Enabled}
	}
	return s.prov.WriteCron(ctx, a.prov(), pj)
}

func (s *Server) listCron(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	out, err := s.cronJobs(r.Context(), a.ID)
	if err != nil {
		return err
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) addCron(w http.ResponseWriter, r *http.Request) error {
	var req cronJob
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if err := system.Cron(req.Schedule); err != nil {
		return err
	}
	if err := system.OneLine(req.Command, 1024); err != nil {
		return err
	}
	if err := s.within(r.Context(), "cron_jobs", a.ID, a.pkg.MaxCron, "cron jobs"); err != nil {
		return err
	}
	req.ID, req.Enabled = ids.New(), true
	if _, err := s.db.Exec(r.Context(), `INSERT INTO cron_jobs (id, account_id, schedule, command, enabled, created_at) VALUES (?,?,?,?,1,?)`,
		req.ID, a.ID, req.Schedule, req.Command, time.Now().Unix()); err != nil {
		return err
	}
	if err := s.syncCron(r.Context(), a); err != nil {
		return err
	}
	s.audit(r, "cron.add", a.SysUser, true, req.Schedule)
	writeJSON(w, 201, req)
	return nil
}

func (s *Server) toggleCron(w http.ResponseWriter, r *http.Request) error {
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
	e := 0
	if req.Enabled {
		e = 1
	}
	res, err := s.db.Exec(r.Context(), `UPDATE cron_jobs SET enabled = ? WHERE id = ? AND account_id = ?`, e, r.PathValue("id"), a.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	if err := s.syncCron(r.Context(), a); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) deleteCron(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM cron_jobs WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID); err != nil {
		return err
	}
	if err := s.syncCron(r.Context(), a); err != nil {
		return err
	}
	s.audit(r, "cron.delete", a.SysUser, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- SSH keys -----------------------------------------------------------------

type sshKey struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
	PublicKey   string `json:"public_key,omitempty"`
	CreatedAt   int64  `json:"created_at"`
}

func (s *Server) listSSHKeys(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, name, fingerprint, created_at FROM ssh_keys WHERE account_id = ? ORDER BY created_at`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []sshKey{}
	for rows.Next() {
		var k sshKey
		if err := rows.Scan(&k.ID, &k.Name, &k.Fingerprint, &k.CreatedAt); err != nil {
			return err
		}
		out = append(out, k)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) syncSSHKeys(ctx context.Context, a *acct) error {
	rows, err := s.db.Query(ctx, `SELECT public_key FROM ssh_keys WHERE account_id = ? ORDER BY created_at`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return err
		}
		b.WriteString(k + "\n")
	}
	if err := rows.Err(); err != nil {
		return err
	}
	file := path.Join(a.HomeDir, ".ssh", "authorized_keys")
	if err := s.prov.Runner().WriteFile(file, []byte(b.String()), 0o600); err != nil {
		return err
	}
	_, err = s.prov.Runner().Run(ctx, "chown", a.SysUser+":"+a.SysUser, file)
	return err
}

func (s *Server) addSSHKey(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name      string `json:"name"`
		PublicKey string `json:"public_key"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := system.OneLine(req.Name, 64); err != nil {
		return errBadRequest("key name is required")
	}
	pub, _, opts, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(req.PublicKey)))
	if err != nil || len(opts) > 0 {
		// Options (command=, from=) could be abused to widen access; reject them.
		return errBadRequest("enter a valid OpenSSH public key without options")
	}
	if pub.Type() == "ssh-dss" {
		return errBadRequest("DSA keys are not supported")
	}
	canonical := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
	k := sshKey{ID: ids.New(), Name: req.Name, Fingerprint: ssh.FingerprintSHA256(pub)}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO ssh_keys (id, account_id, name, public_key, fingerprint, created_at) VALUES (?,?,?,?,?,?)`,
		k.ID, a.ID, k.Name, canonical, k.Fingerprint, time.Now().Unix()); err != nil {
		return err
	}
	if err := s.syncSSHKeys(r.Context(), a); err != nil {
		return err
	}
	s.audit(r, "ssh.add", a.SysUser, true, k.Fingerprint)
	writeJSON(w, 201, k)
	return nil
}

func (s *Server) deleteSSHKey(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM ssh_keys WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID); err != nil {
		return err
	}
	if err := s.syncSSHKeys(r.Context(), a); err != nil {
		return err
	}
	s.audit(r, "ssh.delete", a.SysUser, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}
