package provision

import (
	"strings"
	"testing"
)

func TestMailFilterSieve(t *testing.T) {
	f := MailFilter{Name: "Invoices", Field: "subject", Op: "begins", Value: `Inv*"x`, Action: "fileinto", Dest: "Billing"}
	out, err := RenderSieve(SpamPolicy{}, nil, f)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `header :matches "subject" "Inv\\*\"x*"`) || !strings.Contains(out, `fileinto :create "Billing";`) {
		t.Fatalf("unexpected script:\n%s", out)
	}
	for _, bad := range []MailFilter{
		{Name: "x", Field: "subject", Op: "contains", Value: "a", Action: "fileinto", Dest: `Junk"; discard; "`},
		{Name: "x", Field: "subject", Op: "contains", Value: "a\nb", Action: "discard"},
		{Name: "x", Field: "body", Op: "contains", Value: "a", Action: "discard"},
		{Name: "x", Field: "from", Op: "contains", Value: "a", Action: "redirect", Dest: "not-an-address"},
		{Name: "x\ny", Field: "from", Op: "contains", Value: "a", Action: "discard"},
	} {
		if _, err := RenderSieve(SpamPolicy{}, nil, bad); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}

func TestVhostIndexesMimeGzip(t *testing.T) {
	a := Account{SysUser: "carla", Home: "/home/carla", PHP: "8.3"}
	v := Vhost{Domain: "carla.test", Indexes: []string{"/", "/downloads", "/private"}, Protected: []ProtectedDir{{ID: "abc12345", Path: "/private", Realm: "Members"}},
		Mime: []MimeType{{Ext: "webmanifest", Type: "application/manifest+json"}, {Ext: "glb", Type: "model/gltf-binary"}},
		Gzip: &Compression{Mode: "all"}}
	out, err := RenderVhost(v, a, "/acme")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"location ^~ /downloads/ {\n        autoindex on;", "gzip_types application/atom+xml", "default_type application/manifest+json;", "default_type model/gltf-binary;"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	// The protected /private/ location carries the listing; no duplicate prefix.
	if strings.Count(out, "location ^~ /private/") != 1 {
		t.Fatalf("duplicate /private/ location:\n%s", out)
	}
	for _, bad := range []Vhost{
		{Domain: "carla.test", Indexes: []string{"/a;b"}},
		{Domain: "carla.test", Mime: []MimeType{{Ext: "php", Type: "text/plain"}}},
		{Domain: "carla.test", Mime: []MimeType{{Ext: "txt", Type: "text/plain; }"}}},
		{Domain: "carla.test", Gzip: &Compression{Mode: "custom", Types: []string{"text/css;"}}},
	} {
		if _, err := RenderVhost(bad, a, "/acme"); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}
