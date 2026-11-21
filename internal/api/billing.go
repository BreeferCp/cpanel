package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/breefercp/breefercp/internal/rbac"
	"github.com/breefercp/breefercp/internal/store"
)

// The billing API is what WHMCS, Paymenter and any other billing system call
// to run the hosting lifecycle. It is authenticated with an API token that
// carries the "billing" scope, and is idempotent on external_ref so retried
// webhooks never create duplicate accounts.

func (s *Server) billingRoutes(mux *http.ServeMux) {
	const b = "/api/v1/billing"
	s.route(mux, "GET "+b+"/packages", rbac.BillingWrite, s.listPackages)
	s.route(mux, "POST "+b+"/accounts", rbac.BillingWrite, s.billingCreate)
	s.route(mux, "GET "+b+"/accounts/{ref}", rbac.BillingWrite, s.billingGet)
	s.route(mux, "POST "+b+"/accounts/{ref}/suspend", rbac.BillingWrite, s.billingSuspend)
	s.route(mux, "POST "+b+"/accounts/{ref}/unsuspend", rbac.BillingWrite, s.billingUnsuspend)
	s.route(mux, "POST "+b+"/accounts/{ref}/terminate", rbac.BillingWrite, s.billingTerminate)
	s.route(mux, "POST "+b+"/accounts/{ref}/package", rbac.BillingWrite, s.billingChangePackage)
	s.route(mux, "POST "+b+"/accounts/{ref}/password", rbac.BillingWrite, s.billingPassword)
	s.route(mux, "POST "+b+"/accounts/{ref}/sso", rbac.BillingWrite, s.billingSSO)
}

// byRef finds the account for a billing reference, constrained to what the
// calling token's owner may manage.
func (s *Server) byRef(r *http.Request) (*acct, error) {
	p := principalFrom(r.Context())
	ref := r.PathValue("ref")
	if ref == "" || len(ref) > 128 {
		return nil, errNotFound()
	}
	q := acctSelect + ` WHERE a.external_ref = ?`
	args := []any{ref}
	if p.Role == rbac.Reseller {
		q += ` AND u.owner_id = ?`
		args = append(args, p.UserID)
	}
	return scanAcct(s.db.QueryRow(r.Context(), q, args...))
}

func (s *Server) billingCreate(w http.ResponseWriter, r *http.Request) error {
	var req CreateAccountParams
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.ExternalRef == "" || len(req.ExternalRef) > 128 {
		return errBadRequest("external_ref (the billing service id) is required")
	}
	p := principalFrom(r.Context())
	// Idempotency: the same service id returns the existing account.
	q := acctSelect + ` WHERE a.external_ref = ?`
	args := []any{req.ExternalRef}
	if p.Role == rbac.Reseller {
		q += ` AND u.owner_id = ?`
		args = append(args, p.UserID)
	}
	if existing, err := scanAcct(s.db.QueryRow(r.Context(), q, args...)); err == nil {
		writeJSON(w, 200, createdAccount{Account: existing})
		return nil
	}
	res, err := s.provisionAccount(r.Context(), p, req)
	if err != nil {
		s.audit(r, "billing.create", req.ExternalRef, false, err.Error())
		return err
	}
	s.audit(r, "billing.create", req.ExternalRef, true, req.Username)
	writeJSON(w, 201, res)
	return nil
}

func (s *Server) billingGet(w http.ResponseWriter, r *http.Request) error {
	a, err := s.byRef(r)
	if err != nil {
		return err
	}
	writeJSON(w, 200, a)
	return nil
}

func (s *Server) billingSuspend(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Reason string `json:"reason"`
	}
	_ = decode(r, &req)
	a, err := s.byRef(r)
	if err != nil {
		return err
	}
	if a.Status == "suspended" {
		w.WriteHeader(http.StatusNoContent) // idempotent
		return nil
	}
	if err := s.doSuspend(r.Context(), a, req.Reason); err != nil {
		return err
	}
	s.audit(r, "billing.suspend", a.ExternalRef, true, req.Reason)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) billingUnsuspend(w http.ResponseWriter, r *http.Request) error {
	a, err := s.byRef(r)
	if err != nil {
		return err
	}
	if a.Status == "active" {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	if err := s.doUnsuspend(r.Context(), a); err != nil {
		return err
	}
	s.audit(r, "billing.unsuspend", a.ExternalRef, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) billingTerminate(w http.ResponseWriter, r *http.Request) error {
	a, err := s.byRef(r)
	if err != nil {
		if err == store.ErrNotFound {
			w.WriteHeader(http.StatusNoContent) // already gone: idempotent
			return nil
		}
		return err
	}
	if err := s.doTerminate(r.Context(), a); err != nil {
		return err
	}
	s.audit(r, "billing.terminate", a.ExternalRef, true, a.SysUser)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) billingChangePackage(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		PackageID string `json:"package_id"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.byRef(r)
	if err != nil {
		return err
	}
	pk, err := s.ownedPackage(r.Context(), principalFrom(r.Context()), req.PackageID)
	if err != nil {
		return errBadRequest("unknown package")
	}
	if _, err := s.db.Exec(r.Context(), `UPDATE accounts SET package_id = ?, updated_at = ? WHERE id = ?`, pk.ID, time.Now().Unix(), a.ID); err != nil {
		return err
	}
	a.pkg = pk
	if err := s.prov.SetLimits(r.Context(), a.prov()); err != nil {
		return err
	}
	if err := s.prov.ApplyPHPPool(r.Context(), a.prov()); err != nil {
		return err
	}
	s.audit(r, "billing.package", a.ExternalRef, true, pk.Name)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) billingPassword(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	a, err := s.byRef(r)
	if err != nil {
		return err
	}
	if err := s.setUserPassword(r.Context(), a.UserID, req.Password); err != nil {
		return err
	}
	s.audit(r, "billing.password", a.ExternalRef, true, "")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) billingSSO(w http.ResponseWriter, r *http.Request) error {
	a, err := s.byRef(r)
	if err != nil {
		return err
	}
	return s.writeSSO(w, r, a)
}

// ---- single sign-on -------------------------------------------------------------
//
// An administrator, reseller or billing system asks for a one-time login URL
// for a customer. The token is an HMAC-signed, 60-second, single-use value
// carried in the URL fragment (never sent to servers or logged).

const ssoTTL = 60 * time.Second

var ssoSeen sync.Map // nonce -> expiry (time.Time)

func (s *Server) ssoKey() []byte { return []byte(s.cfg.SecretKey) }

func (s *Server) signSSO(userID string) string {
	nonce := make([]byte, 12)
	_, _ = rand.Read(nonce)
	payload := userID + "." + strconv.FormatInt(time.Now().Add(ssoTTL).Unix(), 10) + "." + base64.RawURLEncoding.EncodeToString(nonce)
	mac := hmac.New(sha256.New, s.ssoKey())
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) verifySSO(token string) (userID string, ok bool) {
	body, sig, found := strings.Cut(token, ".")
	if !found {
		return "", false
	}
	payload, err1 := base64.RawURLEncoding.DecodeString(body)
	want, err2 := base64.RawURLEncoding.DecodeString(sig)
	if err1 != nil || err2 != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, s.ssoKey())
	mac.Write(payload)
	if !hmac.Equal(mac.Sum(nil), want) {
		return "", false
	}
	parts := strings.Split(string(payload), ".")
	if len(parts) != 3 {
		return "", false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	if _, used := ssoSeen.LoadOrStore(parts[2], time.Unix(exp, 0)); used {
		return "", false // replay
	}
	ssoSeen.Range(func(k, v any) bool { // opportunistic cleanup
		if time.Now().After(v.(time.Time)) {
			ssoSeen.Delete(k)
		}
		return true
	})
	return parts[0], true
}

func (s *Server) writeSSO(w http.ResponseWriter, r *http.Request, a *acct) error {
	if a.Status != "active" {
		return errConflict("account is suspended")
	}
	token := s.signSSO(a.UserID)
	s.audit(r, "sso.create", a.SysUser, true, "")
	writeJSON(w, 200, map[string]any{"token": token, "path": "/sso#token=" + token, "expires_in": int(ssoTTL.Seconds())})
	return nil
}

func (s *Server) createSSO(w http.ResponseWriter, r *http.Request) error {
	a, err := s.loadAccount(r)
	if err != nil {
		return err
	}
	return s.writeSSO(w, r, a)
}

func (s *Server) ssoRedeem(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Token string `json:"token"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if !s.loginLimit.allow("sso:" + s.clientIP(r)) {
		return errTooMany()
	}
	uid, ok := s.verifySSO(req.Token)
	if !ok {
		return &apiError{401, "invalid_token", "this sign-in link is invalid or has expired"}
	}
	u, err := s.userByID(r.Context(), uid)
	if err != nil || u.Suspended {
		return &apiError{401, "invalid_token", "this sign-in link is invalid or has expired"}
	}
	return s.finishLogin(w, r, u)
}

var _ = context.Background
