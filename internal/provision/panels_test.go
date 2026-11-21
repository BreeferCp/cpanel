package provision

import (
	"strings"
	"testing"
)

func TestSieveEscapingAndOrder(t *testing.T) {
	ar := &Autoresponder{Subject: `Away "now" \ back`, Body: "Line 1\n\"quoted\"\nLine 3", IntervalDays: 3, StartsOn: "2026-12-20", EndsOn: "2027-01-05"}
	sp := SpamPolicy{Enabled: true, Threshold: 6, Action: "junk", Allow: []string{"boss@example.com"}, Deny: []string{"@spam.example"}}
	out, err := RenderSieve(sp, ar)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"Away \"now\" \\ back"`, `"******"`, `fileinto :create "Junk"`, `"boss@example.com"`, `currentdate :value "ge" "date" "2026-12-20"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Index(out, "X-Spam-Level") > strings.Index(out, "vacation :days") {
		t.Error("spam rule must run before the autoresponder")
	}
	if _, err := RenderSieve(SpamPolicy{}, &Autoresponder{Subject: "a\nb", Body: "x", IntervalDays: 1}); err == nil {
		t.Error("multi-line subject accepted")
	}
	if _, err := RenderSieve(SpamPolicy{Enabled: true, Threshold: 5, Action: "junk", Deny: []string{`x"]; discard; ["`}}, nil); err == nil {
		t.Error("sieve injection in sender list accepted")
	}
}

func TestVhostProtection(t *testing.T) {
	a := Account{SysUser: "acme", Home: "/home/acme", PHP: "8.3"}
	v := Vhost{Domain: "acme.com", DenyIPs: []string{"203.0.113.0/24", "198.51.100.7"},
		Hotlink: &Hotlink{Extensions: []string{"jpg", "png"}, AllowEmpty: true, Allowed: []string{"partner.example"}},
		Protected: []ProtectedDir{{ID: "0123456789abcdef0123456789abcdef", Path: "/private", Realm: "Members only"}}}
	out, err := RenderVhost(v, a, "/acme")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"deny 203.0.113.0/24;", "deny 198.51.100.7;", "allow all;", `valid_referers none blocked server_names acme.com *.acme.com partner.example;`,
		`location ^~ /private/ {`, `auth_basic "Members only";`, "/etc/nginx/breefercp-htpasswd/acme/0123456789abcdef0123456789abcdef"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, bad := range []Vhost{
		{Domain: "acme.com", DenyIPs: []string{"1.2.3.4; allow all"}},
		{Domain: "acme.com", Protected: []ProtectedDir{{ID: "0123456789abcdef", Path: "/x; }", Realm: "r"}}},
		{Domain: "acme.com", Protected: []ProtectedDir{{ID: "0123456789abcdef", Path: "/x", Realm: `r"; return 200; #`}}},
		{Domain: "acme.com", Hotlink: &Hotlink{Extensions: []string{"jpg)|.*"}}},
	} {
		if _, err := RenderVhost(bad, a, "/acme"); err == nil {
			t.Errorf("accepted unsafe vhost %+v", bad)
		}
	}
}

func TestPureDB(t *testing.T) {
	out, err := RenderPureDB([]FTPUser{{Login: "up@acme.com", Hash: "$6$salt$abc", UID: 1001, Home: "/home/acme", Dir: "public_html/up", QuotaMB: 10}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "up@acme.com:$6$salt$abc:1001:1001::/home/acme/public_html/up/./::::::10485760::") {
		t.Errorf("unexpected line: %s", out)
	}
	for _, bad := range []FTPUser{
		{Login: "up@acme.com", Hash: "$6$x:0:0", UID: 1001, Home: "/home/acme"},
		{Login: "x:y@acme.com", Hash: "$6$x", UID: 1001, Home: "/home/acme"},
		{Login: "up@acme.com", Hash: "$6$x", UID: 0, Home: "/root"},
		{Login: "up@acme.com", Hash: "$6$x", UID: 1001, Home: "/home/acme", Dir: "../.."},
	} {
		if _, err := RenderPureDB([]FTPUser{bad}); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

func TestGitAndRemoteHostValidation(t *testing.T) {
	for _, ok := range []string{"https://github.com/acme/site.git", "https://gitlab.example.com:8443/team/app"} {
		if GitCloneURL(ok) != nil {
			t.Errorf("rejected %s", ok)
		}
	}
	for _, bad := range []string{"file:///etc", "ssh://git@github.com/x", "https://user:pw@github.com/x", "https://github.com/x --upload-pack=sh", "ext::sh -c id"} {
		if GitCloneURL(bad) == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	for _, bad := range []string{"localhost", "a'@'%", "1.2.3.4; DROP", ""} {
		if RemoteHost(bad) == nil {
			t.Errorf("remote host accepted %q", bad)
		}
	}
	if RemoteHost("203.0.113.%") != nil || RemoteHost("db.example.com") != nil {
		t.Error("valid remote host rejected")
	}
}
