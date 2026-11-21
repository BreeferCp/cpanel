package api

import (
	"context"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/files"
	"github.com/breefercp/breefercp/internal/provision"
	"github.com/breefercp/breefercp/internal/rbac"
)

func (s *Server) mailFilterRoutes(mux *http.ServeMux) {
	a := "/api/v1/accounts/{aid}"
	s.route(mux, "GET "+a+"/autoresponders", rbac.HostingRead, s.listAutoresponders)
	s.route(mux, "PUT "+a+"/autoresponders/{mid}", rbac.HostingWrite, s.putAutoresponder)
	s.route(mux, "DELETE "+a+"/autoresponders/{mid}", rbac.HostingWrite, s.deleteAutoresponder)
	s.route(mux, "GET "+a+"/spam", rbac.HostingRead, s.getSpam)
	s.route(mux, "PUT "+a+"/spam", rbac.HostingWrite, s.putSpam)
}

type autoresponderView struct {
	MailboxID    string `json:"mailbox_id"`
	Address      string `json:"address"`
	Subject      string `json:"subject"`
	Body         string `json:"body"`
	IntervalDays int    `json:"interval_days"`
	StartsOn     string `json:"starts_on"`
	EndsOn       string `json:"ends_on"`
}

type spamView struct {
	Enabled   bool     `json:"enabled"`
	Threshold int      `json:"threshold"`
	Action    string   `json:"action"`
	Allow     []string `json:"allow"`
	Deny      []string `json:"deny"`
}

func (s *Server) spamPolicy(ctx context.Context, accountID string) spamView {
	v := spamView{Threshold: 5, Action: "junk", Allow: []string{}, Deny: []string{}}
	var en int
	var allow, deny string
	if err := s.db.QueryRow(ctx, `SELECT enabled, threshold, action, allow, deny FROM spam_settings WHERE account_id = ?`, accountID).
		Scan(&en, &v.Threshold, &v.Action, &allow, &deny); err == nil {
		v.Enabled, v.Allow, v.Deny = en == 1, strings.Fields(allow), strings.Fields(deny)
	}
	return v
}

// syncSieve rewrites the managed Sieve script of every mailbox in the
// account (mailboxID "" ) or of one mailbox. Scripts are written through the
// home-directory jail, so a symlink planted by the customer cannot redirect
// the write.
func (s *Server) syncSieve(ctx context.Context, a *acct, mailboxID string) error {
	q := `SELECT m.id, m.local_part, d.name, ar.subject, ar.body, ar.interval_days, ar.starts_on, ar.ends_on
		FROM mailboxes m JOIN domains d ON d.id = m.domain_id LEFT JOIN autoresponders ar ON ar.mailbox_id = m.id
		WHERE m.account_id = ?`
	args := []any{a.ID}
	if mailboxID != "" {
		q += ` AND m.id = ?`
		args = append(args, mailboxID)
	}
	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return err
	}
	type box struct {
		local, domain string
		ar            *provision.Autoresponder
	}
	var boxes []box
	for rows.Next() {
		var b box
		var id string
		var subj, body, start, end *string
		var days *int
		if err := rows.Scan(&id, &b.local, &b.domain, &subj, &body, &days, &start, &end); err != nil {
			rows.Close()
			return err
		}
		if subj != nil {
			b.ar = &provision.Autoresponder{Subject: *subj, Body: *body, IntervalDays: *days, StartsOn: *start, EndsOn: *end}
		}
		boxes = append(boxes, b)
	}
	rows.Close()
	sp := s.spamPolicy(ctx, a.ID)
	policy := provision.SpamPolicy{Enabled: sp.Enabled, Threshold: sp.Threshold, Action: sp.Action, Allow: sp.Allow, Deny: sp.Deny}
	jail, err := files.Open(a.HomeDir, a.SysUID, a.SysUID)
	if err != nil {
		return err
	}
	defer jail.Close()
	for _, b := range boxes {
		dir := path.Join("mail", b.domain, b.local)
		file := path.Join(dir, ".dovecot.sieve")
		if !policy.Enabled && b.ar == nil {
			_ = jail.Remove(file)
			continue
		}
		script, err := provision.RenderSieve(policy, b.ar)
		if err != nil {
			return err
		}
		if err := jail.Mkdir(dir); err != nil {
			return err
		}
		if err := jail.WriteFile(file, []byte(script)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) listAutoresponders(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT m.id, m.address, ar.subject, ar.body, ar.interval_days, ar.starts_on, ar.ends_on
		FROM autoresponders ar JOIN mailboxes m ON m.id = ar.mailbox_id WHERE m.account_id = ? ORDER BY m.address`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []autoresponderView{}
	for rows.Next() {
		var v autoresponderView
		if err := rows.Scan(&v.MailboxID, &v.Address, &v.Subject, &v.Body, &v.IntervalDays, &v.StartsOn, &v.EndsOn); err != nil {
			return err
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) mailboxOf(ctx context.Context, accountID, id string) (string, error) {
	var addr string
	err := s.db.QueryRow(ctx, `SELECT address FROM mailboxes WHERE id = ? AND account_id = ?`, id, accountID).Scan(&addr)
	if err != nil {
		return "", errNotFound()
	}
	return addr, nil
}

func (s *Server) putAutoresponder(w http.ResponseWriter, r *http.Request) error {
	var req autoresponderView
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	mid := r.PathValue("mid")
	addr, err := s.mailboxOf(r.Context(), a.ID, mid)
	if err != nil {
		return err
	}
	if req.IntervalDays == 0 {
		req.IntervalDays = 1
	}
	ar := provision.Autoresponder{Subject: strings.TrimSpace(req.Subject), Body: req.Body, IntervalDays: req.IntervalDays,
		StartsOn: req.StartsOn, EndsOn: req.EndsOn}
	if err := provision.ValidateAutoresponder(ar); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM autoresponders WHERE mailbox_id = ?`, mid); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO autoresponders (mailbox_id, subject, body, interval_days, starts_on, ends_on, created_at)
		VALUES (?,?,?,?,?,?,?)`, mid, ar.Subject, ar.Body, ar.IntervalDays, ar.StartsOn, ar.EndsOn, time.Now().Unix()); err != nil {
		return err
	}
	if err := s.syncSieve(r.Context(), a, mid); err != nil {
		return fsErr(err)
	}
	s.audit(r, "mail.autoresponder.set", addr, true, "")
	req.MailboxID, req.Address = mid, addr
	writeJSON(w, 200, req)
	return nil
}

func (s *Server) deleteAutoresponder(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	mid := r.PathValue("mid")
	addr, err := s.mailboxOf(r.Context(), a.ID, mid)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM autoresponders WHERE mailbox_id = ?`, mid); err != nil {
		return err
	}
	if err := s.syncSieve(r.Context(), a, mid); err != nil {
		return fsErr(err)
	}
	s.audit(r, "mail.autoresponder.delete", addr, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) getSpam(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	writeJSON(w, 200, s.spamPolicy(r.Context(), a.ID))
	return nil
}

func (s *Server) putSpam(w http.ResponseWriter, r *http.Request) error {
	var req spamView
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	norm := func(in []string) []string {
		out := []string{}
		seen := map[string]bool{}
		for _, x := range in {
			x = strings.ToLower(strings.TrimSpace(x))
			if x != "" && !seen[x] {
				seen[x] = true
				out = append(out, x)
			}
		}
		return out
	}
	req.Allow, req.Deny = norm(req.Allow), norm(req.Deny)
	if err := provision.ValidateSpamPolicy(provision.SpamPolicy{Enabled: true, Threshold: req.Threshold, Action: req.Action,
		Allow: req.Allow, Deny: req.Deny}); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM spam_settings WHERE account_id = ?`, a.ID); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO spam_settings (account_id, enabled, threshold, action, allow, deny) VALUES (?,?,?,?,?,?)`,
		a.ID, b2i(req.Enabled), req.Threshold, req.Action, strings.Join(req.Allow, " "), strings.Join(req.Deny, " ")); err != nil {
		return err
	}
	if err := s.syncSieve(r.Context(), a, ""); err != nil {
		return fsErr(err)
	}
	s.audit(r, "mail.spam.set", a.SysUser, true, "")
	writeJSON(w, 200, req)
	return nil
}
