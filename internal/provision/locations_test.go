package provision

import (
	"regexp"
	"strings"
	"testing"
)

// These tests apply nginx's documented location-selection rules to a rendered
// vhost. They are a model of nginx, not nginx itself: they catch precedence
// mistakes that substring checks cannot, but do not validate the syntax.

type ngxLocation struct {
	mod, arg string
	body     string
	children []*ngxLocation
}

var reTestLocation = regexp.MustCompile(`^location (?:(=|\^~|~\*|~) )?(.+?) \{(.*)$`)

func parseLocations(t *testing.T, conf string) []*ngxLocation {
	t.Helper()
	root := &ngxLocation{}
	var stack []*ngxLocation
	cur := root
	for _, line := range strings.Split(conf, "\n") {
		line = strings.TrimSpace(line)
		if m := reTestLocation.FindStringSubmatch(line); m != nil {
			l := &ngxLocation{mod: m[1], arg: m[2], body: m[3]}
			cur.children = append(cur.children, l)
			if !strings.HasSuffix(m[3], "}") {
				stack = append(stack, cur)
				cur = l
			}
			continue
		}
		if line == "}" && len(stack) > 0 {
			cur, stack = stack[len(stack)-1], stack[:len(stack)-1]
			continue
		}
		cur.body += line + "\n"
	}
	if len(stack) != 0 {
		t.Fatal("unbalanced locations")
	}
	return root.children
}

// selectLocation returns the chain of locations serving path, outermost first.
func selectLocation(locs []*ngxLocation, path string) []*ngxLocation {
	var best *ngxLocation
	for _, l := range locs {
		switch l.mod {
		case "=":
			if l.arg == path {
				return []*ngxLocation{l}
			}
		case "", "^~":
			if strings.HasPrefix(path, l.arg) && (best == nil || len(l.arg) > len(best.arg)) {
				best = l
			}
		}
	}
	var inner []*ngxLocation
	if best != nil {
		if inner = selectLocation(best.children, path); len(inner) > 0 || best.mod == "^~" {
			return append([]*ngxLocation{best}, inner...)
		}
	}
	for _, l := range locs {
		if l.mod != "~" && l.mod != "~*" {
			continue
		}
		pat := l.arg
		// RE2 has no lookahead; this is the only one the template uses.
		excludeWellKnown := strings.HasSuffix(pat, "(?!well-known)")
		pat = strings.TrimSuffix(pat, "(?!well-known)")
		if l.mod == "~*" {
			pat = "(?i)" + pat
		}
		for _, at := range regexp.MustCompile(pat).FindAllStringIndex(path, -1) {
			if !excludeWellKnown || !strings.HasPrefix(path[at[1]:], "well-known") {
				return []*ngxLocation{l}
			}
		}
	}
	if best != nil {
		return []*ngxLocation{best}
	}
	return nil
}

func TestVhostLocationPrecedence(t *testing.T) {
	v := Vhost{
		Domain:  "carla.test",
		Indexes: []string{"/", "/downloads", "/private/sub", "/private/sub/deep", "/open"},
		Protected: []ProtectedDir{
			{ID: "aaaaaaaa", Path: "/private", Realm: "Members"},
			{ID: "bbbbbbbb", Path: "/open/locked", Realm: "Locked"},
		},
	}
	out, err := RenderVhost(v, Account{SysUser: "carla", Home: "/home/carla", PHP: "8.3"}, "/acme")
	if err != nil {
		t.Fatal(err)
	}
	locs := parseLocations(t, out)
	for _, c := range []struct {
		path              string
		denied, auth, php bool
	}{
		{path: "/.env", denied: true},
		{path: "/file.txt"},
		{path: "/index.php", php: true},
		{path: "/downloads/"},
		{path: "/downloads/.env", denied: true},
		{path: "/downloads/.git/config", denied: true},
		{path: "/downloads/.hidden.php", denied: true},
		{path: "/downloads/x.php", php: true},
		{path: "/private/file.txt", auth: true},
		{path: "/private/.env", denied: true},
		{path: "/private/x.php", auth: true, php: true},
		{path: "/private/sub/", auth: true},
		{path: "/private/sub/file.txt", auth: true},
		{path: "/private/sub/x.php", auth: true, php: true},
		{path: "/private/sub/.env", denied: true},
		{path: "/private/sub/deep/file.txt", auth: true},
		{path: "/open/"},
		{path: "/open/locked/file.txt", auth: true},
	} {
		var body string
		for _, l := range selectLocation(locs, c.path) {
			body += l.body
		}
		if got := strings.Contains(body, "deny all"); got != c.denied {
			t.Errorf("%s: denied=%v, want %v", c.path, got, c.denied)
		}
		if c.denied {
			continue
		}
		if got := strings.Contains(body, "auth_basic_user_file"); got != c.auth {
			t.Errorf("%s: password=%v, want %v", c.path, got, c.auth)
		}
		if got := strings.Contains(body, "fastcgi_pass"); got != c.php {
			t.Errorf("%s: sent to PHP=%v, want %v", c.path, got, c.php)
		}
	}
}

func TestHotlinkCannotExposeScripts(t *testing.T) {
	for _, ext := range []string{"php", "phtml", "phar"} {
		if ValidateHotlink(Hotlink{Extensions: []string{"jpg", ext}}) == nil {
			t.Errorf("hotlink accepted %q, which nginx would then send as source", ext)
		}
	}
}
