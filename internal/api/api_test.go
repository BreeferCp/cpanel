package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/breefercp/breefercp/internal/auth"
	"github.com/breefercp/breefercp/internal/config"
	"github.com/breefercp/breefercp/internal/ids"
	"github.com/breefercp/breefercp/internal/provision"
	"github.com/breefercp/breefercp/internal/store"
	"github.com/breefercp/breefercp/internal/system"
)

type env struct {
	t   *testing.T
	srv *httptest.Server
	db  *store.Store
}

const adminPW = "Admin-passphrase-42"

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DryRun = true
	cfg.DBDSN = filepath.Join(dir, "t.db")
	cfg.DataDir = dir
	cfg.HomeBase = filepath.ToSlash(filepath.Join(dir, "home"))
	cfg.SecretKey = strings.Repeat("k", 40)
	cfg.WebDir = dir
	db, err := store.Open(context.Background(), "sqlite", cfg.DBDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	prov := provision.New(cfg, &system.Runner{DryRun: true, Log: log}, log)
	s := New(cfg, db, prov, nil, log, "test")
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	hash, _ := auth.HashPassword(adminPW)
	now := time.Now().Unix()
	if _, err := db.Exec(context.Background(), `INSERT INTO users (id, username, email, password_hash, role, created_at, updated_at) VALUES (?,?,?,?,'root',?,?)`,
		ids.New(), "admin", "a@example.com", hash, now, now); err != nil {
		t.Fatal(err)
	}
	return &env{t: t, srv: ts, db: db}
}

type client struct {
	e      *env
	cookie *http.Cookie
	csrf   string
	tok    string
}

func (e *env) do(c *client, method, path string, body any, out any) int {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	req.Header.Set("Content-Type", "application/json")
	if c != nil {
		if c.cookie != nil {
			req.AddCookie(c.cookie)
			req.Header.Set(csrfHeader, c.csrf)
		}
		if c.tok != "" {
			req.Header.Set("Authorization", "Bearer "+c.tok)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			e.t.Fatalf("decode %s %s: %v (%s)", method, path, err, raw)
		}
	}
	return resp.StatusCode
}

func (e *env) login(user, pw string) *client {
	e.t.Helper()
	body, _ := json.Marshal(map[string]string{"username": user, "password": pw})
	resp, err := http.Post(e.srv.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		e.t.Fatalf("login %s: status %d", user, resp.StatusCode)
	}
	var out struct {
		CSRF string `json:"csrf_token"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie {
			return &client{e: e, cookie: c, csrf: out.CSRF}
		}
	}
	e.t.Fatal("no session cookie")
	return nil
}

func TestLoginAndLockout(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < maxFailedLogins; i++ {
		if code := e.do(nil, "POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": "wrong-password"}, nil); code != 401 {
			t.Fatalf("attempt %d: want 401 got %d", i, code)
		}
	}
	if code := e.do(nil, "POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": adminPW}, nil); code != 423 {
		t.Fatalf("locked account must refuse even the right password, got %d", code)
	}
}

func TestCSRFAndAuthRequired(t *testing.T) {
	e := newEnv(t)
	if code := e.do(nil, "GET", "/api/v1/accounts", nil, nil); code != 401 {
		t.Fatalf("anonymous list: want 401 got %d", code)
	}
	c := e.login("admin", adminPW)
	bad := &client{e: e, cookie: c.cookie, csrf: "forged"}
	if code := e.do(bad, "POST", "/api/v1/packages", map[string]any{"name": "x"}, nil); code != 403 {
		t.Fatalf("forged CSRF token: want 403 got %d", code)
	}
}

var starter = map[string]any{
	"name": "Starter", "disk_mb": 5000, "bandwidth_mb": 0, "max_domains": 2, "max_databases": 1, "max_mailboxes": 5,
	"max_cron": 2, "max_apps": 1, "cpu_percent": 100, "memory_mb": 1024, "io_weight": 100, "max_procs": 200, "php_versions": "8.3",
}

func TestAccountLifecycleAndTenancy(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)

	var pk pkg
	if code := e.do(admin, "POST", "/api/v1/packages", starter, &pk); code != 201 {
		t.Fatalf("create package: %d", code)
	}
	var made []createdAccount
	for _, n := range []string{"alice", "bobby"} {
		var res createdAccount
		code := e.do(admin, "POST", "/api/v1/accounts", map[string]any{
			"username": n, "email": n + "@example.com", "domain": n + "-site.com", "package_id": pk.ID, "password": "Strong-pass-" + n + "-9"}, &res)
		if code != 201 {
			t.Fatalf("create account %s: %d", n, code)
		}
		made = append(made, res)
	}

	// Duplicate username is a conflict, not a 500.
	if code := e.do(admin, "POST", "/api/v1/accounts", map[string]any{
		"username": "alice", "email": "x@example.com", "domain": "other.com", "package_id": pk.ID, "password": "Strong-pass-zz-9"}, nil); code != 409 {
		t.Fatalf("duplicate account: want 409 got %d", code)
	}
	// Injection attempt in the domain is rejected before any config is written.
	if code := e.do(admin, "POST", "/api/v1/accounts", map[string]any{
		"username": "carol", "email": "c@example.com", "domain": "a.com;}\nserver{", "package_id": pk.ID}, nil); code != 400 {
		t.Fatalf("injected domain: want 400 got %d", code)
	}

	alice := e.login("alice", "Strong-pass-alice-9")
	// Alice reads her own account via "me" and cannot see Bob's by id.
	if code := e.do(alice, "GET", "/api/v1/accounts/me", nil, nil); code != 200 {
		t.Fatalf("own account: %d", code)
	}
	if code := e.do(alice, "GET", "/api/v1/accounts/"+made[1].Account.ID+"/domains", nil, nil); code != 404 {
		t.Fatalf("cross-tenant read must be 404, got %d", code)
	}
	if code := e.do(alice, "GET", "/api/v1/server/status", nil, nil); code != 403 {
		t.Fatalf("customer must not read server status, got %d", code)
	}
	if code := e.do(alice, "GET", "/api/v1/accounts", nil, nil); code != 403 {
		t.Fatalf("customer must not list accounts, got %d", code)
	}

	// Quota enforcement: package allows 2 domains, one is the primary.
	if code := e.do(alice, "POST", "/api/v1/accounts/me/domains", map[string]string{"name": "addon-one.com", "kind": "addon"}, nil); code != 201 {
		t.Fatalf("first addon: %d", code)
	}
	if code := e.do(alice, "POST", "/api/v1/accounts/me/domains", map[string]string{"name": "addon-two.com", "kind": "addon"}, nil); code != 422 {
		t.Fatalf("domain over quota: want 422 got %d", code)
	}
	// Document root cannot escape the home directory.
	if code := e.do(alice, "POST", "/api/v1/accounts/me/domains", map[string]string{"name": "x.example.com", "kind": "alias", "doc_root": "../../etc"}, nil); code == 201 {
		t.Fatal("doc_root escaping home must be rejected")
	}

	// Suspension blocks the customer's sessions and login.
	if code := e.do(admin, "POST", "/api/v1/accounts/"+made[0].Account.ID+"/suspend", map[string]string{"reason": "abuse"}, nil); code != 204 {
		t.Fatalf("suspend: %d", code)
	}
	if code := e.do(alice, "GET", "/api/v1/accounts/me", nil, nil); code != 401 {
		t.Fatalf("suspended session must be dead, got %d", code)
	}
}

func TestBillingAPIIdempotentAndScoped(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)
	var pk pkg
	e.do(admin, "POST", "/api/v1/packages", starter, &pk)

	var tok struct{ Token string }
	if code := e.do(admin, "POST", "/api/v1/me/tokens", map[string]any{"name": "whmcs", "scopes": []string{"billing"}}, &tok); code != 201 {
		t.Fatalf("token create: %d", code)
	}
	bill := &client{e: e, tok: tok.Token}

	req := map[string]any{"username": "dave", "email": "d@example.com", "domain": "dave-site.com", "package_id": pk.ID, "external_ref": "svc-100"}
	var first createdAccount
	if code := e.do(bill, "POST", "/api/v1/billing/accounts", req, &first); code != 201 {
		t.Fatalf("billing create: %d", code)
	}
	if first.Password == "" {
		t.Fatal("generated password must be returned once on creation")
	}
	var again createdAccount
	if code := e.do(bill, "POST", "/api/v1/billing/accounts", req, &again); code != 200 || again.Account.ID != first.Account.ID || again.Password != "" {
		t.Fatalf("replayed create must be idempotent, got code=%d", code)
	}
	if code := e.do(bill, "POST", "/api/v1/billing/accounts/svc-100/suspend", map[string]string{"reason": "unpaid"}, nil); code != 204 {
		t.Fatalf("suspend: %d", code)
	}
	if code := e.do(bill, "POST", "/api/v1/billing/accounts/svc-100/unsuspend", nil, nil); code != 204 {
		t.Fatalf("unsuspend: %d", code)
	}
	// A billing-scoped token cannot touch server administration.
	if code := e.do(bill, "GET", "/api/v1/server/audit", nil, nil); code != 403 {
		t.Fatalf("billing token on audit log: want 403 got %d", code)
	}
	if code := e.do(bill, "POST", "/api/v1/billing/accounts/svc-100/terminate", nil, nil); code != 204 {
		t.Fatalf("terminate: %d", code)
	}
	if code := e.do(bill, "POST", "/api/v1/billing/accounts/svc-100/terminate", nil, nil); code != 204 {
		t.Fatalf("terminate must be idempotent: %d", code)
	}
}

func TestSSOIsSingleUse(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)
	var pk pkg
	e.do(admin, "POST", "/api/v1/packages", starter, &pk)
	var res createdAccount
	e.do(admin, "POST", "/api/v1/accounts", map[string]any{"username": "erin", "email": "e@example.com", "domain": "erin-site.com", "package_id": pk.ID}, &res)

	var sso struct{ Token string }
	if code := e.do(admin, "POST", "/api/v1/accounts/"+res.Account.ID+"/sso", nil, &sso); code != 200 {
		t.Fatalf("sso create: %d", code)
	}
	if code := e.do(nil, "POST", "/api/v1/auth/sso", map[string]string{"token": sso.Token}, nil); code != 200 {
		t.Fatalf("sso redeem: %d", code)
	}
	if code := e.do(nil, "POST", "/api/v1/auth/sso", map[string]string{"token": sso.Token}, nil); code != 401 {
		t.Fatalf("sso replay must fail, got %d", code)
	}
	if code := e.do(nil, "POST", "/api/v1/auth/sso", map[string]string{"token": sso.Token + "x"}, nil); code != 401 {
		t.Fatalf("tampered sso token must fail, got %d", code)
	}
}

func TestMasterResellerHierarchy(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)

	// root creates a master reseller allowed one sub-reseller and two accounts.
	if code := e.do(admin, "POST", "/api/v1/resellers", map[string]any{"username": "master", "email": "m@example.com",
		"password": "Master-pass-1234", "is_master": true, "max_resellers": 1, "max_accounts": 2, "max_disk_mb": 0}, nil); code != 201 {
		t.Fatalf("create master: %d", code)
	}
	master := e.login("master", "Master-pass-1234")
	if code := e.do(master, "POST", "/api/v1/resellers", map[string]any{"username": "sub", "email": "s@example.com", "password": "Sub-pass-123456"}, nil); code != 201 {
		t.Fatalf("master creates reseller: %d", code)
	}
	if code := e.do(master, "POST", "/api/v1/resellers", map[string]any{"username": "sub2", "email": "s@example.com", "password": "Sub-pass-123456"}, nil); code != 422 {
		t.Fatalf("max_resellers must be enforced, got %d", code)
	}
	sub := e.login("sub", "Sub-pass-123456")
	if code := e.do(sub, "POST", "/api/v1/resellers", map[string]any{"username": "sub3", "email": "s@example.com", "password": "Sub-pass-123456"}, nil); code != 403 {
		t.Fatalf("plain reseller must not create resellers, got %d", code)
	}

	// The sub-reseller provisions a customer; the master sees it, a sibling does not.
	var pk pkg
	if code := e.do(sub, "POST", "/api/v1/packages", starter, &pk); code != 201 {
		t.Fatalf("sub package: %d", code)
	}
	var res createdAccount
	if code := e.do(sub, "POST", "/api/v1/accounts", map[string]any{"username": "cust1", "email": "c@example.com", "domain": "cust1.com", "package_id": pk.ID}, &res); code != 201 {
		t.Fatalf("sub creates account: %d", code)
	}
	if code := e.do(master, "GET", "/api/v1/accounts/"+res.Account.ID, nil, nil); code != 200 {
		t.Fatalf("master must see its reseller's customer, got %d", code)
	}
	if code := e.do(admin, "POST", "/api/v1/resellers", map[string]any{"username": "other", "email": "o@example.com", "password": "Other-pass-1234"}, nil); code != 201 {
		t.Fatal("create sibling reseller")
	}
	other := e.login("other", "Other-pass-1234")
	if code := e.do(other, "GET", "/api/v1/accounts/"+res.Account.ID, nil, nil); code != 404 {
		t.Fatalf("unrelated reseller must get 404, got %d", code)
	}
	var list []map[string]any
	e.do(master, "GET", "/api/v1/accounts", nil, &list)
	if len(list) != 1 {
		t.Fatalf("master account list: want 1 got %d", len(list))
	}
}

func TestResellerAllocationLimits(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)
	e.do(admin, "POST", "/api/v1/resellers", map[string]any{"username": "rsl", "email": "r@example.com", "password": "Resell-pass-123", "max_accounts": 1, "max_disk_mb": 6000}, nil)
	rs := e.login("rsl", "Resell-pass-123")
	var pk pkg
	e.do(rs, "POST", "/api/v1/packages", starter, &pk) // 5000 MB disk
	if code := e.do(rs, "POST", "/api/v1/accounts", map[string]any{"username": "one", "email": "o@example.com", "domain": "one-site.com", "package_id": pk.ID}, nil); code != 201 {
		t.Fatalf("first account: %d", code)
	}
	if code := e.do(rs, "POST", "/api/v1/accounts", map[string]any{"username": "two", "email": "t@example.com", "domain": "two-site.com", "package_id": pk.ID}, nil); code != 422 {
		t.Fatalf("max_accounts must be enforced, got %d", code)
	}
}

func TestTicketsAreIsolated(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)
	var pk pkg
	e.do(admin, "POST", "/api/v1/packages", starter, &pk)
	for _, n := range []string{"tina", "tom"} {
		e.do(admin, "POST", "/api/v1/accounts", map[string]any{"username": n, "email": n + "@example.com", "domain": n + "-site.com", "package_id": pk.ID, "password": "Strong-pass-" + n + "-9"}, nil)
	}
	tina := e.login("tina", "Strong-pass-tina-9")
	tom := e.login("tom", "Strong-pass-tom-9")
	var tk ticket
	if code := e.do(tina, "POST", "/api/v1/tickets", map[string]string{"subject": "Site down", "body": "Please help"}, &tk); code != 201 {
		t.Fatalf("open ticket: %d", code)
	}
	if code := e.do(tom, "GET", "/api/v1/tickets/"+tk.ID, nil, nil); code != 404 {
		t.Fatalf("another customer must not read the ticket, got %d", code)
	}
	if code := e.do(admin, "POST", "/api/v1/tickets/"+tk.ID+"/replies", map[string]string{"body": "Looking into it"}, nil); code != 204 {
		t.Fatalf("staff reply: %d", code)
	}
	var got struct {
		Ticket   ticket          `json:"ticket"`
		Messages []ticketMessage `json:"messages"`
	}
	e.do(tina, "GET", "/api/v1/tickets/"+tk.ID, nil, &got)
	if got.Ticket.Status != "answered" || len(got.Messages) != 2 || !got.Messages[1].Staff {
		t.Fatalf("unexpected ticket state: %+v", got)
	}
	if code := e.do(tina, "PATCH", "/api/v1/tickets/"+tk.ID, map[string]string{"status": "answered"}, nil); code != 403 {
		t.Fatalf("customer must not mark answered, got %d", code)
	}
}

func TestShellRequiresPackagePermission(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)
	var pk pkg
	e.do(admin, "POST", "/api/v1/packages", starter, &pk) // shell_access false
	var res createdAccount
	e.do(admin, "POST", "/api/v1/accounts", map[string]any{"username": "shelly", "email": "s@example.com", "domain": "shelly-site.com", "package_id": pk.ID}, &res)
	if code := e.do(admin, "PUT", "/api/v1/accounts/"+res.Account.ID+"/shell", map[string]string{"mode": "full"}, nil); code != 409 {
		t.Fatalf("full shell without package permission: want 409 got %d", code)
	}
	if code := e.do(admin, "PUT", "/api/v1/accounts/"+res.Account.ID+"/shell", map[string]string{"mode": "sftp"}, nil); code != 204 {
		t.Fatalf("sftp shell: %d", code)
	}
	if code := e.do(admin, "PUT", "/api/v1/accounts/"+res.Account.ID+"/shell", map[string]string{"mode": "root"}, nil); code != 400 {
		t.Fatalf("invalid mode: want 400 got %d", code)
	}
}

func TestPMARedeemIsLoopbackAndSingleUse(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)
	var pk pkg
	e.do(admin, "POST", "/api/v1/packages", starter, &pk)
	var res createdAccount
	e.do(admin, "POST", "/api/v1/accounts", map[string]any{"username": "dbby", "email": "d@example.com", "domain": "dbby-site.com", "package_id": pk.ID}, &res)
	base := "/api/v1/accounts/" + res.Account.ID
	if code := e.do(admin, "POST", base+"/phpmyadmin", nil, nil); code != 409 {
		t.Fatalf("no databases yet: want 409 got %d", code)
	}
	e.do(admin, "POST", base+"/databases", map[string]string{"name": "shop"}, nil)
	var out struct{ URL string }
	if code := e.do(admin, "POST", base+"/phpmyadmin", nil, &out); code != 200 {
		t.Fatalf("pma login: %d", code)
	}
	token := out.URL[strings.Index(out.URL, "token=")+6:]
	// httptest listens on loopback, so the first redeem succeeds and the second must not.
	if code := e.do(nil, "POST", "/api/v1/internal/pma/redeem", map[string]string{"token": token}, nil); code != 200 {
		t.Fatalf("redeem: %d", code)
	}
	if code := e.do(nil, "POST", "/api/v1/internal/pma/redeem", map[string]string{"token": token}, nil); code != 401 {
		t.Fatalf("second redeem must fail, got %d", code)
	}
}

func TestSuccessfulLoginsAreNotRateLimited(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 25; i++ {
		e.login("admin", adminPW) // fails the test on any non-200
	}
}
