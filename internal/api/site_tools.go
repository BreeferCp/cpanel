package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/mail"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/auth"
	"github.com/breefercp/breefercp/internal/files"
	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/provision"
	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/store"
	"github.com/breefercp/breefercp/internal/system"
)

func (s *Server) siteToolRoutes(mux *http.ServeMux) {
	a := "/api/v1/accounts/{aid}"
	r, w := rbac.HostingRead, rbac.HostingWrite

	s.route(mux, "GET "+a+"/indexes", r, s.listIndexes)
	s.route(mux, "POST "+a+"/indexes", w, s.addIndex)
	s.route(mux, "DELETE "+a+"/indexes/{id}", w, s.deleteIndex)

	s.route(mux, "GET "+a+"/mime-types", r, s.listMime)
	s.route(mux, "POST "+a+"/mime-types", w, s.addMime)
	s.route(mux, "DELETE "+a+"/mime-types/{id}", w, s.deleteMime)

	s.route(mux, "GET "+a+"/optimize", r, s.getOptimize)
	s.route(mux, "PUT "+a+"/optimize", w, s.putOptimize)

	s.route(mux, "GET "+a+"/ddns", r, s.listDDNS)
	s.route(mux, "POST "+a+"/ddns", w, s.createDDNS)
	s.route(mux, "DELETE "+a+"/ddns/{id}", w, s.deleteDDNS)
	mux.HandleFunc("GET /api/v1/ddns/update", s.wrap(s.ddnsUpdate))
	mux.HandleFunc("POST /api/v1/ddns/update", s.wrap(s.ddnsUpdate))

	s.route(mux, "POST "+a+"/dns-lookup", r, s.dnsLookup)
	s.route(mux, "POST "+a+"/traceroute", r, s.traceroute)

	s.route(mux, "GET "+a+"/visitors", r, s.visitors)
	s.route(mux, "GET "+a+"/bandwidth", r, s.bandwidth)
	s.route(mux, "GET "+a+"/logs/download", r, s.downloadLog)

	s.route(mux, "GET /api/v1/me/contact", rbac.ProfileWrite, s.getContact)
	s.route(mux, "PUT /api/v1/me/contact", rbac.ProfileWrite, s.putContact)
}

// siteOptions loads the account-wide web settings applied to every vhost.
func (s *Server) siteOptions(ctx context.Context, accountID, domainID string, v *provision.Vhost) error {
	rows, err := s.db.Query(ctx, `SELECT path FROM dir_indexes WHERE domain_id = ? ORDER BY path`, domainID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return err
		}
		v.Indexes = append(v.Indexes, p)
	}
	rows.Close()
	rows, err = s.db.Query(ctx, `SELECT ext, mime FROM mime_types WHERE account_id = ? ORDER BY ext`, accountID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var m provision.MimeType
		if err := rows.Scan(&m.Ext, &m.Type); err != nil {
			rows.Close()
			return err
		}
		v.Mime = append(v.Mime, m)
	}
	rows.Close()
	if c := s.compressionOf(ctx, accountID); c.Mode != "off" {
		v.Gzip = &c
	}
	return nil
}

// ---- indexes ----------------------------------------------------------------

func (s *Server) listIndexes(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT i.id, i.domain_id, d.name, i.path, i.created_at FROM dir_indexes i JOIN domains d ON d.id = i.domain_id WHERE i.account_id = ? ORDER BY d.name, i.path`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type v struct {
		ID        string `json:"id"`
		DomainID  string `json:"domain_id"`
		Domain    string `json:"domain"`
		Path      string `json:"path"`
		CreatedAt int64  `json:"created_at"`
	}
	out := []v{}
	for rows.Next() {
		var x v
		if err := rows.Scan(&x.ID, &x.DomainID, &x.Domain, &x.Path, &x.CreatedAt); err != nil {
			return err
		}
		out = append(out, x)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) addIndex(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		DomainID string `json:"domain_id"`
		Path     string `json:"path"`
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
	p := "/" + strings.Trim(strings.TrimSpace(req.Path), "/")
	if err := provision.ValidateIndexPath(p); err != nil {
		return err
	}
	if err := s.within(r.Context(), "dir_indexes", a.ID, 100, "directory indexes"); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO dir_indexes (id, account_id, domain_id, path, created_at) VALUES (?,?,?,?,?)`,
		ids.New(), a.ID, d.ID, p, time.Now().Unix()); err != nil {
		if store.IsUnique(err) {
			return &apiError{409, "conflict", p + " already lists its files"}
		}
		return err
	}
	if err := s.applyDomain(r.Context(), a, d); err != nil {
		return err
	}
	s.audit(r, "web.index_on", d.Name+p, true, "")
	w.WriteHeader(http.StatusCreated)
	return nil
}

func (s *Server) deleteIndex(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	var domainID string
	if err := s.db.QueryRow(r.Context(), `SELECT domain_id FROM dir_indexes WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID).Scan(&domainID); err != nil {
		return store.IsNoRows(err)
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM dir_indexes WHERE id = ?`, r.PathValue("id")); err != nil {
		return err
	}
	d, err := s.domainOf(r.Context(), a.ID, domainID)
	if err != nil {
		return err
	}
	if err := s.applyDomain(r.Context(), a, d); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- MIME types ---------------------------------------------------------------

func (s *Server) listMime(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, ext, mime, created_at FROM mime_types WHERE account_id = ? ORDER BY ext`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type v struct {
		ID        string `json:"id"`
		Ext       string `json:"ext"`
		Type      string `json:"type"`
		CreatedAt int64  `json:"created_at"`
	}
	out := []v{}
	for rows.Next() {
		var x v
		if err := rows.Scan(&x.ID, &x.Ext, &x.Type, &x.CreatedAt); err != nil {
			return err
		}
		out = append(out, x)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) addMime(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Ext  string `json:"ext"`
		Type string `json:"type"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	m := provision.MimeType{Ext: strings.ToLower(strings.TrimPrefix(strings.TrimSpace(req.Ext), ".")), Type: strings.ToLower(strings.TrimSpace(req.Type))}
	if err := provision.ValidateMime(m); err != nil {
		return err
	}
	if err := s.within(r.Context(), "mime_types", a.ID, 100, "MIME types"); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO mime_types (id, account_id, ext, mime, created_at) VALUES (?,?,?,?,?)`,
		ids.New(), a.ID, m.Ext, m.Type, time.Now().Unix()); err != nil {
		if store.IsUnique(err) {
			return &apiError{409, "conflict", "." + m.Ext + " already has a type; delete it first"}
		}
		return err
	}
	if err := s.rewriteVhosts(r.Context(), a); err != nil {
		return err
	}
	s.audit(r, "web.mime_add", m.Ext, true, m.Type)
	w.WriteHeader(http.StatusCreated)
	return nil
}

func (s *Server) deleteMime(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM mime_types WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID); err != nil {
		return err
	}
	if err := s.rewriteVhosts(r.Context(), a); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- optimize website (compression) ---------------------------------------------

func (s *Server) compressionOf(ctx context.Context, accountID string) provision.Compression {
	c := provision.Compression{Mode: "off"}
	var types string
	if err := s.db.QueryRow(ctx, `SELECT gzip, gzip_types FROM web_settings WHERE account_id = ?`, accountID).Scan(&c.Mode, &types); err != nil {
		return provision.Compression{Mode: "off"}
	}
	c.Types = strings.Fields(types)
	return c
}

func (s *Server) getOptimize(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	c := s.compressionOf(r.Context(), a.ID)
	if c.Types == nil {
		c.Types = []string{}
	}
	writeJSON(w, 200, map[string]any{"mode": c.Mode, "types": c.Types, "defaults": provision.DefaultGzipTypes})
	return nil
}

func (s *Server) putOptimize(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Mode  string   `json:"mode"`
		Types []string `json:"types"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	c := provision.Compression{Mode: req.Mode}
	for _, t := range req.Types {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			c.Types = append(c.Types, t)
		}
	}
	if err := provision.ValidateCompression(c); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM web_settings WHERE account_id = ?`, a.ID); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO web_settings (account_id, gzip, gzip_types) VALUES (?,?,?)`, a.ID, c.Mode, strings.Join(c.Types, " ")); err != nil {
		return err
	}
	if err := s.rewriteVhosts(r.Context(), a); err != nil {
		return err
	}
	s.audit(r, "web.compression", a.Username, true, c.Mode)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- dynamic DNS ---------------------------------------------------------------

var reDDNSLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func (s *Server) listDDNS(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, zone, name, description, last_ip, last_update, created_at FROM ddns_hosts WHERE account_id = ? ORDER BY name, zone`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type v struct {
		ID          string `json:"id"`
		Host        string `json:"host"`
		Zone        string `json:"zone"`
		Description string `json:"description"`
		LastIP      string `json:"last_ip"`
		LastUpdate  int64  `json:"last_update"`
		CreatedAt   int64  `json:"created_at"`
	}
	out := []v{}
	for rows.Next() {
		var x v
		var name string
		if err := rows.Scan(&x.ID, &x.Zone, &name, &x.Description, &x.LastIP, &x.LastUpdate, &x.CreatedAt); err != nil {
			return err
		}
		x.Host = name + "." + x.Zone
		out = append(out, x)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) createDDNS(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name        string `json:"name"`
		Zone        string `json:"zone"`
		Description string `json:"description"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	name := strings.ToLower(strings.TrimSpace(req.Name))
	if !reDDNSLabel.MatchString(name) || name == "www" || name == "mail" {
		return errBadRequest("the host name is one label such as home or office (not www or mail)")
	}
	if err := s.zoneOwned(r.Context(), a, req.Zone); err != nil {
		return errBadRequest("choose one of your domains")
	}
	desc, err := validDescription(req.Description)
	if err != nil {
		return err
	}
	if err := s.within(r.Context(), "ddns_hosts", a.ID, 20, "dynamic DNS hosts"); err != nil {
		return err
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		return err
	}
	id := ids.New()
	if _, err := s.db.Exec(r.Context(), `INSERT INTO ddns_hosts (id, account_id, zone, name, token_hash, description, created_at) VALUES (?,?,?,?,?,?,?)`,
		id, a.ID, req.Zone, name, hash, desc, time.Now().Unix()); err != nil {
		if store.IsUnique(err) {
			return &apiError{409, "conflict", name + "." + req.Zone + " already exists"}
		}
		return err
	}
	s.audit(r, "ddns.create", name+"."+req.Zone, true, "")
	writeJSON(w, 201, map[string]string{"id": id, "token": token, "path": "/api/v1/ddns/update?token=" + token})
	return nil
}

func (s *Server) deleteDDNS(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM ddns_hosts WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ddnsUpdate is the public endpoint routers and scripts call. The token is
// the only credential and only allows changing that one A/AAAA record. The
// reply follows the dyndns2 convention (good / nochg / badauth).
func (s *Server) ddnsUpdate(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("X-DDNS-Token")
	}
	if !s.loginLimit.allow("ddns:" + s.clientIP(r)) {
		w.WriteHeader(429)
		_, _ = io.WriteString(w, "abuse\n")
		return nil
	}
	var id, zone, name, last, status string
	err := s.db.QueryRow(r.Context(), `SELECT h.id, h.zone, h.name, h.last_ip, a.status FROM ddns_hosts h JOIN accounts a ON a.id = h.account_id WHERE h.token_hash = ?`,
		auth.HashToken(token)).Scan(&id, &zone, &name, &last, &status)
	if err != nil || token == "" || status != "active" {
		s.loginLimit.fail("ddns:" + s.clientIP(r))
		w.WriteHeader(401)
		_, _ = io.WriteString(w, "badauth\n")
		return nil
	}
	ip := net.ParseIP(s.clientIP(r))
	if q := r.URL.Query().Get("ip"); q != "" {
		ip = net.ParseIP(q)
	}
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() {
		if !s.cfg.DryRun || ip == nil {
			w.WriteHeader(400)
			_, _ = io.WriteString(w, "badip\n")
			return nil
		}
	}
	if ip.String() == last {
		_, _ = io.WriteString(w, "nochg "+last+"\n")
		return nil
	}
	typ := "A"
	if ip.To4() == nil {
		typ = "AAAA"
	}
	if err := s.prov.DNS().UpsertRecord(r.Context(), zone, provision.Record{Name: name, Type: typ, TTL: 60, Content: ip.String()}); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE ddns_hosts SET last_ip = ?, last_update = ? WHERE id = ?`, ip.String(), time.Now().Unix(), id); err != nil {
		return err
	}
	s.auditAs(r.Context(), "", "ddns", s.clientIP(r), "ddns.update", name+"."+zone, true, ip.String())
	_, _ = io.WriteString(w, "good "+ip.String()+"\n")
	return nil
}

// ---- track DNS -------------------------------------------------------------------

func lookupHost(raw string) (string, error) {
	h := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
	if net.ParseIP(h) != nil {
		return h, nil
	}
	if err := system.Domain(h); err != nil {
		return "", errBadRequest("enter a host name such as example.com or an IP address")
	}
	return h, nil
}

func (s *Server) dnsLookup(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Host string `json:"host"`
		Type string `json:"type"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	h, err := lookupHost(req.Host)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	res := net.DefaultResolver
	var out []string
	switch strings.ToUpper(req.Type) {
	case "A", "AAAA":
		ips, e := res.LookupIP(ctx, map[string]string{"A": "ip4", "AAAA": "ip6"}[strings.ToUpper(req.Type)], h)
		for _, ip := range ips {
			out = append(out, ip.String())
		}
		err = e
	case "MX":
		mx, e := res.LookupMX(ctx, h)
		for _, m := range mx {
			out = append(out, fmt.Sprintf("%d %s", m.Pref, m.Host))
		}
		err = e
	case "TXT":
		out, err = res.LookupTXT(ctx, h)
	case "NS":
		ns, e := res.LookupNS(ctx, h)
		for _, n := range ns {
			out = append(out, n.Host)
		}
		err = e
	case "CNAME":
		c, e := res.LookupCNAME(ctx, h)
		if c != "" {
			out = []string{c}
		}
		err = e
	case "PTR":
		out, err = res.LookupAddr(ctx, h)
	default:
		return errBadRequest("type must be A, AAAA, MX, TXT, NS, CNAME or PTR")
	}
	msg := ""
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			msg = "no records found"
		} else {
			msg = "lookup failed: " + err.Error()
		}
	}
	if out == nil {
		out = []string{}
	}
	writeJSON(w, 200, map[string]any{"host": h, "type": strings.ToUpper(req.Type), "records": out, "message": msg})
	return nil
}

func (s *Server) traceroute(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Host string `json:"host"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	h, err := lookupHost(req.Host)
	if err != nil {
		return err
	}
	if s.cfg.DryRun {
		writeJSON(w, 200, map[string]string{"output": "traceroute to " + h + ", 20 hops max\n 1  10.0.0.1  0.412 ms\n 2  192.0.2.1  1.902 ms\n 3  198.51.100.14  4.771 ms\n 4  203.0.113.9  9.225 ms\n 5  " + h + "  11.083 ms\n"})
		return nil
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	res, err := s.prov.Runner().Run(ctx, "traceroute", "-n", "-q", "1", "-w", "2", "-m", "20", "--", h)
	out := res.Stdout
	if err != nil && out == "" {
		out = "traceroute failed: " + err.Error()
	}
	writeJSON(w, 200, map[string]string{"output": out})
	return nil
}

// ---- visitors, bandwidth, raw logs ---------------------------------------------------

// logFile resolves and authorizes a log path relative to the account home.
func (s *Server) logFile(ctx context.Context, a *acct, kind, domainName string) (string, error) {
	switch kind {
	case "php":
		return "logs/php-error.log", nil
	case "access", "error":
		if err := system.Domain(domainName); err != nil {
			return "", err
		}
		var n int
		if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM domains WHERE account_id = ? AND name = ?`, a.ID, domainName).Scan(&n); err != nil {
			return "", err
		}
		if n == 0 {
			return "", store.ErrNotFound
		}
		return domainName + "." + kind + ".log", nil
	}
	return "", errBadRequest("kind must be access, error or php")
}

// openLogJail opens the directory a path from logFile is relative to: the
// home for PHP's log, the root-owned log directory for the web server's.
func openLogJail(a *acct, rel string) (*files.FS, error) {
	if strings.HasPrefix(rel, "logs/") {
		return files.Open(a.HomeDir, a.SysUID, a.SysUID)
	}
	return files.Open(provision.LogDir(a.prov()), -1, -1)
}

// tailFile reads up to max bytes from the end of a log.
func tailFile(a *acct, rel string, max int64) ([]byte, error) {
	jail, err := openLogJail(a, rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer jail.Close()
	f, info, err := jail.OpenRead(rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	if info.Size() > max {
		if _, err := f.Seek(info.Size()-max, io.SeekStart); err != nil {
			return nil, err
		}
	}
	data, err := io.ReadAll(io.LimitReader(f, max))
	if err == nil && info.Size() > max {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	return data, err
}

type hit struct {
	IP      string `json:"ip"`
	Time    int64  `json:"time"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	Status  int    `json:"status"`
	Bytes   int64  `json:"bytes"`
	Referer string `json:"referer"`
	Agent   string `json:"agent"`
}

var reCombined = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) (\S+)[^"]*" (\d{3}) (\d+|-) "([^"]*)" "([^"]*)"`)

func parseAccessLog(data []byte) []hit {
	var out []hit
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		m := reCombined.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		t, err := time.Parse("02/Jan/2006:15:04:05 -0700", m[2])
		if err != nil {
			continue
		}
		st, _ := strconv.Atoi(m[5])
		b, _ := strconv.ParseInt(m[6], 10, 64)
		ref := m[7]
		if ref == "-" {
			ref = ""
		}
		out = append(out, hit{IP: m[1], Time: t.Unix(), Method: m[3], Path: m[4], Status: st, Bytes: b, Referer: ref, Agent: m[8]})
	}
	return out
}

// demoAccessLog fakes a few days of traffic for dry-run installs.
func demoAccessLog(domain string, days int) []byte {
	paths := []string{"/", "/about", "/contact", "/blog/hello-world", "/wp-content/themes/site/style.css", "/images/logo.png", "/shop", "/wp-login.php", "/feed", "/favicon.ico"}
	agents := []string{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/130.0", "Mozilla/5.0 (iPhone; CPU iPhone OS 18_1) Safari/604.1", "Mozilla/5.0 (compatible; Googlebot/2.1)", "Mozilla/5.0 (Macintosh) Firefox/132.0"}
	var b strings.Builder
	now := time.Now()
	n := 0
	for d := days - 1; d >= 0; d-- {
		perDay := 40 + (d*37)%35
		for i := 0; i < perDay; i++ {
			n++
			t := now.Add(-time.Duration(d)*24*time.Hour - time.Duration((perDay-i)*97)*time.Second)
			p := paths[(n*7)%len(paths)]
			st := 200
			if p == "/wp-login.php" {
				st = 403
			} else if n%23 == 0 {
				st = 404
				p = "/old-page"
			}
			ip := fmt.Sprintf("203.0.113.%d", (n*13)%200+1)
			size := 1800 + (n*733)%90000
			ref := "-"
			if n%3 == 0 {
				ref = "https://www.google.com/"
			}
			fmt.Fprintf(&b, "%s - - [%s] \"GET %s HTTP/2.0\" %d %d \"%s\" \"%s\"\n", ip, t.Format("02/Jan/2006:15:04:05 -0700"), p, st, size, ref, agents[n%len(agents)])
		}
	}
	_ = domain
	return []byte(b.String())
}

func (s *Server) accessLogOf(a *acct, domain string, max int64) []byte {
	data, err := tailFile(a, domain+".access.log", max)
	if (err != nil || len(data) == 0) && s.cfg.DryRun {
		return demoAccessLog(domain, 14)
	}
	return data
}

func (s *Server) visitors(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	dom := r.URL.Query().Get("domain")
	if _, err := s.logFile(r.Context(), a, "access", dom); err != nil {
		return err
	}
	hits := parseAccessLog(s.accessLogOf(a, dom, 2<<20))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	// Summary over everything read, list of the newest hits.
	pages, ips, refs := map[string]int{}, map[string]int{}, map[string]int{}
	statuses := map[string]int{}
	for _, h := range hits {
		pages[h.Path]++
		ips[h.IP]++
		if h.Referer != "" {
			refs[h.Referer]++
		}
		statuses[strconv.Itoa(h.Status/100)+"xx"]++
	}
	recent := []hit{}
	for i := len(hits) - 1; i >= 0 && len(recent) < limit; i-- {
		recent = append(recent, hits[i])
	}
	writeJSON(w, 200, map[string]any{
		"hits": recent, "total": len(hits), "unique_ips": len(ips),
		"top_pages": topN(pages, 10), "top_ips": topN(ips, 10), "top_referrers": topN(refs, 10), "statuses": statuses,
	})
	return nil
}

type countRow struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

func topN(m map[string]int, n int) []countRow {
	out := make([]countRow, 0, len(m))
	for k, v := range m {
		out = append(out, countRow{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func (s *Server) bandwidth(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	doms, err := s.domainsOf(r.Context(), a.ID)
	if err != nil {
		return err
	}
	type day struct {
		Day   string `json:"day"`
		Bytes int64  `json:"bytes"`
		Hits  int    `json:"hits"`
	}
	type dv struct {
		Domain string `json:"domain"`
		Bytes  int64  `json:"bytes"`
		Hits   int    `json:"hits"`
		Days   []day  `json:"days"`
	}
	out := []dv{}
	for _, d := range doms {
		if d.Kind == "alias" {
			continue
		}
		byDay := map[string]*day{}
		x := dv{Domain: d.Name, Days: []day{}}
		for _, h := range parseAccessLog(s.accessLogOf(a, d.Name, 8<<20)) {
			k := time.Unix(h.Time, 0).UTC().Format("2006-01-02")
			if byDay[k] == nil {
				byDay[k] = &day{Day: k}
			}
			byDay[k].Bytes += h.Bytes
			byDay[k].Hits++
			x.Bytes += h.Bytes
			x.Hits++
		}
		for _, v := range byDay {
			x.Days = append(x.Days, *v)
		}
		sort.Slice(x.Days, func(i, j int) bool { return x.Days[i].Day < x.Days[j].Day })
		out = append(out, x)
	}
	writeJSON(w, 200, map[string]any{"domains": out, "month_used_mb": a.BWUsedMB, "limit_mb": a.pkg.BandwidthMB})
	return nil
}

func (s *Server) downloadLog(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	kind, dom := r.URL.Query().Get("kind"), r.URL.Query().Get("domain")
	rel, err := s.logFile(r.Context(), a, kind, dom)
	if err != nil {
		return err
	}
	data, err := tailFile(a, rel, 64<<20)
	if err != nil {
		return fsErr(err)
	}
	if len(data) == 0 && s.cfg.DryRun && kind == "access" {
		data = demoAccessLog(dom, 3)
	}
	name := strings.ReplaceAll(strings.TrimPrefix(rel, "logs/"), "/", "_")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Security-Policy", "sandbox")
	_, err = w.Write(data)
	return err
}

// ---- contact information -----------------------------------------------------------

type contactPrefs struct {
	Email     string          `json:"email"`
	Secondary string          `json:"secondary_email"`
	Notify    map[string]bool `json:"notify"`
}

// notifyKeys are the events a user can be emailed about, with defaults.
var notifyKeys = map[string]bool{"disk_quota": true, "bandwidth": true}

func (s *Server) contactOf(ctx context.Context, userID string) contactPrefs {
	c := contactPrefs{Notify: map[string]bool{}}
	var raw string
	if s.db.QueryRow(ctx, `SELECT value FROM user_prefs WHERE user_id = ? AND key = 'contact'`, userID).Scan(&raw) == nil {
		_ = json.Unmarshal([]byte(raw), &c)
	}
	if c.Notify == nil {
		c.Notify = map[string]bool{}
	}
	for k, def := range notifyKeys {
		if _, ok := c.Notify[k]; !ok {
			c.Notify[k] = def
		}
	}
	_ = s.db.QueryRow(ctx, `SELECT email FROM users WHERE id = ?`, userID).Scan(&c.Email)
	return c
}

func (s *Server) getContact(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, 200, s.contactOf(r.Context(), principalFrom(r.Context()).UserID))
	return nil
}

func (s *Server) putContact(w http.ResponseWriter, r *http.Request) error {
	var req contactPrefs
	if err := decode(r, &req); err != nil {
		return err
	}
	uid := principalFrom(r.Context()).UserID
	for _, e := range []string{req.Email, req.Secondary} {
		if e == "" {
			continue
		}
		if _, err := mail.ParseAddress(e); err != nil || len(e) > 254 {
			return errBadRequest(e + " is not a valid address")
		}
	}
	if req.Email == "" {
		return errBadRequest("a contact address is required")
	}
	clean := contactPrefs{Secondary: req.Secondary, Notify: map[string]bool{}}
	for k := range notifyKeys {
		clean.Notify[k] = req.Notify[k]
	}
	raw, _ := json.Marshal(clean)
	if _, err := s.db.Exec(r.Context(), `UPDATE users SET email = ? WHERE id = ?`, req.Email, uid); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM user_prefs WHERE user_id = ? AND key = 'contact'`, uid); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO user_prefs (user_id, key, value) VALUES (?, 'contact', ?)`, uid, string(raw)); err != nil {
		return err
	}
	s.audit(r, "profile.contact", "", true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// sendNotice emails a plain-text notice from the server's contact address.
func (s *Server) sendNotice(ctx context.Context, to []string, subject, body string) {
	from := s.setting(ctx, "contact_email")
	if from == "" {
		host := s.setting(ctx, "hostname")
		if host == "" {
			host = "localhost"
		}
		from = "noreply@" + host
	}
	for _, addr := range to {
		if addr == "" {
			continue
		}
		if _, err := mail.ParseAddress(addr); err != nil {
			continue
		}
		msg := "From: " + from + "\r\nTo: " + addr + "\r\nSubject: " + subject +
			"\r\nContent-Type: text/plain; charset=utf-8\r\nAuto-Submitted: auto-generated\r\n\r\n" + body + "\r\n"
		if _, err := s.prov.Runner().RunInput(ctx, []byte(msg), "/usr/sbin/sendmail", "-i", "--", addr); err != nil {
			s.log.Warn("notice mail", "err", err)
		}
	}
}

// quotaNotices warns account holders at 80% of disk or bandwidth, at most
// once a day per kind, if they kept the notification switched on.
func (s *Server) quotaNotices(ctx context.Context) {
	rows, err := s.db.Query(ctx, `SELECT a.user_id, u.username, a.disk_used_mb, k.disk_mb, a.bw_used_mb, k.bandwidth_mb FROM accounts a JOIN users u ON u.id = a.user_id JOIN packages k ON k.id = a.package_id WHERE a.status = 'active'`)
	if err != nil {
		return
	}
	type row struct {
		uid, user                string
		disk, diskMax, bw, bwMax int
	}
	var list []row
	for rows.Next() {
		var x row
		if rows.Scan(&x.uid, &x.user, &x.disk, &x.diskMax, &x.bw, &x.bwMax) == nil {
			list = append(list, x)
		}
	}
	rows.Close()
	now := time.Now().Unix()
	for _, x := range list {
		c := s.contactOf(ctx, x.uid)
		for _, k := range []struct {
			key       string
			used, max int
			what      string
		}{{"disk_quota", x.disk, x.diskMax, "disk space"}, {"bandwidth", x.bw, x.bwMax, "monthly bandwidth"}} {
			if !c.Notify[k.key] || k.max <= 0 || k.used*100 < k.max*80 {
				continue
			}
			mark := "notified:" + k.key
			var last string
			_ = s.db.QueryRow(ctx, `SELECT value FROM user_prefs WHERE user_id = ? AND key = ?`, x.uid, mark).Scan(&last)
			if t, _ := strconv.ParseInt(last, 10, 64); now-t < 86400 {
				continue
			}
			s.sendNotice(ctx, []string{c.Email, c.Secondary}, "Your account "+x.user+" is using "+strconv.Itoa(k.used*100/k.max)+"% of its "+k.what,
				fmt.Sprintf("The hosting account %s is using %d MB of %d MB %s.\n\nDelete unused files or ask your provider for a larger plan before the limit is reached.\n\nYou can turn these emails off under Contact Information in cPanel.", x.user, k.used, k.max, k.what))
			_, _ = s.db.Exec(ctx, `DELETE FROM user_prefs WHERE user_id = ? AND key = ?`, x.uid, mark)
			_, _ = s.db.Exec(ctx, `INSERT INTO user_prefs (user_id, key, value) VALUES (?,?,?)`, x.uid, mark, strconv.FormatInt(now, 10))
		}
	}
}
