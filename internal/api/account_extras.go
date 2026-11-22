package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/files"
	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/provision"
	"github.com/breefercp/breefercp/internal/rbac"
)

func (s *Server) extraRoutes(mux *http.ServeMux) {
	const a = "/api/v1/accounts/{aid}"
	r, w := rbac.HostingRead, rbac.HostingWrite

	s.route(mux, "PUT "+a+"/shell", rbac.AccountsWrite, s.setShell)
	s.route(mux, "GET "+a+"/disk-usage", r, s.diskUsage)

	s.route(mux, "GET "+a+"/php", r, s.getPHP)
	s.route(mux, "PUT "+a+"/php", w, s.putPHP)

	s.route(mux, "GET "+a+"/domains/{id}/redirects", r, s.listRedirects)
	s.route(mux, "POST "+a+"/domains/{id}/redirects", w, s.addRedirect)
	s.route(mux, "DELETE "+a+"/domains/{id}/redirects/{rid}", w, s.deleteRedirect)

	s.route(mux, "GET "+a+"/domains/{id}/error-pages", r, s.listErrorPages)
	s.route(mux, "PUT "+a+"/domains/{id}/error-pages/{code}", w, s.putErrorPage)
	s.route(mux, "DELETE "+a+"/domains/{id}/error-pages/{code}", w, s.deleteErrorPage)

	s.route(mux, "GET "+a+"/logs", r, s.readLog)
}

// ---- shell access & disk ----------------------------------------------------

func (s *Server) setShell(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Mode string `json:"mode"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if !provision.ValidShell(req.Mode) {
		return errBadRequest("mode must be none, sftp or full")
	}
	if req.Mode == provision.ShellFull && !a.pkg.ShellAccess {
		return errConflict("this account's package does not allow shell access")
	}
	if err := s.prov.SetShell(r.Context(), a.prov(), req.Mode); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE accounts SET shell = ?, updated_at = ? WHERE id = ?`, req.Mode, time.Now().Unix(), a.ID); err != nil {
		return err
	}
	s.audit(r, "account.shell", a.SysUser, true, req.Mode)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) diskUsage(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	dirs, err := s.prov.DiskUsage(r.Context(), a.prov())
	if err != nil {
		return err
	}
	var dbMB int
	_ = s.db.QueryRow(r.Context(), `SELECT COALESCE(SUM(size_mb),0) FROM db_instances WHERE account_id = ?`, a.ID).Scan(&dbMB)
	if dirs == nil {
		dirs = []provision.DirUsage{}
	}
	writeJSON(w, 200, map[string]any{"directories": dirs, "databases_mb": dbMB, "limit_mb": a.pkg.DiskMB, "used_mb": a.DiskUsedMB})
	return nil
}

// ---- PHP manager ------------------------------------------------------------

func (s *Server) getPHP(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	installed := s.prov.InstalledPHPVersions()
	allowed := map[string]bool{}
	for _, v := range strings.Split(a.pkg.PHPVersions, ",") {
		allowed[strings.TrimSpace(v)] = true
	}
	type ver struct {
		Version   string `json:"version"`
		Installed bool   `json:"installed"`
		Allowed   bool   `json:"allowed"`
	}
	seen := map[string]bool{}
	var versions []ver
	for _, v := range installed {
		versions = append(versions, ver{v, true, allowed[v]})
		seen[v] = true
	}
	for v := range allowed {
		if !seen[v] {
			versions = append(versions, ver{v, false, true})
		}
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].Version > versions[j].Version })
	keys := make([]string, 0, len(provision.PHPIniKeys))
	for k := range provision.PHPIniKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	writeJSON(w, 200, map[string]any{
		"versions": versions, "default_version": firstPHP(a.pkg.PHPVersions),
		"ini": a.prov().PHPIni, "defaults": provision.PHPIniDefaults, "keys": keys,
	})
	return nil
}

func (s *Server) putPHP(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Ini map[string]string `json:"ini"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	// Drop entries equal to the default so the pool only carries real overrides.
	for k, v := range req.Ini {
		if v == "" || provision.PHPIniDefaults[k] == v {
			delete(req.Ini, k)
		}
	}
	clean, err := provision.CleanPHPIni(req.Ini)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(clean)
	a.phpIniJSON = string(raw)
	// Apply to every PHP version in use by the account's domains.
	doms, err := s.domainsOf(r.Context(), a.ID)
	if err != nil {
		return err
	}
	done := map[string]bool{}
	for _, d := range doms {
		if done[d.PHP] {
			continue
		}
		done[d.PHP] = true
		pa := a.prov()
		pa.PHP = d.PHP
		if err := s.prov.ApplyPHPPool(r.Context(), pa); err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM php_settings WHERE account_id = ?`, a.ID); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO php_settings (account_id, settings) VALUES (?, ?)`, a.ID, string(raw)); err != nil {
		return err
	}
	s.audit(r, "php.ini", a.SysUser, true, string(raw))
	writeJSON(w, 200, map[string]any{"ini": clean})
	return nil
}

// ---- redirects ----------------------------------------------------------------

type redirectRow struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Code   int    `json:"code"`
}

func (s *Server) redirectsOf(ctx context.Context, domainID string) ([]provision.Redirect, error) {
	rows, err := s.db.Query(ctx, `SELECT source, target, code FROM redirects WHERE domain_id = ? ORDER BY source`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []provision.Redirect
	for rows.Next() {
		var r provision.Redirect
		if err := rows.Scan(&r.Source, &r.Target, &r.Code); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Server) listRedirects(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	d, err := s.domainOf(r.Context(), a.ID, r.PathValue("id"))
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, source, target, code FROM redirects WHERE domain_id = ? ORDER BY source`, d.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []redirectRow{}
	for rows.Next() {
		var v redirectRow
		if err := rows.Scan(&v.ID, &v.Source, &v.Target, &v.Code); err != nil {
			return err
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) applyDomain(ctx context.Context, a *acct, d domain) error {
	v, err := s.vhostFor(ctx, d)
	if err != nil {
		return err
	}
	if a.Status != "active" {
		return nil // suspended vhosts are rewritten on reactivation
	}
	return s.prov.WriteVhost(ctx, v, a.prov())
}

func (s *Server) addRedirect(w http.ResponseWriter, r *http.Request) error {
	var req redirectRow
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
	if req.Code == 0 {
		req.Code = 301
	}
	if err := provision.ValidateRedirect(provision.Redirect{Source: req.Source, Target: req.Target, Code: req.Code}); err != nil {
		return err
	}
	req.ID = ids.New()
	if _, err := s.db.Exec(r.Context(), `INSERT INTO redirects (id, domain_id, source, target, code, created_at) VALUES (?,?,?,?,?,?)`,
		req.ID, d.ID, req.Source, req.Target, req.Code, time.Now().Unix()); err != nil {
		return err
	}
	if err := s.applyDomain(r.Context(), a, d); err != nil {
		_, _ = s.db.Exec(context.WithoutCancel(r.Context()), `DELETE FROM redirects WHERE id = ?`, req.ID)
		return err
	}
	s.audit(r, "redirect.add", d.Name+req.Source, true, req.Target)
	writeJSON(w, 201, req)
	return nil
}

func (s *Server) deleteRedirect(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	d, err := s.domainOf(r.Context(), a.ID, r.PathValue("id"))
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM redirects WHERE id = ? AND domain_id = ?`, r.PathValue("rid"), d.ID); err != nil {
		return err
	}
	if err := s.applyDomain(r.Context(), a, d); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- custom error pages -------------------------------------------------------

const maxErrorPage = 64 << 10

func parseCodes(list string) []int {
	var out []int
	for _, p := range strings.Split(list, ",") {
		if c, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
			out = append(out, c)
		}
	}
	return out
}

func joinCodes(codes []int) string {
	sort.Ints(codes)
	parts := make([]string, len(codes))
	for i, c := range codes {
		parts[i] = strconv.Itoa(c)
	}
	return strings.Join(parts, ",")
}

func errorPageCode(r *http.Request) (int, error) {
	c, err := strconv.Atoi(r.PathValue("code"))
	if err != nil {
		return 0, errBadRequest("invalid status code")
	}
	for _, x := range provision.ErrorCodes {
		if x == c {
			return c, nil
		}
	}
	return 0, errBadRequest("custom pages are supported for 400, 401, 403, 404, 500, 502 and 503")
}

// errorPagePath is the jail-relative location of a page.
func errorPagePath(domain string, code int) string {
	return path.Join("error_pages", domain, strconv.Itoa(code)+".html")
}

func (s *Server) listErrorPages(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	d, err := s.domainOf(r.Context(), a.ID, r.PathValue("id"))
	if err != nil {
		return err
	}
	var list string
	if err := s.db.QueryRow(r.Context(), `SELECT error_pages FROM domains WHERE id = ?`, d.ID).Scan(&list); err != nil {
		return err
	}
	configured := map[int]bool{}
	for _, c := range parseCodes(list) {
		configured[c] = true
	}
	jail, err := files.Open(a.HomeDir, a.SysUID, a.SysUID)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	type page struct {
		Code    int    `json:"code"`
		Custom  bool   `json:"custom"`
		Content string `json:"content"`
	}
	out := make([]page, 0, len(provision.ErrorCodes))
	for _, c := range provision.ErrorCodes {
		pg := page{Code: c, Custom: configured[c]}
		if pg.Custom && jail != nil {
			if b, err := jail.ReadText(errorPagePath(d.Name, c)); err == nil {
				pg.Content = string(b)
			}
		}
		out = append(out, pg)
	}
	if jail != nil {
		jail.Close()
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) setErrorPages(ctx context.Context, a *acct, d domain, codes []int) error {
	if _, err := s.db.Exec(ctx, `UPDATE domains SET error_pages = ? WHERE id = ?`, joinCodes(codes), d.ID); err != nil {
		return err
	}
	return s.applyDomain(ctx, a, d)
}

func (s *Server) putErrorPage(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Content string `json:"content"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if len(req.Content) == 0 || len(req.Content) > maxErrorPage {
		return errBadRequest("page content must be between 1 byte and 64 KB")
	}
	code, err := errorPageCode(r)
	if err != nil {
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
	jail, err := files.Open(a.HomeDir, a.SysUID, a.SysUID)
	if err != nil {
		return fsErr(err)
	}
	defer jail.Close()
	if err := jail.Mkdir(path.Join("error_pages", d.Name)); err != nil {
		return fsErr(err)
	}
	if err := jail.WriteFile(errorPagePath(d.Name, code), []byte(req.Content)); err != nil {
		return fsErr(err)
	}
	var list string
	_ = s.db.QueryRow(r.Context(), `SELECT error_pages FROM domains WHERE id = ?`, d.ID).Scan(&list)
	codes := parseCodes(list)
	present := false
	for _, c := range codes {
		present = present || c == code
	}
	if !present {
		codes = append(codes, code)
	}
	if err := s.setErrorPages(r.Context(), a, d, codes); err != nil {
		return err
	}
	s.audit(r, "errorpage.set", d.Name, true, strconv.Itoa(code))
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) deleteErrorPage(w http.ResponseWriter, r *http.Request) error {
	code, err := errorPageCode(r)
	if err != nil {
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
	var list string
	_ = s.db.QueryRow(r.Context(), `SELECT error_pages FROM domains WHERE id = ?`, d.ID).Scan(&list)
	var keep []int
	for _, c := range parseCodes(list) {
		if c != code {
			keep = append(keep, c)
		}
	}
	if err := s.setErrorPages(r.Context(), a, d, keep); err != nil {
		return err
	}
	if jail, err := files.Open(a.HomeDir, a.SysUID, a.SysUID); err == nil {
		_ = jail.Remove(errorPagePath(d.Name, code))
		jail.Close()
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- logs ---------------------------------------------------------------------

const maxLogTail = 256 << 10

// readLog returns the last lines of a domain's access or error log, or of the
// account's PHP error log, read through the file jail.
func (s *Server) readLog(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	kind := r.URL.Query().Get("kind")
	domainName := r.URL.Query().Get("domain")
	rel, err := s.logFile(r.Context(), a, kind, domainName)
	if err != nil {
		return err
	}
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	if lines <= 0 || lines > 2000 {
		lines = 200
	}
	jail, err := openLogJail(a, rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeJSON(w, 200, map[string]any{"lines": []string{}})
			return nil
		}
		return fsErr(err)
	}
	defer jail.Close()
	f, info, err := jail.OpenRead(rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeJSON(w, 200, map[string]any{"lines": []string{}})
			return nil
		}
		return fsErr(err)
	}
	defer f.Close()
	if info.Size() > maxLogTail {
		if _, err := f.Seek(info.Size()-maxLogTail, io.SeekStart); err != nil {
			return err
		}
	}
	data, err := io.ReadAll(io.LimitReader(f, maxLogTail))
	if err != nil {
		return err
	}
	all := strings.Split(strings.TrimRight(string(bytes.ToValidUTF8(data, []byte("?"))), "\n"), "\n")
	if info.Size() > maxLogTail && len(all) > 0 {
		all = all[1:] // first line is probably partial
	}
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	if len(all) == 1 && all[0] == "" {
		all = []string{}
	}
	writeJSON(w, 200, map[string]any{"lines": all})
	return nil
}
