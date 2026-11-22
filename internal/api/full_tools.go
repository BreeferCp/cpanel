package api

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/files"
	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/provision"
	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/store"
	"github.com/breefercp/breefercp/internal/system"
)

func isMissing(err error) bool { return errors.Is(store.IsNoRows(err), store.ErrNotFound) }

func (s *Server) fullToolRoutes(mux *http.ServeMux) {
	a := "/api/v1/accounts/{aid}"
	r, w := rbac.HostingRead, rbac.HostingWrite

	// Backups hold every secret of the account, so reading one needs write rights.
	s.route(mux, "GET "+a+"/backups/{id}/download", w, s.downloadBackup)
	s.route(mux, "POST "+a+"/ssl/csr", w, s.generateCSR)
	s.route(mux, "GET "+a+"/ssl/autossl", r, s.listAutoSSL)
	s.route(mux, "PUT "+a+"/ssl/autossl/{id}", w, s.putAutoSSL)
	s.route(mux, "GET "+a+"/logs/archives", r, s.listLogArchives)
	s.route(mux, "GET "+a+"/logs/archives/{name}", r, s.downloadLogArchive)
	s.route(mux, "PUT "+a+"/protected-dirs/{id}/leech", w, s.putLeech)
}

// ---- backups ----------------------------------------------------------------

func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	var file, status string
	if err := s.db.QueryRow(r.Context(), `SELECT path, status FROM backups WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID).Scan(&file, &status); err != nil {
		return store.IsNoRows(err)
	}
	if status != "complete" {
		return errConflict("backup is not complete")
	}
	f, err := os.Open(file)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return errNotFound()
		}
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	s.audit(r, "backup.download", a.SysUser, true, r.PathValue("id"))
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(file)+`"`)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "", info.ModTime(), f)
	return nil
}

// restoreDatabases imports the dumps in a backup into the account's databases.
func (s *Server) restoreDatabases(ctx context.Context, a *acct, file string, names []string) (int, error) {
	run := s.prov.Runner()
	stage := filepath.Join(s.cfg.DataDir, "staging", a.SysUser+"-restore-"+ids.New())
	if _, err := run.Run(ctx, "install", "-d", "-m", "0700", stage); err != nil {
		return 0, err
	}
	defer run.Run(context.WithoutCancel(ctx), "rm", "-rf", "--", stage) //nolint:errcheck
	done := 0
	for _, name := range names {
		if err := system.DBName(name); err != nil {
			return done, err
		}
		dump := filepath.Join(stage, name+".sql")
		// The first member is the panel's dump; a file planted in the home comes later.
		if err := run.RunToFile(ctx, dump, "tar", "-xzOf", file, "--occurrence=1", "--", "./"+name+".sql"); err != nil {
			if strings.Contains(err.Error(), "Not found in archive") {
				continue // created after this backup
			}
			return done, err
		}
		if err := s.prov.ImportDatabase(ctx, a.prov(), name, dump); err != nil {
			return done, err
		}
		done++
	}
	return done, nil
}

func (s *Server) databaseNames(ctx context.Context, accountID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT name FROM db_instances WHERE account_id = ?`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ---- keys and signing requests ------------------------------------------------

var (
	reCountry = regexp.MustCompile(`^[A-Z]{2}$`)
	reOrg     = regexp.MustCompile(`^[\pL\pN .,&'()-]{1,64}$`)
)

func (s *Server) generateCSR(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		DomainID     string `json:"domain_id"`
		KeyType      string `json:"key_type"`
		Organization string `json:"organization"`
		Country      string `json:"country"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	d, err := s.domainOf(r.Context(), a.ID, req.DomainID)
	if err != nil {
		return errBadRequest("choose one of your domains")
	}
	tpl := x509.CertificateRequest{Subject: pkix.Name{CommonName: d.Name}, DNSNames: []string{d.Name}}
	if d.Kind == "primary" || d.Kind == "addon" {
		tpl.DNSNames = append(tpl.DNSNames, "www."+d.Name)
	}
	if v := strings.TrimSpace(req.Organization); v != "" {
		if !reOrg.MatchString(v) {
			return errBadRequest("organization may use letters, digits and . , & ' ( ) -")
		}
		tpl.Subject.Organization = []string{v}
	}
	if v := strings.ToUpper(strings.TrimSpace(req.Country)); v != "" {
		if !reCountry.MatchString(v) {
			return errBadRequest("country is a two-letter code such as BD or US")
		}
		tpl.Subject.Country = []string{v}
	}
	var key crypto.Signer
	switch req.KeyType {
	case "", "ecdsa-p256":
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "ecdsa-p384":
		key, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case "rsa-2048":
		key, err = rsa.GenerateKey(rand.Reader, 2048)
	case "rsa-4096":
		key, err = rsa.GenerateKey(rand.Reader, 4096)
	default:
		return errBadRequest("unknown key type")
	}
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &tpl, key)
	if err != nil {
		return err
	}
	// The key is returned once and never stored.
	s.audit(r, "ssl.csr", d.Name, true, req.KeyType)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]string{
		"key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})),
		"csr": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})),
	})
	return nil
}

// ---- AutoSSL exclusions --------------------------------------------------------

func (s *Server) listAutoSSL(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, autossl FROM domains WHERE account_id = ?`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		var on int
		if err := rows.Scan(&id, &on); err != nil {
			return err
		}
		out[id] = on == 1
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) putAutoSSL(w http.ResponseWriter, r *http.Request) error {
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
	d, err := s.domainOf(r.Context(), a.ID, r.PathValue("id"))
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE domains SET autossl = ? WHERE id = ?`, b2i(req.Enabled), d.ID); err != nil {
		return err
	}
	s.audit(r, "ssl.autossl", d.Name, true, strconv.FormatBool(req.Enabled))
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- archived logs -----------------------------------------------------------

var reArchiveName = regexp.MustCompile(`^([a-z0-9.-]+)\.(access|error)\.log\.[0-9]{1,3}(\.gz)?$`)

// archiveAllowed ties a rotated file to a domain the account has now.
func archiveAllowed(doms []domain, name string) bool {
	m := reArchiveName.FindStringSubmatch(name)
	if m == nil {
		return false
	}
	for _, d := range doms {
		if d.Name == m[1] {
			return true
		}
	}
	return false
}

func (s *Server) listLogArchives(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	doms, err := s.domainsOf(r.Context(), a.ID)
	if err != nil {
		return err
	}
	type archive struct {
		Name    string    `json:"name"`
		Size    int64     `json:"size"`
		ModTime time.Time `json:"mod_time"`
	}
	out := []archive{}
	jail, err := files.Open(provision.LogDir(a.prov()), -1, -1)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeJSON(w, 200, out)
			return nil
		}
		return err
	}
	defer jail.Close()
	list, err := jail.List("/")
	if err != nil {
		return fsErr(err)
	}
	for _, e := range list {
		if !e.Dir && !e.Symlink && archiveAllowed(doms, e.Name) {
			out = append(out, archive{e.Name, e.Size, e.ModTime})
		}
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) downloadLogArchive(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	doms, err := s.domainsOf(r.Context(), a.ID)
	if err != nil {
		return err
	}
	name := r.PathValue("name")
	if !archiveAllowed(doms, name) {
		return errNotFound()
	}
	jail, err := files.Open(provision.LogDir(a.prov()), -1, -1)
	if err != nil {
		return fsErr(err)
	}
	defer jail.Close()
	f, info, err := jail.OpenRead(name)
	if err != nil {
		return fsErr(err)
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Security-Policy", "sandbox")
	http.ServeContent(w, r, "", info.ModTime(), f)
	return nil
}

// ---- notices ----------------------------------------------------------------

// securityNotice mails the account's contacts, plus extra addresses (the old
// one, when the contact address itself changed).
func (s *Server) securityNotice(ctx context.Context, userID, key, subject, body string, extra ...string) {
	c := s.contactOf(ctx, userID)
	if !c.Notify[key] {
		return
	}
	to := []string{}
	seen := map[string]bool{"": true}
	for _, v := range append([]string{c.Email, c.Secondary}, extra...) {
		if !seen[v] {
			seen[v] = true
			to = append(to, v)
		}
	}
	s.sendNotice(ctx, to, subject, body+"\n\nIf this was not you, contact your hosting provider now.")
}

// certNotices warns a week apart about certificates expiring within 14 days.
// Renewal starts at 30 days, so a Let's Encrypt one here is failing to renew.
func (s *Server) certNotices(ctx context.Context) {
	now := time.Now().Unix()
	rows, err := s.db.Query(ctx, `SELECT c.id, d.name, c.not_after, a.user_id FROM ssl_certs c
		JOIN domains d ON d.id = c.domain_id JOIN accounts a ON a.id = d.account_id
		WHERE a.status = 'active' AND c.not_after < ?`, now+14*86400)
	if err != nil {
		s.log.Error("certificate notices", "err", err)
		return
	}
	type expiring struct {
		id, name, uid string
		until         int64
	}
	var list []expiring
	for rows.Next() {
		var x expiring
		if rows.Scan(&x.id, &x.name, &x.until, &x.uid) == nil {
			list = append(list, x)
		}
	}
	rows.Close()
	for _, x := range list {
		c := s.contactOf(ctx, x.uid)
		if !c.Notify["ssl_expiry"] {
			continue
		}
		mark := "notified:ssl:" + x.id
		var last string
		_ = s.db.QueryRow(ctx, `SELECT value FROM user_prefs WHERE user_id = ? AND key = ?`, x.uid, mark).Scan(&last)
		if t, _ := strconv.ParseInt(last, 10, 64); now-t < 7*86400 {
			continue
		}
		if _, err := s.db.Exec(ctx, `INSERT INTO user_prefs (user_id, key, value) VALUES (?,?,?)
			ON CONFLICT (user_id, key) DO UPDATE SET value = excluded.value`, x.uid, mark, strconv.FormatInt(now, 10)); err != nil {
			s.log.Error("certificate notices", "err", err)
			continue
		}
		s.sendNotice(ctx, []string{c.Email, c.Secondary}, "The SSL certificate for "+x.name+" expires soon",
			fmt.Sprintf("The certificate for %s expires on %s.\n\nInstall a new one, or get a free one, under SSL/TLS.", x.name, time.Unix(x.until, 0).UTC().Format("2 January 2006")))
	}
}
