package provision

import (
	"bytes"
	"strings"
	"testing"

	"github.com/breefercp/breefercp/internal/system"
)

func TestStripDefiners(t *testing.T) {
	long := "INSERT INTO t VALUES ('" + strings.Repeat("x", 200<<10) + "CREATE DEFINER=`keep`@`me`');\n"
	in := "CREATE DEFINER=`root`@`localhost` PROCEDURE p()\n" +
		"/*!50013 DEFINER=`a`@`localhost` SQL SECURITY DEFINER */\n" +
		"INSERT INTO t VALUES ('DEFINER=`data`@`stays`');\n" + long + "last line without newline"
	var out bytes.Buffer
	if err := stripDefiners(&out, strings.NewReader(in)); err != nil {
		t.Fatal(err)
	}
	want := "CREATE  PROCEDURE p()\n" +
		"/*!50013  SQL SECURITY DEFINER */\n" +
		"INSERT INTO t VALUES ('DEFINER=`data`@`stays`');\n" + long + "last line without newline"
	if out.String() != want {
		t.Fatalf("unexpected output: %.300q", out.String())
	}
}

func TestWildcardRedirect(t *testing.T) {
	a := Account{SysUser: "carla", Home: "/home/carla", PHP: "8.3"}
	out, err := RenderVhost(Vhost{Domain: "x.test", Redirects: []Redirect{{Source: "/old", Target: "https://new.test/", Code: 301, Wildcard: true}}}, a, "/acme")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `return 301 https://new.test/$bfrest$is_args$args;`) {
		t.Fatalf("no wildcard redirect:\n%s", out)
	}
	for _, target := range []string{"https://new.test/?a=b", "https://new.test/#top"} {
		if ValidateRedirect(Redirect{Source: "/old", Target: target, Code: 301, Wildcard: true}) == nil {
			t.Errorf("accepted %s", target)
		}
	}
}

func TestImportUserCannotBeClaimed(t *testing.T) {
	u := importUser("acme_shop")
	if len(u) > 32 || u != importUser("acme_shop") || u == importUser("acme_blog") {
		t.Fatalf("bad import user %q", u)
	}
	for _, name := range []string{"bfimp", "pma"} {
		if system.SysUser(name) == nil {
			t.Errorf("a customer could be named %s and own the panel's database users", name)
		}
	}
}
