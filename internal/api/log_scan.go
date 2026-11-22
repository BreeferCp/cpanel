package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/files"
	"github.com/breefercp/breefercp/internal/provision"
	"github.com/breefercp/breefercp/internal/store"
)

// The access logs are read incrementally for two jobs: monthly bandwidth, and
// Leech Protection (one folder login used from too many addresses).

const (
	scanChunk   = 16 << 20
	scanPerRun  = 512 << 20
	leechWindow = 2 * 60 * 60 // seconds
)

var reLogLine = regexp.MustCompile(`^(\S+) \S+ (\S+) \[([^\]]+)\] "([^"]*)" (\d{3}) (\d+|-)`)

type logEntry struct {
	ip, user, target string
	at               time.Time
	status           int
	bytes            int64
}

func parseLogEntry(line string) (logEntry, bool) {
	m := reLogLine.FindStringSubmatch(line)
	if m == nil {
		return logEntry{}, false
	}
	at, err := time.Parse("02/Jan/2006:15:04:05 -0700", m[3])
	if err != nil {
		return logEntry{}, false
	}
	e := logEntry{ip: m[1], user: m[2], at: at}
	e.status, _ = strconv.Atoi(m[5])
	e.bytes, _ = strconv.ParseInt(m[6], 10, 64)
	if f := strings.Fields(m[4]); len(f) >= 2 {
		e.target = f[1]
	}
	return e, true
}

type leechDir struct {
	id, path string
	limit    int
}

// protects reports whether a logged request was an authenticated one in the
// folder. nginx logs whatever user name a client sends, including where no
// password is checked, so only successful requests inside the folder count.
// Otherwise anyone could get somebody else's login blocked.
func (d leechDir) protects(e logEntry) bool {
	if e.user == "-" || e.user == "" || e.status < 200 || e.status > 299 {
		return false
	}
	raw, _, _ := strings.Cut(e.target, "?")
	p, err := url.PathUnescape(raw)
	if err != nil || !strings.HasPrefix(p, "/") {
		return false
	}
	clean := path.Clean(p)
	if strings.HasSuffix(p, "/") {
		clean += "/"
	}
	return strings.HasPrefix(clean, strings.TrimSuffix(d.path, "/")+"/")
}

func (s *Server) scanLogs(ctx context.Context) {
	if s.cfg.DryRun {
		return
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM accounts WHERE status = 'active'`)
	if err != nil {
		s.log.Error("log scan", "err", err)
		return
	}
	var list []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			list = append(list, id)
		}
	}
	rows.Close()
	for _, id := range list {
		a, err := scanAcct(s.db.QueryRow(ctx, acctSelect+` WHERE a.id = ?`, id))
		if err == nil {
			err = s.scanAccountLogs(ctx, a, provision.LogDir(a.prov()))
		}
		if err != nil {
			s.log.Error("log scan", "account", id, "err", err)
		}
	}
}

func (s *Server) scanAccountLogs(ctx context.Context, a *acct, dir string) error {
	doms, err := s.domainsOf(ctx, a.ID)
	if err != nil {
		return err
	}
	jail, err := files.Open(dir, -1, -1)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if jail != nil {
		defer jail.Close()
		for _, d := range doms {
			if err := s.scanDomainLog(ctx, jail, a.ID, d); err != nil {
				return fmt.Errorf("%s: %w", d.Name, err)
			}
		}
	}
	// Written even without logs, so the number resets when the month changes.
	var total int64
	if err := s.db.QueryRow(ctx, `SELECT COALESCE(SUM(bytes), 0) FROM bw_monthly WHERE account_id = ? AND month = ?`,
		a.ID, time.Now().UTC().Format("2006-01")).Scan(&total); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `UPDATE accounts SET bw_used_mb = ? WHERE id = ?`, total>>20, a.ID); err != nil {
		return err
	}
	return s.enforceLeech(ctx, a)
}

func (s *Server) scanDomainLog(ctx context.Context, jail *files.FS, accountID string, d domain) error {
	f, info, err := jail.OpenRead(d.Name + ".access.log")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	var offset int64
	var head string
	err = s.db.QueryRow(ctx, `SELECT log_offset, head FROM log_cursors WHERE domain_id = ?`, d.ID).Scan(&offset, &head)
	if err != nil && !isMissing(err) {
		return err
	}
	// Rotation is recognised by the first line changing, or the file shrinking.
	first := make([]byte, 512)
	n, err := f.ReadAt(first, 0)
	if err != nil && err != io.EOF {
		return err
	}
	i := bytes.IndexByte(first[:n], '\n')
	if i < 0 && n < len(first) {
		return nil // no complete line yet
	}
	if i < 0 {
		i = n
	}
	sum := sha256.Sum256(first[:i])
	if cur := hex.EncodeToString(sum[:]); cur != head || offset > info.Size() {
		head, offset = cur, 0
	}

	var dirs []leechDir
	rows, err := s.db.Query(ctx, `SELECT id, path, leech_limit FROM protected_dirs WHERE domain_id = ? AND leech_limit > 0`, d.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var l leechDir
		if err := rows.Scan(&l.id, &l.path, &l.limit); err != nil {
			rows.Close()
			return err
		}
		dirs = append(dirs, l)
	}
	rows.Close()

	recent := time.Now().Unix() - leechWindow
	for read := int64(0); read < scanPerRun && offset < info.Size(); {
		buf := make([]byte, min(int64(scanChunk), info.Size()-offset))
		n, err := f.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			return err
		}
		end := bytes.LastIndexByte(buf[:n], '\n')
		if end < 0 {
			if n < scanChunk {
				break // a line still being written
			}
			end = n - 1 // one absurd line: skip it
		}
		byMonth := map[string]int64{}
		type seen struct{ dir, user, ip string }
		hits := map[seen]int64{}
		for _, line := range strings.Split(string(buf[:end]), "\n") {
			e, ok := parseLogEntry(line)
			if !ok {
				continue
			}
			byMonth[e.at.UTC().Format("2006-01")] += e.bytes
			if e.at.Unix() < recent {
				continue
			}
			for _, l := range dirs {
				if l.protects(e) {
					hits[seen{l.id, e.user, e.ip}] = e.at.Unix()
				}
			}
		}
		offset += int64(end) + 1
		read += int64(end) + 1
		// Counts and cursor move together, so a crash cannot double count.
		err = s.db.Tx(ctx, func(tx *store.Tx) error {
			for month, b := range byMonth {
				if _, err := tx.Exec(ctx, `INSERT INTO bw_monthly (account_id, month, bytes) VALUES (?,?,?)
					ON CONFLICT (account_id, month) DO UPDATE SET bytes = bw_monthly.bytes + excluded.bytes`, accountID, month, b); err != nil {
					return err
				}
			}
			for h, at := range hits {
				if _, err := tx.Exec(ctx, `INSERT INTO leech_hits (dir_id, username, ip, at) VALUES (?,?,?,?)
					ON CONFLICT (dir_id, username, ip) DO UPDATE SET at = excluded.at`, h.dir, h.user, h.ip, at); err != nil {
					return err
				}
			}
			_, err := tx.Exec(ctx, `INSERT INTO log_cursors (domain_id, log_offset, head) VALUES (?,?,?)
				ON CONFLICT (domain_id) DO UPDATE SET log_offset = excluded.log_offset, head = excluded.head`, d.ID, offset, head)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// enforceLeech blocks folder logins seen from more addresses than allowed.
func (s *Server) enforceLeech(ctx context.Context, a *acct) error {
	rows, err := s.db.Query(ctx, `SELECT h.dir_id, h.username, d.name, p.path
		FROM leech_hits h
		JOIN protected_dirs p ON p.id = h.dir_id
		JOIN domains d ON d.id = p.domain_id
		JOIN protected_dir_users u ON u.dir_id = h.dir_id AND u.username = h.username
		WHERE d.account_id = ? AND h.at >= ? AND p.leech_limit > 0 AND u.disabled = 0
		GROUP BY h.dir_id, h.username, d.name, p.path, p.leech_limit
		HAVING COUNT(*) > p.leech_limit`, a.ID, time.Now().Unix()-leechWindow)
	if err != nil {
		return err
	}
	type offender struct{ dir, user, domain, path string }
	var list []offender
	for rows.Next() {
		var o offender
		if err := rows.Scan(&o.dir, &o.user, &o.domain, &o.path); err != nil {
			rows.Close()
			return err
		}
		list = append(list, o)
	}
	rows.Close()
	for _, o := range list {
		if _, err := s.db.Exec(ctx, `UPDATE protected_dir_users SET disabled = 1 WHERE dir_id = ? AND username = ?`, o.dir, o.user); err != nil {
			return err
		}
		if err := s.writeProtectedUsers(ctx, a, o.dir); err != nil {
			// Not blocked on disk, so it must not be shown as blocked.
			_, _ = s.db.Exec(ctx, `UPDATE protected_dir_users SET disabled = 0 WHERE dir_id = ? AND username = ?`, o.dir, o.user)
			return err
		}
		c := s.contactOf(ctx, a.UserID)
		s.sendNotice(ctx, []string{c.Email, c.Secondary}, "A shared folder login was blocked on "+o.domain,
			fmt.Sprintf("The user %q of %s%s signed in from more addresses than Leech Protection allows and has been blocked.\n\nSet a new password for that user under Directory Privacy to unblock it.", o.user, o.domain, o.path))
	}
	_, err = s.db.Exec(ctx, `DELETE FROM leech_hits WHERE at < ?`, time.Now().Unix()-leechWindow)
	return err
}

func (s *Server) putLeech(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Limit int `json:"limit"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Limit < 0 || req.Limit > 1000 {
		return errBadRequest("the limit is 1 to 1000 addresses, or 0 for off")
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	id := r.PathValue("id")
	if err := s.protectedOf(r.Context(), a.ID, id); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE protected_dirs SET leech_limit = ? WHERE id = ?`, req.Limit, id); err != nil {
		return err
	}
	s.audit(r, "web.leech", id, true, strconv.Itoa(req.Limit))
	w.WriteHeader(http.StatusNoContent)
	return nil
}
