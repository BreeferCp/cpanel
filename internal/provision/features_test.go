package provision

import (
	"strings"
	"testing"
)

func TestRedirectValidationBlocksInjection(t *testing.T) {
	bad := []Redirect{
		{Source: "/a; } server { listen 1", Target: "https://x.com", Code: 301},
		{Source: "/a", Target: "https://x.com/$host", Code: 301},
		{Source: "/a", Target: "https://x.com/a;return 200", Code: 301},
		{Source: "/a", Target: "javascript:alert(1)", Code: 301},
		{Source: "/../etc", Target: "https://x.com", Code: 301},
		{Source: "/a", Target: "https://x.com", Code: 200},
		{Source: "/a b", Target: "https://x.com", Code: 301},
	}
	for _, r := range bad {
		if ValidateRedirect(r) == nil {
			t.Errorf("redirect %+v must be rejected", r)
		}
	}
	if err := ValidateRedirect(Redirect{Source: "/old-page", Target: "https://example.com/new?x=1", Code: 308}); err != nil {
		t.Fatalf("valid redirect rejected: %v", err)
	}
}

func TestVhostRedirectsAndErrorPages(t *testing.T) {
	out, err := RenderVhost(Vhost{Domain: "example.com", Redirects: []Redirect{{"/old", "https://example.com/new", 301}},
		ErrorPages: []int{404, 503}}, testAcct(), "/acme")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"location = /old { return 301 https://example.com/new; }",
		"error_page 404 /.breefer-errors/404.html;",
		"location ^~ /.breefer-errors/ { alias /home/alice01/error_pages/example.com/; internal; }",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("vhost missing %q", want)
		}
	}
	if _, err := RenderVhost(Vhost{Domain: "example.com", ErrorPages: []int{418}}, testAcct(), "/acme"); err == nil {
		t.Fatal("unsupported error code must be rejected")
	}
}

func TestPHPIniAllowList(t *testing.T) {
	if _, err := CleanPHPIni(map[string]string{"disable_functions": ""}); err == nil {
		t.Fatal("disable_functions must not be overridable")
	}
	if _, err := CleanPHPIni(map[string]string{"memory_limit": "512M\nphp_admin_value[open_basedir] = /"}); err == nil {
		t.Fatal("multi-line value must be rejected")
	}
	if _, err := CleanPHPIni(map[string]string{"memory_limit": "512M", "display_errors": "On", "date.timezone": "Europe/Berlin"}); err != nil {
		t.Fatalf("valid overrides rejected: %v", err)
	}
}

func TestOLSRender(t *testing.T) {
	a := testAcct()
	a.PHPIni = map[string]string{"memory_limit": "512M"}
	out, err := RenderOLSVhost(Vhost{Domain: "example.com", Aliases: []string{"www.example.com"},
		Redirects: []Redirect{{"/old.html", "https://example.com/new", 301}}, CertPath: "/c.pem", KeyPath: "/k.pem"}, a, "/acme")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"vhDomain                  example.com",
		"extUser                 alice01",
		"autoLoadHtaccess        1",
		`RewriteRule ^old\.html$ https://example.com/new [R=301,L]`,
		`php_value memory_limit "512M"`,
		"path                    /usr/local/lsws/lsphp83/bin/lsphp",
		"certFile                /c.pem",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("OLS vhost missing %q\n%s", want, out)
		}
	}
	inc := RenderOLSIncludes([]string{"b.com", "a.com"})
	if !strings.Contains(inc, "virtualhost a.com {") || !strings.Contains(inc, "map a.com a.com, www.a.com") {
		t.Errorf("includes wrong:\n%s", inc)
	}
}

func TestAppRuntimesAndVersions(t *testing.T) {
	u, err := RenderAppUnit(testAcct(), App{Name: "shop", Runtime: "ruby", Version: "3.3", Command: "bin/rails server -p $PORT", Dir: "/home/alice01/apps/shop", Port: 3005})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "/opt/breefercp/runtimes/ruby-3.3/bin/ruby") {
		t.Errorf("ruby runtime path missing:\n%s", u)
	}
	if _, err := RenderAppUnit(testAcct(), App{Name: "x", Runtime: "node", Version: "20;rm", Command: "a", Port: 3000}); err == nil {
		t.Fatal("malformed version must be rejected")
	}
}

func TestWPActionAllowList(t *testing.T) {
	p := newTestProv()
	if err := p.WPAction(t.Context(), testAcct(), "/home/alice01/public_html", "eval", "", ""); err == nil {
		t.Fatal("unknown WP-CLI action must be rejected")
	}
	if err := p.WPAction(t.Context(), testAcct(), "/home/alice01/public_html", "plugin-delete", "a b; id", ""); err == nil {
		t.Fatal("malformed plugin slug must be rejected")
	}
	if err := p.WPAction(t.Context(), testAcct(), "/etc", "cache-flush", "", ""); err == nil {
		t.Fatal("path outside home must be rejected")
	}
}
