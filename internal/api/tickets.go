package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/store"
	"github.com/breefercp/breefercp/internal/system"
)

// Support tickets. A customer's ticket goes to whoever owns the customer
// (its reseller, or root). Resellers can also open tickets with their own
// owner. Staff see tickets addressed to them; root sees all.

func (s *Server) ticketRoutes(mux *http.ServeMux) {
	s.route(mux, "GET /api/v1/tickets", rbac.ProfileWrite, s.listTickets)
	s.route(mux, "POST /api/v1/tickets", rbac.ProfileWrite, s.createTicket)
	s.route(mux, "GET /api/v1/tickets/{id}", rbac.ProfileWrite, s.getTicket)
	s.route(mux, "POST /api/v1/tickets/{id}/replies", rbac.ProfileWrite, s.replyTicket)
	s.route(mux, "PATCH /api/v1/tickets/{id}", rbac.ProfileWrite, s.updateTicket)
}

type ticket struct {
	ID         string `json:"id"`
	Number     int    `json:"number"`
	Subject    string `json:"subject"`
	Department string `json:"department"`
	Priority   string `json:"priority"`
	Status     string `json:"status"`
	UserID     string `json:"user_id"`
	Customer   string `json:"customer"`
	Mine       bool   `json:"mine"` // opened by the caller (vs. addressed to the caller)
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
}

type ticketMessage struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	Staff     bool   `json:"staff"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"created_at"`
}

const ticketSelect = `SELECT t.id, t.number, t.subject, t.department, t.priority, t.status, t.user_id, u.username, t.owner_id, t.created_at, t.updated_at
	FROM tickets t JOIN users u ON u.id = t.user_id`

func scanTicket(row rowScanner, me string) (ticket, string, error) {
	var t ticket
	var owner string
	err := row.Scan(&t.ID, &t.Number, &t.Subject, &t.Department, &t.Priority, &t.Status, &t.UserID, &t.Customer, &owner, &t.CreatedAt, &t.UpdatedAt)
	t.Mine = t.UserID == me
	return t, owner, store.IsNoRows(err)
}

// visibleTicket loads a ticket the caller opened, answers, or (root) any.
func (s *Server) visibleTicket(r *http.Request) (ticket, bool, error) {
	p := principalFrom(r.Context())
	t, owner, err := scanTicket(s.db.QueryRow(r.Context(), ticketSelect+` WHERE t.id = ?`, r.PathValue("id")), p.UserID)
	if err != nil {
		return t, false, err
	}
	staff := p.Role == rbac.Root || owner == p.UserID
	if !staff && t.UserID != p.UserID {
		return t, false, store.ErrNotFound
	}
	return t, staff && t.UserID != p.UserID, nil
}

func (s *Server) listTickets(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	q, args := ticketSelect, []any{}
	switch p.Role {
	case rbac.Root:
	case rbac.Reseller:
		q += ` WHERE (t.owner_id = ? OR t.user_id = ?)`
		args = append(args, p.UserID, p.UserID)
	default:
		q += ` WHERE t.user_id = ?`
		args = append(args, p.UserID)
	}
	if st := r.URL.Query().Get("status"); st == "open" {
		if len(args) == 0 {
			q += ` WHERE`
		} else {
			q += ` AND`
		}
		q += ` t.status <> 'closed'`
	}
	rows, err := s.db.Query(r.Context(), q+` ORDER BY t.updated_at DESC LIMIT 500`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []ticket{}
	for rows.Next() {
		t, _, err := scanTicket(rows, p.UserID)
		if err != nil {
			return err
		}
		out = append(out, t)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

func validTicketText(subject, body string) error {
	if err := system.OneLine(strings.TrimSpace(subject), 160); err != nil {
		return errBadRequest("subject is required (max 160 characters, one line)")
	}
	if strings.TrimSpace(body) == "" || len(body) > 20000 {
		return errBadRequest("message is required (max 20,000 characters)")
	}
	return nil
}

func (s *Server) createTicket(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Subject    string `json:"subject"`
		Department string `json:"department"`
		Priority   string `json:"priority"`
		Body       string `json:"body"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if err := validTicketText(req.Subject, req.Body); err != nil {
		return err
	}
	switch req.Priority {
	case "":
		req.Priority = "normal"
	case "low", "normal", "high", "urgent":
	default:
		return errBadRequest("invalid priority")
	}
	switch req.Department {
	case "":
		req.Department = "support"
	case "support", "billing", "sales", "abuse":
	default:
		return errBadRequest("invalid department")
	}
	p := principalFrom(r.Context())
	var owner string
	if err := s.db.QueryRow(r.Context(), `SELECT COALESCE(owner_id,'') FROM users WHERE id = ?`, p.UserID).Scan(&owner); err != nil {
		return err
	}
	if owner == "" {
		return errBadRequest("administrators cannot open tickets with themselves")
	}
	id, now := ids.New(), time.Now().Unix()
	err := s.db.Tx(r.Context(), func(tx *store.Tx) error {
		var n int
		if err := tx.QueryRow(r.Context(), `SELECT COALESCE(MAX(number), 1000) + 1 FROM tickets`).Scan(&n); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO tickets (id, number, user_id, owner_id, subject, department, priority, status, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,'open',?,?)`, id, n, p.UserID, owner, strings.TrimSpace(req.Subject), req.Department, req.Priority, now, now); err != nil {
			return err
		}
		_, err := tx.Exec(r.Context(), `INSERT INTO ticket_messages (id, ticket_id, author_id, author, staff, body, created_at) VALUES (?,?,?,?,0,?,?)`,
			ids.Ordered(), id, p.UserID, p.Username, req.Body, now)
		return err
	})
	if err != nil {
		return err
	}
	s.audit(r, "ticket.create", req.Subject, true, "")
	t, _, err := scanTicket(s.db.QueryRow(r.Context(), ticketSelect+` WHERE t.id = ?`, id), p.UserID)
	if err != nil {
		return err
	}
	writeJSON(w, 201, t)
	return nil
}

func (s *Server) getTicket(w http.ResponseWriter, r *http.Request) error {
	t, staff, err := s.visibleTicket(r)
	if err != nil {
		return err
	}
	rows, err := s.db.Query(r.Context(), `SELECT id, author, staff, body, created_at FROM ticket_messages WHERE ticket_id = ? ORDER BY created_at, id`, t.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	msgs := []ticketMessage{}
	for rows.Next() {
		var m ticketMessage
		var st int
		if err := rows.Scan(&m.ID, &m.Author, &st, &m.Body, &m.CreatedAt); err != nil {
			return err
		}
		m.Staff = st == 1
		msgs = append(msgs, m)
	}
	writeJSON(w, 200, map[string]any{"ticket": t, "messages": msgs, "can_answer": staff})
	return rows.Err()
}

func (s *Server) replyTicket(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Body string `json:"body"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Body) == "" || len(req.Body) > 20000 {
		return errBadRequest("message is required (max 20,000 characters)")
	}
	t, staff, err := s.visibleTicket(r)
	if err != nil {
		return err
	}
	p := principalFrom(r.Context())
	now := time.Now().Unix()
	status := "customer_reply"
	if staff {
		status = "answered"
	}
	if _, err := s.db.Exec(r.Context(), `INSERT INTO ticket_messages (id, ticket_id, author_id, author, staff, body, created_at) VALUES (?,?,?,?,?,?,?)`,
		ids.Ordered(), t.ID, p.UserID, p.Username, b2i(staff), req.Body, now); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE tickets SET status = ?, updated_at = ? WHERE id = ?`, status, now, t.ID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) updateTicket(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Status   *string `json:"status"`
		Priority *string `json:"priority"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	t, staff, err := s.visibleTicket(r)
	if err != nil {
		return err
	}
	if req.Status != nil {
		// Customers may only close or reopen their own tickets.
		switch *req.Status {
		case "closed", "open":
		case "answered":
			if !staff {
				return errForbidden()
			}
		default:
			return errBadRequest("invalid status")
		}
		t.Status = *req.Status
	}
	if req.Priority != nil {
		switch *req.Priority {
		case "low", "normal", "high", "urgent":
			t.Priority = *req.Priority
		default:
			return errBadRequest("invalid priority")
		}
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE tickets SET status = ?, priority = ?, updated_at = ? WHERE id = ?`,
		t.Status, t.Priority, time.Now().Unix(), t.ID); err != nil {
		return err
	}
	writeJSON(w, 200, t)
	return nil
}
