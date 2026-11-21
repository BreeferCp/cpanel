package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/auth"
	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/store"
	"github.com/breefercp/breefercp/internal/system"
)

// Reseller hierarchy:
//
//	root ─┬─ master reseller ─┬─ reseller ── customers
//	      │                   └─ customers
//	      └─ reseller ── customers
//
// A master reseller may create resellers (up to max_resellers) and sees the
// customers of those resellers. Every reseller has allocation limits on the
// number of accounts and the disk space its packages may hand out.

type resellerView struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	OwnerID      string `json:"owner_id"`
	OwnerName    string `json:"owner_name"`
	IsMaster     bool   `json:"is_master"`
	Suspended    bool   `json:"suspended"`
	MaxAccounts  int    `json:"max_accounts"`
	MaxDiskMB    int    `json:"max_disk_mb"`
	MaxResellers int    `json:"max_resellers"`
	Accounts     int    `json:"accounts"`
	Resellers    int    `json:"resellers"`
	DiskAllocMB  int    `json:"disk_allocated_mb"`
	DiskUsedMB   int    `json:"disk_used_mb"`
	CreatedAt    int64  `json:"created_at"`
}

const resellerSelect = `SELECT u.id, u.username, u.email, COALESCE(u.owner_id,''), COALESCE(o.username,''), u.is_master, u.suspended,
	u.max_accounts, u.max_disk_mb, u.max_resellers,
	(SELECT COUNT(*) FROM accounts a JOIN users c ON c.id = a.user_id WHERE c.owner_id = u.id),
	(SELECT COUNT(*) FROM users r WHERE r.owner_id = u.id AND r.role = 'reseller'),
	(SELECT COALESCE(SUM(k.disk_mb),0) FROM accounts a JOIN users c ON c.id = a.user_id JOIN packages k ON k.id = a.package_id WHERE c.owner_id = u.id),
	(SELECT COALESCE(SUM(a.disk_used_mb),0) FROM accounts a JOIN users c ON c.id = a.user_id WHERE c.owner_id = u.id),
	u.created_at
	FROM users u LEFT JOIN users o ON o.id = u.owner_id WHERE u.role = 'reseller'`

func scanReseller(row rowScanner) (resellerView, error) {
	var v resellerView
	var master, susp int
	err := row.Scan(&v.ID, &v.Username, &v.Email, &v.OwnerID, &v.OwnerName, &master, &susp, &v.MaxAccounts, &v.MaxDiskMB,
		&v.MaxResellers, &v.Accounts, &v.Resellers, &v.DiskAllocMB, &v.DiskUsedMB, &v.CreatedAt)
	v.IsMaster, v.Suspended = master == 1, susp == 1
	return v, store.IsNoRows(err)
}

// isMaster reports whether a reseller principal may manage resellers.
func (s *Server) isMaster(ctx context.Context, p *Principal) bool {
	if p.Role == rbac.Root {
		return true
	}
	var m int
	_ = s.db.QueryRow(ctx, `SELECT is_master FROM users WHERE id = ?`, p.UserID).Scan(&m)
	return p.Role == rbac.Reseller && m == 1
}

func (s *Server) listResellers(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	if !s.isMaster(r.Context(), p) {
		return errForbidden()
	}
	q, args := resellerSelect, []any{}
	if p.Role != rbac.Root {
		q += ` AND u.owner_id = ?`
		args = append(args, p.UserID)
	}
	rows, err := s.db.Query(r.Context(), q+` ORDER BY u.username`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []resellerView{}
	for rows.Next() {
		v, err := scanReseller(rows)
		if err != nil {
			return err
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
	return rows.Err()
}

// ownedReseller loads a reseller the caller manages.
func (s *Server) ownedReseller(ctx context.Context, p *Principal, id string) (resellerView, error) {
	q, args := resellerSelect+` AND u.id = ?`, []any{id}
	if p.Role != rbac.Root {
		q += ` AND u.owner_id = ?`
		args = append(args, p.UserID)
	}
	return scanReseller(s.db.QueryRow(ctx, q, args...))
}

type resellerReq struct {
	Username     string `json:"username"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	IsMaster     bool   `json:"is_master"`
	MaxAccounts  int    `json:"max_accounts"`
	MaxDiskMB    int    `json:"max_disk_mb"`
	MaxResellers int    `json:"max_resellers"`
}

func (s *Server) createReseller(w http.ResponseWriter, r *http.Request) error {
	var req resellerReq
	if err := decode(r, &req); err != nil {
		return err
	}
	p := principalFrom(r.Context())
	if !s.isMaster(r.Context(), p) {
		return errForbidden()
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	if err := system.SysUser(req.Username); err != nil {
		return err
	}
	if !strings.Contains(req.Email, "@") {
		return errBadRequest("enter a valid email address")
	}
	if err := auth.CheckPasswordPolicy(req.Password); err != nil {
		return errBadRequest(err.Error())
	}
	if req.MaxAccounts < 0 || req.MaxDiskMB < 0 || req.MaxResellers < 0 {
		return errBadRequest("limits cannot be negative")
	}
	if p.Role != rbac.Root {
		// Only root creates master resellers; a master's resellers are plain resellers.
		req.IsMaster, req.MaxResellers = false, 0
		me, err := s.userLimits(r.Context(), p.UserID)
		if err != nil {
			return err
		}
		var n int
		if err := s.db.QueryRow(r.Context(), `SELECT COUNT(*) FROM users WHERE owner_id = ? AND role = 'reseller'`, p.UserID).Scan(&n); err != nil {
			return err
		}
		if me.maxResellers > 0 && n >= me.maxResellers {
			return errLimit("resellers")
		}
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return err
	}
	id, now := ids.New(), time.Now().Unix()
	if _, err := s.db.Exec(r.Context(), `INSERT INTO users (id, username, email, password_hash, role, owner_id, is_master, max_accounts, max_disk_mb, max_resellers, created_at, updated_at)
		VALUES (?,?,?,?,'reseller',?,?,?,?,?,?,?)`, id, req.Username, req.Email, hash, p.UserID, b2i(req.IsMaster),
		req.MaxAccounts, req.MaxDiskMB, req.MaxResellers, now, now); err != nil {
		return err
	}
	s.audit(r, "reseller.create", req.Username, true, "")
	v, err := s.ownedReseller(r.Context(), p, id)
	if err != nil {
		return err
	}
	writeJSON(w, 201, v)
	return nil
}

func (s *Server) updateReseller(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Email        *string `json:"email"`
		Password     *string `json:"password"`
		IsMaster     *bool   `json:"is_master"`
		MaxAccounts  *int    `json:"max_accounts"`
		MaxDiskMB    *int    `json:"max_disk_mb"`
		MaxResellers *int    `json:"max_resellers"`
		Suspended    *bool   `json:"suspended"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	p := principalFrom(r.Context())
	if !s.isMaster(r.Context(), p) {
		return errForbidden()
	}
	v, err := s.ownedReseller(r.Context(), p, r.PathValue("id"))
	if err != nil {
		return err
	}
	if req.Email != nil {
		if !strings.Contains(*req.Email, "@") {
			return errBadRequest("enter a valid email address")
		}
		v.Email = *req.Email
	}
	if req.IsMaster != nil && p.Role == rbac.Root {
		v.IsMaster = *req.IsMaster
	}
	for _, l := range []struct {
		in  *int
		out *int
	}{{req.MaxAccounts, &v.MaxAccounts}, {req.MaxDiskMB, &v.MaxDiskMB}, {req.MaxResellers, &v.MaxResellers}} {
		if l.in != nil {
			if *l.in < 0 {
				return errBadRequest("limits cannot be negative")
			}
			*l.out = *l.in
		}
	}
	if p.Role != rbac.Root {
		v.MaxResellers = 0
	}
	if req.Suspended != nil {
		v.Suspended = *req.Suspended
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE users SET email=?, is_master=?, max_accounts=?, max_disk_mb=?, max_resellers=?, suspended=?, updated_at=? WHERE id=?`,
		v.Email, b2i(v.IsMaster), v.MaxAccounts, v.MaxDiskMB, v.MaxResellers, b2i(v.Suspended), time.Now().Unix(), v.ID); err != nil {
		return err
	}
	if v.Suspended {
		_, _ = s.db.Exec(r.Context(), `DELETE FROM sessions WHERE user_id = ?`, v.ID)
		_, _ = s.db.Exec(r.Context(), `DELETE FROM api_tokens WHERE user_id = ?`, v.ID)
	}
	if req.Password != nil {
		if err := s.setUserPassword(r.Context(), v.ID, *req.Password); err != nil {
			return err
		}
	}
	s.audit(r, "reseller.update", v.Username, true, "")
	v, err = s.ownedReseller(r.Context(), p, v.ID)
	if err != nil {
		return err
	}
	writeJSON(w, 200, v)
	return nil
}

func (s *Server) deleteReseller(w http.ResponseWriter, r *http.Request) error {
	p := principalFrom(r.Context())
	if !s.isMaster(r.Context(), p) {
		return errForbidden()
	}
	v, err := s.ownedReseller(r.Context(), p, r.PathValue("id"))
	if err != nil {
		return err
	}
	if v.Accounts > 0 || v.Resellers > 0 {
		return errConflict("move or terminate this reseller's accounts and resellers first")
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM packages WHERE owner_id = ?`, v.ID); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM users WHERE id = ?`, v.ID); err != nil {
		return err
	}
	s.audit(r, "reseller.delete", v.Username, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type limits struct{ maxAccounts, maxDiskMB, maxResellers int }

func (s *Server) userLimits(ctx context.Context, userID string) (limits, error) {
	var l limits
	err := s.db.QueryRow(ctx, `SELECT max_accounts, max_disk_mb, max_resellers FROM users WHERE id = ?`, userID).
		Scan(&l.maxAccounts, &l.maxDiskMB, &l.maxResellers)
	return l, store.IsNoRows(err)
}

// checkResellerAllocation enforces a reseller's account and disk allocation
// before it creates an account or moves one to a larger package.
func (s *Server) checkResellerAllocation(ctx context.Context, p *Principal, addDiskMB int, newAccount bool) error {
	if p.Role != rbac.Reseller {
		return nil
	}
	l, err := s.userLimits(ctx, p.UserID)
	if err != nil {
		return err
	}
	var count, disk int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*), COALESCE(SUM(k.disk_mb),0) FROM accounts a JOIN users c ON c.id = a.user_id
		JOIN packages k ON k.id = a.package_id WHERE c.owner_id = ?`, p.UserID).Scan(&count, &disk); err != nil {
		return err
	}
	if newAccount && l.maxAccounts > 0 && count >= l.maxAccounts {
		return errLimit("accounts (reseller allocation reached)")
	}
	if l.maxDiskMB > 0 {
		if addDiskMB == 0 {
			return errBadRequest("your reseller plan requires packages with a disk limit")
		}
		if disk+addDiskMB > l.maxDiskMB {
			return errLimit("disk space (reseller allocation reached)")
		}
	}
	return nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
