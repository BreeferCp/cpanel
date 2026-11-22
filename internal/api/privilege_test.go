package api

import (
	"os"
	"strings"
	"testing"

	"github.com/breefercp/breefercp/internal/provision"
)

// The panel runs as root and the customer controls everything below their
// home, symlinks included. So root must never create, write, chown, remove or
// read a path there: those commands run as the account, through runuser.
// The dry-run transcript records what would have been executed.

const testPubKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl test"

func TestRootNeverActsOnCustomerControlledPaths(t *testing.T) {
	e := newEnv(t)
	setupCustomer(t, e, e.login("admin", adminPW), "")
	c := e.login("carla", "Strong-pass-carla-9")
	var doms []domain
	e.do(c, "GET", "/api/v1/accounts/me/domains", nil, &doms)
	if len(doms) == 0 {
		t.Fatal("no domains")
	}
	run := e.s.prov.Runner()
	run.History = nil // account creation works on a fresh home
	for _, x := range []struct {
		path string
		body map[string]any
	}{
		{"domains", map[string]any{"name": "blog.carla-site.com", "kind": "subdomain", "doc_root": "public_html/blog"}},
		{"ssh-keys", map[string]any{"name": "laptop", "public_key": testPubKey}},
		{"apps", map[string]any{"name": "shop", "runtime": "node", "command": "server.js"}},
		{"mailboxes", map[string]any{"local_part": "anna", "domain_id": doms[0].ID, "password": "Strong-pass-box-91"}},
	} {
		if code := e.do(c, "POST", "/api/v1/accounts/me/"+x.path, x.body, nil); code != 201 {
			t.Fatalf("%s: %d", x.path, code)
		}
	}
	for _, line := range run.History {
		line = strings.ReplaceAll(line, `\`, "/")
		cmd, _, _ := strings.Cut(line, " ")
		if cmd == "runuser" {
			continue
		}
		switch cmd {
		case "install", "chown", "chmod", "rm", "tar", "write":
			if strings.Contains(line, "/home/carla") {
				t.Errorf("root acts on a customer path: %s", line)
			}
		}
	}
}

func TestDocRootCannotInjectConfig(t *testing.T) {
	e := newEnv(t)
	setupCustomer(t, e, e.login("admin", adminPW), "")
	c := e.login("carla", "Strong-pass-carla-9")
	for _, root := range []string{"public_html/x; error_log /etc/cron.d/x", "public_html/x {", "public_html/$host", "public_html/x#"} {
		body := map[string]any{"name": "blog.carla-site.com", "kind": "subdomain", "doc_root": root}
		if code := e.do(c, "POST", "/api/v1/accounts/me/domains", body, nil); code != 400 && code != 422 {
			t.Errorf("document root %q: got %d, want it refused", root, code)
		}
	}
	a := provision.Account{SysUser: "carla", Home: "/home/carla", PHP: "8.3"}
	if _, err := provision.RenderVhost(provision.Vhost{Domain: "x.test", DocRoot: "/home/carla/a; include /etc/x"}, a, "/acme"); err == nil {
		t.Error("renderer accepted a document root containing a directive")
	}
}

func TestAppCannotEscapeHomeOrTakeAnotherPort(t *testing.T) {
	e := newEnv(t)
	setupCustomer(t, e, e.login("admin", adminPW), "")
	c := e.login("carla", "Strong-pass-carla-9")
	for name, body := range map[string]map[string]any{
		"traversal":  {"name": "../../../etc", "runtime": "docker", "image": "nginx"},
		"panel port": {"name": "proxy", "runtime": "node", "command": "x.js", "port": 2087},
		"rspamd":     {"name": "proxy", "runtime": "node", "command": "x.js", "port": 11334},
	} {
		if code := e.do(c, "POST", "/api/v1/accounts/me/apps", body, nil); code != 400 && code != 422 {
			t.Errorf("%s: got %d, want it refused", name, code)
		}
	}
	for _, line := range e.s.prov.Runner().History {
		if strings.Contains(line, "../") {
			t.Errorf("traversal reached a command: %s", line)
		}
	}
}

func TestWebLogsAreOutsideTheHome(t *testing.T) {
	a := provision.Account{SysUser: "carla", Home: "/home/carla", PHP: "8.3"}
	out, err := provision.RenderVhost(provision.Vhost{Domain: "x.test"}, a, "/acme")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) > 1 && (f[0] == "access_log" || f[0] == "error_log") && strings.HasPrefix(f[1], a.Home) {
			t.Errorf("nginx opens this as root inside the home: %s", line)
		}
	}
	if !strings.Contains(out, "disable_symlinks if_not_owner") {
		t.Error("vhost follows symlinks into other accounts")
	}
}

func TestServiceCanWriteItsConfig(t *testing.T) {
	b, err := os.ReadFile("../../deploy/breeferd.service")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		switch strings.TrimSpace(line) {
		case "ProtectSystem=full", "ProtectSystem=strict":
			t.Errorf("%s makes /etc read-only, and ReadWritePaths=/etc cannot undo it", line)
		}
	}
}
