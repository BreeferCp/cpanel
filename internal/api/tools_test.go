package api

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMailingListsAndAddressCollisions(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)
	aid := setupCustomer(t, e, admin, "")
	carla := e.login("carla", "Strong-pass-carla-9")
	var doms []domain
	e.do(carla, "GET", "/api/v1/accounts/me/domains", nil, &doms)
	if len(doms) == 0 {
		t.Fatal("no domains")
	}
	body := map[string]any{"local_part": "team", "domain_id": doms[0].ID, "members": []string{"A@example.org", "b@example.org", "a@example.org"}}
	var l mailingList
	if code := e.do(carla, "POST", "/api/v1/accounts/me/mailing-lists", body, &l); code != 201 {
		t.Fatalf("create list: %d", code)
	}
	if len(l.Members) != 2 || l.Members[0] != "a@example.org" {
		t.Fatalf("members not normalized: %v", l.Members)
	}
	if code := e.do(carla, "POST", "/api/v1/accounts/me/mailing-lists", body, nil); code != 409 {
		t.Fatalf("duplicate list: want 409, got %d", code)
	}
	bad := map[string]any{"local_part": "x", "domain_id": doms[0].ID, "members": []string{"not an address"}}
	if code := e.do(carla, "POST", "/api/v1/accounts/me/mailing-lists", bad, nil); code != 400 {
		t.Fatalf("bad member: want 400, got %d", code)
	}
	// A list's address cannot be pointed at a domain the caller does not own.
	if code := e.do(carla, "POST", "/api/v1/accounts/me/domain-forwards", map[string]any{"domain_id": "nope", "target": "example.org"}, nil); code != 400 {
		t.Fatalf("foreign domain forward: want 400, got %d", code)
	}
	_ = aid
}

func TestMailFiltersValidateAndTest(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)
	setupCustomer(t, e, admin, "")
	carla := e.login("carla", "Strong-pass-carla-9")
	f := map[string]any{"mailbox_id": "", "name": "Invoices", "field": "subject", "op": "contains", "value": "invoice", "action": "fileinto", "dest": "Billing"}
	if code := e.do(carla, "POST", "/api/v1/accounts/me/mail-filters", f, nil); code != 201 {
		t.Fatalf("create filter: %d", code)
	}
	f["dest"] = `../../etc"; discard;`
	if code := e.do(carla, "POST", "/api/v1/accounts/me/mail-filters", f, nil); code != 400 {
		t.Fatalf("folder injection: want 400, got %d", code)
	}
	var res struct {
		Result map[string]any `json:"result"`
	}
	e.do(carla, "POST", "/api/v1/accounts/me/mail-filters/test", map[string]any{"subject": "Your INVOICE #42"}, &res)
	if res.Result["action"] != "fileinto" || res.Result["dest"] != "Billing" {
		t.Fatalf("filter test: %v", res.Result)
	}
	e.do(carla, "POST", "/api/v1/accounts/me/mail-filters/test", map[string]any{"subject": "hello"}, &res)
	if res.Result["action"] != "deliver" {
		t.Fatalf("non-matching: %v", res.Result)
	}
}

func TestWebToolsValidation(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)
	setupCustomer(t, e, admin, "")
	carla := e.login("carla", "Strong-pass-carla-9")
	for _, m := range []map[string]any{{"ext": "php", "type": "text/plain"}, {"ext": "x; }", "type": "text/plain"}, {"ext": "foo", "type": "text/plain; charset"}} {
		if code := e.do(carla, "POST", "/api/v1/accounts/me/mime-types", m, nil); code != 400 {
			t.Fatalf("mime %v: want 400, got %d", m, code)
		}
	}
	if code := e.do(carla, "POST", "/api/v1/accounts/me/mime-types", map[string]any{"ext": "webmanifest", "type": "application/manifest+json"}, nil); code != 201 {
		t.Fatalf("valid mime: %d", code)
	}
	if code := e.do(carla, "PUT", "/api/v1/accounts/me/optimize", map[string]any{"mode": "custom", "types": []string{"text/css; gzip off"}}, nil); code != 400 {
		t.Fatalf("gzip injection: want 400, got %d", code)
	}
	if code := e.do(carla, "PUT", "/api/v1/accounts/me/optimize", map[string]any{"mode": "all"}, nil); code != 204 {
		t.Fatalf("gzip all: %d", code)
	}
}

func TestDynamicDNSTokenScope(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)
	setupCustomer(t, e, admin, "")
	carla := e.login("carla", "Strong-pass-carla-9")
	if code := e.do(carla, "POST", "/api/v1/accounts/me/ddns", map[string]any{"name": "home", "zone": "someone-else.com"}, nil); code != 400 {
		t.Fatalf("foreign zone: want 400, got %d", code)
	}
	var created struct {
		Token string `json:"token"`
	}
	if code := e.do(carla, "POST", "/api/v1/accounts/me/ddns", map[string]any{"name": "home", "zone": "carla-site.com"}, &created); code != 201 || created.Token == "" {
		t.Fatalf("create ddns: %d", code)
	}
	get := func(q string) (int, string) {
		resp, err := http.Get(e.srv.URL + "/api/v1/ddns/update?" + q)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, body := get("token=wrong"); code != 401 || !strings.HasPrefix(body, "badauth") {
		t.Fatalf("bad token: %d %q", code, body)
	}
	if code, body := get("token=" + created.Token + "&ip=198.51.100.7"); code != 200 || !strings.HasPrefix(body, "good 198.51.100.7") {
		t.Fatalf("update: %d %q", code, body)
	}
	if _, body := get("token=" + created.Token + "&ip=198.51.100.7"); !strings.HasPrefix(body, "nochg") {
		t.Fatalf("repeat: %q", body)
	}
}

func TestServerToolsAreRootOnlyAndScoped(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)
	setupCustomer(t, e, admin, "")
	carla := e.login("carla", "Strong-pass-carla-9")
	for _, p := range []string{"/api/v1/server/security-advisor", "/api/v1/server/ssh", "/api/v1/server/root-keys", "/api/v1/server/domains", "/api/v1/server/backups"} {
		if code := e.do(carla, "GET", p, nil, nil); code != 403 {
			t.Fatalf("customer %s: want 403, got %d", p, code)
		}
	}
	var doms []serverDomain
	if code := e.do(admin, "GET", "/api/v1/server/domains", nil, &doms); code != 200 || len(doms) == 0 {
		t.Fatalf("root domains: %d %d", code, len(doms))
	}
	if code := e.do(admin, "POST", "/api/v1/server/root-keys", map[string]string{"public_key": `command="sh" ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl x`}, nil); code != 400 {
		t.Fatalf("key with options: want 400, got %d", code)
	}
	if code := e.do(admin, "POST", "/api/v1/server/root-password", map[string]string{"current_password": "wrong", "password": "Another-strong-pass-1"}, nil); code != 403 {
		t.Fatalf("root password without re-auth: want 403, got %d", code)
	}
	if code := e.do(admin, "PUT", "/api/v1/server/time", map[string]string{"timezone": "Mars/Base; rm"}, nil); code != 400 {
		t.Fatalf("bad tz: want 400, got %d", code)
	}
}
