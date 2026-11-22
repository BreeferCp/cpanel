package api

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/provision"
	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/store"
	"github.com/breefercp/breefercp/internal/system"
)

func (s *Server) emailToolRoutes(mux *http.ServeMux) {
	a := "/api/v1/accounts/{aid}"
	r, w := rbac.HostingRead, rbac.HostingWrite

	s.route(mux, "PATCH "+a+"/mailboxes/{id}", w, s.updateMailbox)

	s.route(mux, "GET "+a+"/domain-forwards", r, s.listDomainForwards)
	s.route(mux, "POST "+a+"/domain-forwards", w, s.createDomainForward)
	s.route(mux, "DELETE "+a+"/domain-forwards/{id}", w, s.deleteDomainForward)

	s.route(mux, "GET "+a+"/mailing-lists", r, s.listMailingLists)
	s.route(mux, "POST "+a+"/mailing-lists", w, s.createMailingList)
	s.route(mux, "PUT "+a+"/mailing-lists/{id}", w, s.updateMailingList)
	s.route(mux, "DELETE "+a+"/mailing-lists/{id}", w, s.deleteMailingList)

	s.route(mux, "GET "+a+"/email-routing", r, s.listEmailRouting)
	s.route(mux, "PUT "+a+"/email-routing/{id}", w, s.putEmailRouting)

	s.route(mux, "GET "+a+"/default-address", r, s.listDefaultAddress)
	s.route(mux, "PUT "+a+"/default-address/{id}", w, s.putDefaultAddress)

	s.route(mux, "GET "+a+"/mail-filters", r, s.listMailRules)
	s.route(mux, "POST "+a+"/mail-filters", w, s.createMailRule)
	s.route(mux, "DELETE "+a+"/mail-filters/{id}", w, s.deleteMailRule)
	s.route(mux, "POST "+a+"/mail-filters/order", w, s.orderMailRules)
	s.route(mux, "POST "+a+"/mail-filters/test", r, s.testMailRules)

	s.route(mux, "GET "+a+"/mail-delivery", r, s.trackDelivery)
	s.route(mux, "GET "+a+"/email-deliverability", r, s.deliverability)
	s.route(mux, "POST "+a+"/email-deliverability/{zone}/repair", w, s.repairDeliverability)
}

// mailDomains are the account's domains that can receive mail.
func (s *Server) mailDomains(ctx context.Context, accountID string) ([]domain, error) {
	all, err := s.domainsOf(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, d := range all {
		if d.Kind == "primary" || d.Kind == "addon" {
			out = append(out, d)
		}
	}
	return out, nil
}

// addressTaken reports whether an address already exists as a mailbox,
// forwarder or list anywhere on the server.
func (s *Server) addressTaken(ctx context.Context, addr string) (bool, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM mailboxes WHERE address = ?) + (SELECT COUNT(*) FROM mail_aliases WHERE source = ?) + (SELECT COUNT(*) FROM mailing_lists WHERE address = ?)`,
		addr, addr, addr).Scan(&n)
	return n > 0, err
}

// notAList refuses an address that is already a mailing list.
func (s *Server) notAList(ctx context.Context, addr string) error {
	var n int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM mailing_lists WHERE address = ?`, addr).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return errConflict("a mailing list already uses that address")
	}
	return nil
}

// ---- mailbox settings -------------------------------------------------------

func (s *Server) updateMailbox(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		QuotaMB *int  `json:"quota_mb"`
		Active  *bool `json:"active"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	var addr string
	if err := s.db.QueryRow(r.Context(), `SELECT address FROM mailboxes WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID).Scan(&addr); err != nil {
		return store.IsNoRows(err)
	}
	if req.QuotaMB != nil {
		if *req.QuotaMB < 1 || *req.QuotaMB > 1<<20 {
			return errBadRequest("quota must be between 1 MB and 1 TB")
		}
		if _, err := s.db.Exec(r.Context(), `UPDATE mailboxes SET quota_mb = ? WHERE id = ?`, *req.QuotaMB, r.PathValue("id")); err != nil {
			return err
		}
	}
	if req.Active != nil {
		if _, err := s.db.Exec(r.Context(), `UPDATE mailboxes SET active = ? WHERE id = ?`, b2i(*req.Active), r.PathValue("id")); err != nil {
			return err
		}
	}
	s.audit(r, "mailbox.update", addr, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- domain forwarders --------------------------------------------------------

func (s *Server) listDomainForwards(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, domain, target, created_at FROM mail_domain_forwards WHERE account_id = ? ORDER BY domain`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type v struct {
		ID        string `json:"id"`
		Domain    string `json:"domain"`
		Target    string `json:"target"`
		CreatedAt int64  `json:"created_at"`
	}
	out := []v{}
	for rows.Next() {
		var x v
		if err := rows.Scan(&x.ID, &x.Domain, &x.Target, &x.CreatedAt); err != nil {
			return err
		}
		out = append(out, x)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func (s *Server) createDomainForward(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		DomainID string `json:"domain_id"`
		Target   string `json:"target"`
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
	target := strings.ToLower(strings.TrimSpace(req.Target))
	if err := system.Domain(target); err != nil {
		return errBadRequest("enter the domain to forward to, such as example.com")
	}
	if target == d.Name {
		return errBadRequest("a domain cannot forward to itself")
	}
	id := ids.New()
	if _, err := s.db.Exec(r.Context(), `INSERT INTO mail_domain_forwards (id, account_id, domain, target, created_at) VALUES (?,?,?,?,?)`,
		id, a.ID, d.Name, target, time.Now().Unix()); err != nil {
		if store.IsUnique(err) {
			return &apiError{409, "conflict", d.Name + " already has a domain forwarder"}
		}
		return err
	}
	s.audit(r, "mail.domain_forward", d.Name, true, target)
	writeJSON(w, 201, map[string]string{"id": id})
	return nil
}

func (s *Server) deleteDomainForward(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM mail_domain_forwards WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- mailing lists --------------------------------------------------------------

type mailingList struct {
	ID          string   `json:"id"`
	Address     string   `json:"address"`
	Description string   `json:"description"`
	Members     []string `json:"members"`
	CreatedAt   int64    `json:"created_at"`
}

func cleanMembers(in []string, self string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, m := range in {
		m = strings.ToLower(strings.TrimSpace(m))
		if m == "" || seen[m] {
			continue
		}
		if !provision.ValidAddress(m) {
			return nil, errBadRequest(m + " is not a valid address")
		}
		if m == self {
			return nil, errBadRequest("a list cannot contain its own address")
		}
		seen[m] = true
		out = append(out, m)
	}
	if len(out) > 1000 {
		return nil, errBadRequest("a list can have at most 1000 members")
	}
	sort.Strings(out)
	return out, nil
}

func validDescription(d string) (string, error) {
	d = strings.TrimSpace(d)
	if len(d) > 200 || strings.ContainsAny(d, "\r\n") {
		return "", errBadRequest("the description is one line of up to 200 characters")
	}
	return d, nil
}

func (s *Server) listMailingLists(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, address, description, members, created_at FROM mailing_lists WHERE account_id = ? ORDER BY address`, a.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []mailingList{}
	for rows.Next() {
		var l mailingList
		var members string
		if err := rows.Scan(&l.ID, &l.Address, &l.Description, &members, &l.CreatedAt); err != nil {
			return err
		}
		l.Members = splitList(members)
		out = append(out, l)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func splitList(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) createMailingList(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		LocalPart   string   `json:"local_part"`
		DomainID    string   `json:"domain_id"`
		Description string   `json:"description"`
		Members     []string `json:"members"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	local := strings.ToLower(strings.TrimSpace(req.LocalPart))
	if err := system.LocalPart(local); err != nil {
		return err
	}
	d, err := s.domainOf(r.Context(), a.ID, req.DomainID)
	if err != nil {
		return errBadRequest("choose one of your domains")
	}
	addr := local + "@" + d.Name
	if taken, err := s.addressTaken(r.Context(), addr); err != nil {
		return err
	} else if taken {
		return &apiError{409, "conflict", addr + " is already a mailbox, forwarder or list"}
	}
	members, err := cleanMembers(req.Members, addr)
	if err != nil {
		return err
	}
	desc, err := validDescription(req.Description)
	if err != nil {
		return err
	}
	l := mailingList{ID: ids.New(), Address: addr, Description: desc, Members: members, CreatedAt: time.Now().Unix()}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO mailing_lists (id, account_id, address, description, members, created_at) VALUES (?,?,?,?,?,?)`,
		l.ID, a.ID, l.Address, l.Description, strings.Join(members, ","), l.CreatedAt); err != nil {
		return err
	}
	s.audit(r, "mail.list_create", addr, true, "")
	writeJSON(w, 201, l)
	return nil
}

func (s *Server) updateMailingList(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Description string   `json:"description"`
		Members     []string `json:"members"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	var addr string
	if err := s.db.QueryRow(r.Context(), `SELECT address FROM mailing_lists WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID).Scan(&addr); err != nil {
		return store.IsNoRows(err)
	}
	members, err := cleanMembers(req.Members, addr)
	if err != nil {
		return err
	}
	desc, err := validDescription(req.Description)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE mailing_lists SET description = ?, members = ? WHERE id = ?`, desc, strings.Join(members, ","), r.PathValue("id")); err != nil {
		return err
	}
	s.audit(r, "mail.list_update", addr, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) deleteMailingList(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM mailing_lists WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- email routing ------------------------------------------------------------

func (s *Server) listEmailRouting(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	doms, err := s.mailDomains(r.Context(), a.ID)
	if err != nil {
		return err
	}
	type v struct {
		ID      string   `json:"id"`
		Name    string   `json:"name"`
		Routing string   `json:"routing"`
		MX      []string `json:"mx"`
	}
	out := []v{}
	for _, d := range doms {
		x := v{ID: d.ID, Name: d.Name, MX: []string{}}
		if err := s.db.QueryRow(r.Context(), `SELECT mail_routing FROM domains WHERE id = ?`, d.ID).Scan(&x.Routing); err != nil {
			return err
		}
		if recs, err := s.prov.DNS().ListRecords(r.Context(), d.Name); err == nil {
			for _, rec := range recs {
				if rec.Type == "MX" && strings.EqualFold(strings.TrimSuffix(rec.Name, "."), d.Name) {
					x.MX = append(x.MX, rec.Content)
				}
			}
		}
		out = append(out, x)
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) putEmailRouting(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Routing string `json:"routing"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Routing != "local" && req.Routing != "remote" {
		return errBadRequest("routing must be local or remote")
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	d, err := s.domainOf(r.Context(), a.ID, r.PathValue("id"))
	if err != nil {
		return err
	}
	// Postfix reads this column through domains.cf: a remote domain is not a
	// local mailbox domain, so mail for it is relayed to its MX instead.
	if _, err := s.db.Exec(r.Context(), `UPDATE domains SET mail_routing = ? WHERE id = ?`, req.Routing, d.ID); err != nil {
		return err
	}
	s.audit(r, "mail.routing", d.Name, true, req.Routing)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- default address (catch-all) ------------------------------------------------

func (s *Server) listDefaultAddress(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	doms, err := s.mailDomains(r.Context(), a.ID)
	if err != nil {
		return err
	}
	type v struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Destination string `json:"destination"`
	}
	out := []v{}
	for _, d := range doms {
		x := v{ID: d.ID, Name: d.Name}
		err := s.db.QueryRow(r.Context(), `SELECT destination FROM mail_aliases WHERE account_id = ? AND source = ?`, a.ID, "*@"+d.Name).Scan(&x.Destination)
		if err != nil && store.IsNoRows(err) != store.ErrNotFound {
			return err
		}
		out = append(out, x)
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) putDefaultAddress(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Destination string `json:"destination"`
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
	src := "*@" + d.Name
	dest := strings.ToLower(strings.TrimSpace(req.Destination))
	if _, err := s.db.Exec(r.Context(), `DELETE FROM mail_aliases WHERE account_id = ? AND source = ?`, a.ID, src); err != nil {
		return err
	}
	if dest != "" {
		if !provision.ValidAddress(dest) {
			return errBadRequest("enter a valid address, or leave it empty to reject unknown addresses")
		}
		if _, err := s.db.Exec(r.Context(), `INSERT INTO mail_aliases (id, account_id, source, destination, created_at) VALUES (?,?,?,?,?)`,
			ids.New(), a.ID, src, dest, time.Now().Unix()); err != nil {
			return err
		}
	}
	s.audit(r, "mail.default_address", d.Name, true, dest)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ---- email filters ----------------------------------------------------------------

type mailRule struct {
	ID        string `json:"id"`
	MailboxID string `json:"mailbox_id"`
	Name      string `json:"name"`
	Field     string `json:"field"`
	Op        string `json:"op"`
	Value     string `json:"value"`
	Action    string `json:"action"`
	Dest      string `json:"dest"`
	Position  int    `json:"position"`
}

func (m mailRule) filter() provision.MailFilter {
	return provision.MailFilter{Name: m.Name, Field: m.Field, Op: m.Op, Value: m.Value, Action: m.Action, Dest: m.Dest}
}

func (s *Server) mailRules(ctx context.Context, accountID string) ([]mailRule, error) {
	rows, err := s.db.Query(ctx, `SELECT id, mailbox_id, name, field, op, value, action, dest, position FROM mail_rules WHERE account_id = ? ORDER BY position, created_at`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mailRule{}
	for rows.Next() {
		var m mailRule
		if err := rows.Scan(&m.ID, &m.MailboxID, &m.Name, &m.Field, &m.Op, &m.Value, &m.Action, &m.Dest, &m.Position); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// rulesFor returns the global rules followed by the mailbox's own rules.
func rulesFor(all []mailRule, mailboxID string) []provision.MailFilter {
	var out []provision.MailFilter
	for _, pass := range []string{"", mailboxID} {
		for _, m := range all {
			if m.MailboxID == pass {
				out = append(out, m.filter())
			}
		}
	}
	return out
}

func (s *Server) listMailRules(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rules, err := s.mailRules(r.Context(), a.ID)
	if err != nil {
		return err
	}
	writeJSON(w, 200, rules)
	return nil
}

func (s *Server) createMailRule(w http.ResponseWriter, r *http.Request) error {
	var m mailRule
	if err := decode(r, &m); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	m.Name, m.Value, m.Dest = strings.TrimSpace(m.Name), strings.TrimSpace(m.Value), strings.TrimSpace(m.Dest)
	if m.Action == "redirect" {
		m.Dest = strings.ToLower(m.Dest)
	}
	if err := provision.ValidateMailFilter(m.filter()); err != nil {
		return err
	}
	if m.MailboxID != "" {
		var n int
		if err := s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM mailboxes WHERE id = ? AND account_id = ?`, m.MailboxID, a.ID).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return errBadRequest("choose one of your mailboxes")
		}
	}
	var count int
	if err := s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM mail_rules WHERE account_id = ?`, a.ID).Scan(&count); err != nil {
		return err
	}
	if count >= 200 {
		return errBadRequest("an account can have at most 200 filters")
	}
	m.ID, m.Position = ids.New(), count
	if _, err := s.db.Exec(r.Context(), `INSERT INTO mail_rules (id, account_id, mailbox_id, name, field, op, value, action, dest, position, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		m.ID, a.ID, m.MailboxID, m.Name, m.Field, m.Op, m.Value, m.Action, m.Dest, m.Position, time.Now().Unix()); err != nil {
		return err
	}
	if err := s.syncSieve(r.Context(), a, ""); err != nil {
		return fsErr(err)
	}
	s.audit(r, "mail.filter_create", m.Name, true, "")
	writeJSON(w, 201, m)
	return nil
}

func (s *Server) deleteMailRule(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM mail_rules WHERE id = ? AND account_id = ?`, r.PathValue("id"), a.ID); err != nil {
		return err
	}
	if err := s.syncSieve(r.Context(), a, ""); err != nil {
		return fsErr(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) orderMailRules(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	for i, id := range req.IDs {
		if _, err := s.db.Exec(r.Context(), `UPDATE mail_rules SET position = ? WHERE id = ? AND account_id = ?`, i, id, a.ID); err != nil {
			return err
		}
	}
	if err := s.syncSieve(r.Context(), a, ""); err != nil {
		return fsErr(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// testMailRules runs the rules for a mailbox against a sample message
// without delivering anything, the way cPanel's filter test does.
func (s *Server) testMailRules(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		MailboxID string `json:"mailbox_id"`
		From      string `json:"from"`
		To        string `json:"to"`
		Subject   string `json:"subject"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	rules, err := s.mailRules(r.Context(), a.ID)
	if err != nil {
		return err
	}
	hdr := map[string]string{"from": req.From, "to": req.To, "subject": req.Subject}
	type step struct {
		Name    string `json:"name"`
		Matched bool   `json:"matched"`
	}
	steps := []step{}
	result := map[string]any{"action": "deliver", "dest": "Inbox"}
	// Same order as the Sieve script: global rules, then the mailbox's own.
	passes := []string{""}
	if req.MailboxID != "" {
		passes = append(passes, req.MailboxID)
	}
	for _, pass := range passes {
		for _, m := range rules {
			if m.MailboxID != pass {
				continue
			}
			ok := ruleMatches(m, hdr)
			steps = append(steps, step{m.Name, ok})
			if ok {
				result = map[string]any{"action": m.Action, "dest": m.Dest, "rule": m.Name}
				writeJSON(w, 200, map[string]any{"steps": steps, "result": result})
				return nil
			}
		}
	}
	writeJSON(w, 200, map[string]any{"steps": steps, "result": result})
	return nil
}

func ruleMatches(m mailRule, hdr map[string]string) bool {
	var vals []string
	if m.Field == "any" {
		vals = []string{hdr["from"], hdr["to"], hdr["subject"]}
	} else {
		vals = []string{hdr[m.Field]}
	}
	needle := strings.ToLower(m.Value)
	for _, v := range vals {
		v = strings.ToLower(v)
		var ok bool
		switch m.Op {
		case "contains":
			ok = strings.Contains(v, needle)
		case "not_contains":
			ok = !strings.Contains(v, needle)
		case "is":
			ok = v == needle
		case "begins":
			ok = strings.HasPrefix(v, needle)
		case "ends":
			ok = strings.HasSuffix(v, needle)
		}
		if m.Op == "not_contains" {
			if !ok {
				return false
			}
			continue
		}
		if ok {
			return true
		}
	}
	return m.Op == "not_contains"
}

// ---- track delivery -------------------------------------------------------------

type deliveryEvent struct {
	Time    string `json:"time"`
	QueueID string `json:"queue_id"`
	From    string `json:"from"`
	To      string `json:"to"`
	Status  string `json:"status"`
	Relay   string `json:"relay"`
	Detail  string `json:"detail"`
}

var (
	reMailLogLine = regexp.MustCompile(`^(\S+(?:\s+\d+\s+[\d:]+)?)\s+\S+\s+postfix/[\w/-]+\[\d+\]:\s+([0-9A-F]{6,}|[0-9A-Za-z]{10,}):\s+(.*)$`)
	reKV          = regexp.MustCompile(`(from|to|relay|status)=<?([^,>\s]*)>?`)
	reParen       = regexp.MustCompile(`\((.*)\)\s*$`)
)

// parseMailLog joins Postfix's from= and to= lines by queue id.
func parseMailLog(data []byte) []deliveryEvent {
	from := map[string]string{}
	var out []deliveryEvent
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		m := reMailLogLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		kv := map[string]string{}
		for _, p := range reKV.FindAllStringSubmatch(m[3], -1) {
			kv[p[1]] = p[2]
		}
		if f, ok := kv["from"]; ok && kv["to"] == "" {
			from[m[2]] = f
			continue
		}
		if kv["to"] == "" || kv["status"] == "" {
			continue
		}
		ev := deliveryEvent{Time: m[1], QueueID: m[2], From: from[m[2]], To: kv["to"], Status: kv["status"], Relay: kv["relay"]}
		if p := reParen.FindStringSubmatch(m[3]); p != nil {
			ev.Detail = p[1]
		}
		out = append(out, ev)
	}
	return out
}

func demoMailLog(domain string) []byte {
	now := time.Now().UTC()
	t := func(min int) string { return now.Add(-time.Duration(min) * time.Minute).Format(time.RFC3339) }
	lines := []string{
		t(95) + " mail postfix/qmgr[811]: 4F1A2B3C4D: from=<info@" + domain + ">, size=4210, nrcpt=1 (queue active)",
		t(95) + " mail postfix/smtp[902]: 4F1A2B3C4D: to=<client@example.org>, relay=mx.example.org[203.0.113.25]:25, delay=1.2, status=sent (250 2.0.0 OK queued)",
		t(60) + " mail postfix/qmgr[811]: 5A6B7C8D9E: from=<newsletter@example.net>, size=18233, nrcpt=1 (queue active)",
		t(60) + " mail postfix/lmtp[903]: 5A6B7C8D9E: to=<sales@" + domain + ">, relay=private/dovecot-lmtp, delay=0.3, status=sent (250 2.0.0 Saved)",
		t(41) + " mail postfix/qmgr[811]: 6B7C8D9E0F: from=<billing@" + domain + ">, size=3001, nrcpt=1 (queue active)",
		t(41) + " mail postfix/smtp[905]: 6B7C8D9E0F: to=<old-client@example.com>, relay=none, delay=30, status=deferred (connect to mx.example.com[198.51.100.9]:25: Connection timed out)",
		t(12) + " mail postfix/qmgr[811]: 7C8D9E0F1A: from=<info@" + domain + ">, size=2880, nrcpt=1 (queue active)",
		t(12) + " mail postfix/smtp[909]: 7C8D9E0F1A: to=<nobody@example.invalid>, relay=none, delay=0.1, status=bounced (Host or domain name not found)",
		t(3) + " mail postfix/qmgr[811]: 8D9E0F1A2B: from=<friend@example.com>, size=9120, nrcpt=1 (queue active)",
		t(3) + " mail postfix/lmtp[912]: 8D9E0F1A2B: to=<info@" + domain + ">, relay=private/dovecot-lmtp, delay=0.2, status=sent (250 2.0.0 Saved)",
	}
	return []byte(strings.Join(lines, "\n"))
}

func (s *Server) trackDelivery(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	doms, err := s.domainsOf(r.Context(), a.ID)
	if err != nil {
		return err
	}
	var data []byte
	if s.cfg.DryRun {
		data = demoMailLog(a.PrimaryDomain)
	} else {
		res, _ := s.prov.Runner().Run(r.Context(), "tail", "-n", "50000", "/var/log/mail.log")
		data = []byte(res.Stdout)
	}
	mine := func(addr string) bool {
		at := strings.LastIndexByte(addr, '@')
		if at < 0 {
			return false
		}
		dom := strings.ToLower(addr[at+1:])
		for _, d := range doms {
			if dom == d.Name {
				return true
			}
		}
		return false
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	status := r.URL.Query().Get("status")
	out := []deliveryEvent{}
	events := parseMailLog(data)
	for i := len(events) - 1; i >= 0 && len(out) < 500; i-- {
		e := events[i]
		// Only events where the account sent or received the message.
		if !mine(e.From) && !mine(e.To) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(e.From+" "+e.To), q) {
			continue
		}
		if status != "" && e.Status != status {
			continue
		}
		out = append(out, e)
	}
	writeJSON(w, 200, out)
	return nil
}

// ---- email deliverability ------------------------------------------------------------

type authCheck struct {
	Kind      string `json:"kind"`
	Status    string `json:"status"` // valid | missing | invalid
	Current   string `json:"current"`
	Suggested string `json:"suggested"`
	Name      string `json:"name"`
	Note      string `json:"note"`
}

func (s *Server) deliverabilityOf(ctx context.Context, zone string) ([]authCheck, error) {
	recs, err := s.prov.DNS().ListRecords(ctx, zone)
	if err != nil {
		return nil, err
	}
	ip := s.serverIP(ctx)
	host := s.setting(ctx, "hostname")
	if host == "" {
		host = "mail." + zone
	}
	txt := func(name string) []string {
		var out []string
		for _, r := range recs {
			if r.Type == "TXT" && strings.EqualFold(strings.TrimSuffix(r.Name, "."), name) {
				out = append(out, strings.Trim(r.Content, `"`))
			}
		}
		return out
	}
	var checks []authCheck

	spf := authCheck{Kind: "spf", Name: zone, Suggested: `"v=spf1 +a +mx +ip4:` + ip + ` ~all"`, Status: "missing"}
	for _, t := range txt(zone) {
		if strings.HasPrefix(t, "v=spf1") {
			spf.Current = t
			if strings.Contains(t, ip) || strings.Contains(t, "+a") || strings.Contains(t, " a ") || strings.Contains(t, "mx") {
				spf.Status = "valid"
			} else {
				spf.Status, spf.Note = "invalid", "the record does not authorize this server ("+ip+")"
			}
		}
	}
	checks = append(checks, spf)

	dkim := authCheck{Kind: "dkim", Name: "breefer._domainkey." + zone, Status: "missing", Note: "Repairing generates a new 2048-bit key."}
	if t := txt("breefer._domainkey." + zone); len(t) > 0 {
		dkim.Current, dkim.Status = strings.Join(t, " "), "valid"
		if !strings.Contains(dkim.Current, "p=") {
			dkim.Status, dkim.Note = "invalid", "the record has no public key"
		}
	}
	checks = append(checks, dkim)

	dmarc := authCheck{Kind: "dmarc", Name: "_dmarc." + zone, Suggested: `"v=DMARC1; p=quarantine; rua=mailto:postmaster@` + zone + `"`, Status: "missing"}
	if t := txt("_dmarc." + zone); len(t) > 0 {
		dmarc.Current = t[0]
		dmarc.Status = "valid"
		if !strings.HasPrefix(t[0], "v=DMARC1") {
			dmarc.Status, dmarc.Note = "invalid", "the record must start with v=DMARC1"
		}
	}
	checks = append(checks, dmarc)

	mx := authCheck{Kind: "mx", Name: zone, Suggested: host + ".", Status: "missing"}
	for _, r := range recs {
		if r.Type == "MX" && strings.EqualFold(strings.TrimSuffix(r.Name, "."), zone) {
			mx.Current = strings.TrimSpace(mx.Current + " " + r.Content)
			mx.Status = "valid"
		}
	}
	checks = append(checks, mx)

	ptr := authCheck{Kind: "ptr", Name: ip, Status: "missing", Note: "Reverse DNS is set by whoever owns the IP address, usually your server provider."}
	if s.cfg.DryRun {
		ptr.Status, ptr.Current = "valid", host+"."
	} else if names, err := net.DefaultResolver.LookupAddr(ctx, ip); err == nil && len(names) > 0 {
		ptr.Current = strings.Join(names, " ")
		ptr.Status = "valid"
		if !strings.EqualFold(strings.TrimSuffix(names[0], "."), host) {
			ptr.Status, ptr.Note = "invalid", "reverse DNS should be "+host+" (the server host name)"
		}
	}
	checks = append(checks, ptr)
	return checks, nil
}

func (s *Server) deliverability(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	doms, err := s.mailDomains(r.Context(), a.ID)
	if err != nil {
		return err
	}
	type v struct {
		Domain string      `json:"domain"`
		Checks []authCheck `json:"checks"`
	}
	out := []v{}
	for _, d := range doms {
		c, err := s.deliverabilityOf(r.Context(), d.Name)
		if err != nil {
			c = []authCheck{}
		}
		out = append(out, v{d.Name, c})
	}
	writeJSON(w, 200, out)
	return nil
}

func (s *Server) repairDeliverability(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Kind string `json:"kind"`
	}
	if err := decode(r, &req); err != nil {
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
	checks, err := s.deliverabilityOf(r.Context(), zone)
	if err != nil {
		return err
	}
	var c authCheck
	for _, x := range checks {
		if x.Kind == req.Kind {
			c = x
		}
	}
	dns := s.prov.DNS()
	switch req.Kind {
	case "spf", "dmarc":
		name := ""
		if req.Kind == "dmarc" {
			name = "_dmarc"
		}
		if err := dns.UpsertRecord(r.Context(), zone, provision.Record{Name: name, Type: "TXT", TTL: 3600, Content: c.Suggested}); err != nil {
			return err
		}
	case "mx":
		if err := dns.UpsertRecord(r.Context(), zone, provision.Record{Name: "", Type: "MX", TTL: 3600, Priority: 10, Content: c.Suggested}); err != nil {
			return err
		}
	case "dkim":
		k, err := s.prov.GenerateDKIM(r.Context(), zone)
		if err != nil {
			return err
		}
		if err := dns.UpsertRecord(r.Context(), zone, provision.Record{Name: k.DNSName, Type: "TXT", TTL: 3600, Content: k.DNSValue}); err != nil {
			return err
		}
	default:
		return errBadRequest("only SPF, DKIM, DMARC and MX can be repaired here")
	}
	s.audit(r, "mail.deliverability_repair", zone, true, req.Kind)
	w.WriteHeader(http.StatusNoContent)
	return nil
}
