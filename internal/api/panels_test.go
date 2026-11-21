package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

// setupCustomer creates a package (optionally with a feature list) and one
// customer account, returning the account id.
func setupCustomer(t *testing.T, e *env, admin *client, featureList string) string {
	t.Helper()
	p := map[string]any{}
	for k, v := range starter {
		p[k] = v
	}
	p["feature_list_id"] = featureList
	var pk pkg
	if code := e.do(admin, "POST", "/api/v1/packages", p, &pk); code != 201 {
		t.Fatalf("package: %d", code)
	}
	var res createdAccount
	if code := e.do(admin, "POST", "/api/v1/accounts", map[string]any{"username": "carla", "email": "c@example.com",
		"domain": "carla-site.com", "package_id": pk.ID, "password": "Strong-pass-carla-9"}, &res); code != 201 {
		t.Fatalf("account: %d", code)
	}
	return res.Account.ID
}

func panelLogin(t *testing.T, url, user, pw string) (int, []*http.Cookie) {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"username": user, "password": pw})
	resp, err := http.Post(url+"/api/v1/auth/login", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode, resp.Cookies()
}

func TestDedicatedPanels(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)
	setupCustomer(t, e, admin, "")
	whm := httptest.NewServer(e.s.Handler(PanelWHM))
	defer whm.Close()
	cp := httptest.NewServer(e.s.Handler(PanelCPanel))
	defer cp.Close()

	// Each panel accepts only its own users, after checking the password.
	if code, _ := panelLogin(t, cp.URL, "admin", adminPW); code != 403 {
		t.Fatalf("admin on cPanel: want 403, got %d", code)
	}
	if code, _ := panelLogin(t, whm.URL, "carla", "Strong-pass-carla-9"); code != 403 {
		t.Fatalf("customer on WHM: want 403, got %d", code)
	}
	if code, _ := panelLogin(t, whm.URL, "carla", "wrong-password-1A"); code != 401 {
		t.Fatalf("wrong password must stay 401 (no panel hint), got %d", code)
	}
	code, cookies := panelLogin(t, cp.URL, "carla", "Strong-pass-carla-9")
	if code != 200 || len(cookies) == 0 || cookies[0].Name != "breefer_cpanel" {
		t.Fatalf("customer on cPanel: %d %v", code, cookies)
	}
	// A cPanel session cookie is useless on WHM, even under WHM's name.
	req, _ := http.NewRequest("GET", whm.URL+"/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: "breefer_whm", Value: cookies[0].Value})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("customer session on WHM: want 401, got %d", resp.StatusCode)
	}
}

func TestFeatureListIsEnforced(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)
	var fl featureList
	if code := e.do(admin, "POST", "/api/v1/feature-lists", map[string]any{"name": "No FTP", "disabled": []string{"ftp", "terminal"}}, &fl); code != 200 {
		t.Fatalf("feature list: %d", code)
	}
	if code := e.do(admin, "POST", "/api/v1/feature-lists", map[string]any{"name": "Bad", "disabled": []string{"rm -rf"}}, nil); code != 400 {
		t.Fatalf("unknown feature: want 400, got %d", code)
	}
	aid := setupCustomer(t, e, admin, fl.ID)
	carla := e.login("carla", "Strong-pass-carla-9")
	if code := e.do(carla, "GET", "/api/v1/accounts/me/ftp", nil, nil); code != 403 {
		t.Fatalf("disabled feature: want 403, got %d", code)
	}
	if code := e.do(carla, "GET", "/api/v1/accounts/me/domains", nil, nil); code != 200 {
		t.Fatalf("enabled feature: want 200, got %d", code)
	}
	// Administrators managing the account are not limited.
	if code := e.do(admin, "GET", "/api/v1/accounts/"+aid+"/ftp", nil, nil); code != 200 {
		t.Fatalf("admin on disabled feature: want 200, got %d", code)
	}
	// A list in use cannot be deleted.
	if code := e.do(admin, "DELETE", "/api/v1/feature-lists/"+fl.ID, nil, nil); code != 409 {
		t.Fatalf("delete used list: want 409, got %d", code)
	}
}

func TestHostAccessRules(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)
	if code := e.do(admin, "POST", "/api/v1/server/login-rules", map[string]string{"cidr": "127.0.0.1", "kind": "deny"}, nil); code != 400 {
		t.Fatalf("denying own address must be refused, got %d", code)
	}
	if code := e.do(admin, "POST", "/api/v1/server/login-rules", map[string]string{"cidr": "127.0.0.0/8", "kind": "allow", "note": "office"}, nil); code != 201 {
		t.Fatalf("allow rule: %d", code)
	}
	// Allow-listed addresses are not rate limited by failures.
	for range 15 {
		code, _ := panelLogin(t, e.srv.URL, "nobody", "Wrong-password-1")
		if code == 429 {
			t.Fatal("allow-listed address was rate limited")
		}
	}
	// The WHM allow list must include the caller.
	if code := e.do(admin, "PUT", "/api/v1/server/security", map[string]string{"require_2fa": "off", "whm_allowed_ips": "192.0.2.1"}, nil); code != 400 {
		t.Fatalf("lock-out allow list: want 400, got %d", code)
	}
	// 2FA policy cannot be switched on by an admin without 2FA.
	if code := e.do(admin, "PUT", "/api/v1/server/security", map[string]string{"require_2fa": "all"}, nil); code != 400 {
		t.Fatalf("2fa policy without own 2fa: want 400, got %d", code)
	}
}

func TestWebProtectionValidation(t *testing.T) {
	var errBody map[string]any
	e := newEnv(t)
	admin := e.login("admin", adminPW)
	setupCustomer(t, e, admin, "")
	carla := e.login("carla", "Strong-pass-carla-9")
	if code := e.do(carla, "POST", "/api/v1/accounts/me/ip-blocks", map[string]string{"cidr": "0.0.0.0/0"}, nil); code != 400 {
		t.Fatalf("blocking everything: want 400, got %d", code)
	}
	if code := e.do(carla, "POST", "/api/v1/accounts/me/ip-blocks", map[string]string{"cidr": "1.2.3.4; allow all"}, nil); code != 400 {
		t.Fatalf("injection: want 400, got %d", code)
	}
	if code := e.do(carla, "POST", "/api/v1/accounts/me/ip-blocks", map[string]string{"cidr": "203.0.113.0/24"}, nil); code != 201 {
		t.Fatalf("block range: want 201, got %d", code)
	}
	var doms []domain
	e.do(carla, "GET", "/api/v1/accounts/me/domains", nil, &doms)
	if code := e.do(carla, "POST", "/api/v1/accounts/me/protected-dirs",
		map[string]string{"domain_id": doms[0].ID, "path": "/a\"; }", "realm": "x"}, nil); code != 400 {
		t.Fatalf("protected path injection: want 400, got %d", code)
	}
	if code := e.do(carla, "POST", "/api/v1/accounts/me/ftp",
		map[string]string{"name": "up", "domain": "carla-site.com", "password": "Strong-ftp-pass-1", "directory": "../../etc"}, nil); code != 400 {
		t.Fatalf("ftp dir escape: want 400, got %d", code)
	}
	if code := e.do(carla, "POST", "/api/v1/accounts/me/ftp",
		map[string]string{"name": "up", "domain": "other.com", "password": "Strong-ftp-pass-1"}, nil); code != 400 {
		t.Fatalf("ftp on foreign domain: want 400, got %d", code)
	}
	// Homes are absolute POSIX paths on a real server; Windows temp paths
	// (C:/...) are rightly refused by the FTP database writer.
	if runtime.GOOS != "linux" {
		return
	}
	if code := e.do(carla, "POST", "/api/v1/accounts/me/ftp",
		map[string]string{"name": "up", "domain": "carla-site.com", "password": "Strong-ftp-pass-1", "directory": "public_html/uploads"}, &errBody); code != 201 {
		t.Logf("body: %v", errBody)
		t.Fatalf("ftp create: want 201, got %d", code)
	}
}
