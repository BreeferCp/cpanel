package api

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/breefercp/breefercp/migrations"
)

// postfixExpand evaluates the queries in deploy/postfix as virtual(5) and
// sqlite_table(5) describe: user@domain in every map, then @domain in every
// map; %u suppresses the query for an empty local part; results are recursive.
func postfixExpand(t *testing.T, e *env, addr string, seen map[string]bool) []string {
	t.Helper()
	if seen[addr] {
		return []string{addr}
	}
	seen[addr] = true
	_, dom, _ := strings.Cut(addr, "@")
	for _, key := range []string{addr, "@" + dom} {
		user, _, _ := strings.Cut(key, "@")
		for _, name := range []string{"aliases", "lists", "domain_forwards"} {
			raw, err := os.ReadFile("../../deploy/postfix/" + name + ".cf")
			if err != nil {
				t.Fatal(err)
			}
			var q string
			for _, l := range strings.Split(string(raw), "\n") {
				if v, ok := strings.CutPrefix(strings.TrimSpace(l), "query = "); ok {
					q = v
				}
			}
			if user == "" && strings.Contains(q, "%u") {
				continue
			}
			q = strings.NewReplacer("%u", user, "%d", dom, "%s", key).Replace(q)
			rows, err := e.db.Query(context.Background(), q)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			var found []string
			for rows.Next() {
				var v string
				if err := rows.Scan(&v); err != nil {
					t.Fatal(err)
				}
				found = append(found, strings.Split(v, ",")...)
			}
			rows.Close()
			if len(found) == 0 {
				continue
			}
			var out []string
			for _, f := range found {
				if f == addr {
					out = append(out, f)
				} else {
					out = append(out, postfixExpand(t, e, f, seen)...)
				}
			}
			slices.Sort(out)
			return slices.Compact(out)
		}
	}
	return []string{addr}
}

func TestPostfixMapPrecedence(t *testing.T) {
	e := newEnv(t)
	setupCustomer(t, e, e.login("admin", adminPW), "")
	c := e.login("carla", "Strong-pass-carla-9")
	var doms []domain
	e.do(c, "GET", "/api/v1/accounts/me/domains", nil, &doms)
	if len(doms) == 0 {
		t.Fatal("no domains")
	}
	id, dn := doms[0].ID, "@carla-site.com"
	post := func(path string, body map[string]any) {
		t.Helper()
		if code := e.do(c, "POST", "/api/v1/accounts/me/"+path, body, nil); code != 201 {
			t.Fatalf("%s: %d", path, code)
		}
	}
	for _, box := range []string{"anna", "ben"} {
		post("mailboxes", map[string]any{"local_part": box, "domain_id": id, "password": "Strong-pass-box-91"})
	}
	post("mailing-lists", map[string]any{"local_part": "team", "domain_id": id, "members": []string{"x@example.org", "anna" + dn}})
	post("mail-aliases", map[string]any{"local_part": "ben", "domain_id": id, "destination": "ben@example.org"})
	post("mail-aliases", map[string]any{"local_part": "sales", "domain_id": id, "destination": "sales@example.org"})

	check := func(local string, want ...string) {
		t.Helper()
		slices.Sort(want)
		if got := postfixExpand(t, e, local+dn, map[string]bool{}); !slices.Equal(got, want) {
			t.Errorf("%s: delivered to %v, want %v", local, got, want)
		}
	}
	if code := e.do(c, "PUT", "/api/v1/accounts/me/default-address/"+id, map[string]any{"destination": "catch@example.org"}, nil); code != 204 {
		t.Fatalf("default address: %d", code)
	}
	check("anna", "anna"+dn)
	check("ben", "ben"+dn, "ben@example.org")
	check("sales", "sales@example.org")
	check("team", "x@example.org", "anna"+dn)
	check("nobody", "catch@example.org")

	post("domain-forwards", map[string]any{"domain_id": id, "target": "example.net"})
	check("anna", "anna"+dn)
	check("team", "x@example.org", "anna"+dn)
	check("nobody", "nobody@example.net")
}

func TestCannotClaimSomeoneElsesDomain(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)
	setupCustomer(t, e, admin, "")
	var pks []pkg
	e.do(admin, "GET", "/api/v1/packages", nil, &pks)
	if len(pks) == 0 {
		t.Fatal("no package")
	}
	for user, dom := range map[string]string{"victim": "victim-site.com", "childco": "shop.parent-site.com"} {
		if code := e.do(admin, "POST", "/api/v1/accounts", map[string]any{"username": user, "email": "v@example.com",
			"domain": dom, "package_id": pks[0].ID, "password": "Strong-pass-victim-9"}, nil); code != 201 {
			t.Fatalf("account %s: %d", user, code)
		}
	}
	if err := e.s.putSetting(context.Background(), "hostname", "server.hostco-panel.com"); err != nil {
		t.Fatal(err)
	}
	c := e.login("carla", "Strong-pass-carla-9")
	add := func(name, kind string) int {
		return e.do(c, "POST", "/api/v1/accounts/me/domains", map[string]any{"name": name, "kind": kind, "doc_root": ""}, nil)
	}
	for _, name := range []string{"gmail.com", "mail.gmail.com", "outlook.com", "login.victim-site.com", "parent-site.com", "server.hostco-panel.com"} {
		if code := add(name, "addon"); code < 400 || code >= 500 {
			t.Errorf("customer claimed %s: %d", name, code)
		}
	}
	if code := add("blog.carla-site.com", "subdomain"); code != 201 {
		t.Errorf("own subdomain refused: %d", code)
	}
}

func TestSSHCannotLockEveryoneOut(t *testing.T) {
	e := newEnv(t)
	admin := e.login("admin", adminPW)
	for _, root := range []string{"yes", "prohibit-password", "no"} {
		if code := e.do(admin, "PUT", "/api/v1/server/ssh", map[string]any{"password_auth": false, "root_login": root, "port": 22}, nil); code != 400 {
			t.Errorf("passwords off, root=%s, no keys: want 400, got %d", root, code)
		}
	}
}

func TestOldEmailFeatureListsStillBlockForwarders(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.db.Exec(ctx, `INSERT INTO feature_lists (id, owner_id, name, disabled, created_at) SELECT 'old1', id, 'Old', 'cron email', 0 FROM users`); err != nil {
		t.Fatal(err)
	}
	q, err := migrations.FS.ReadFile("0006_fixes.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(ctx, string(q)); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := e.db.QueryRow(ctx, `SELECT disabled FROM feature_lists WHERE id = 'old1'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"email", "forwarders", "default_address"} {
		if !slices.Contains(strings.Fields(got), k) {
			t.Errorf("%q lost from migrated list %q", k, got)
		}
	}
}
