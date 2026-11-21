package provision

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/breefercp/breefercp/internal/config"
	"github.com/breefercp/breefercp/internal/system"
)

func testAcct() Account {
	return Account{SysUser: "alice01", UID: 20001, Home: "/home/alice01", PHP: "8.3",
		Limits: Limits{CPUPercent: 100, MemoryMB: 1024, IOWeight: 100, MaxProcs: 200}}
}

func newTestProv() *Provisioner {
	cfg := config.Default()
	cfg.DryRun = true
	return New(cfg, &system.Runner{DryRun: true, Log: slog.Default()}, slog.Default())
}

func TestRenderVhostRejectsInjection(t *testing.T) {
	_, err := RenderVhost(Vhost{Domain: "a.com;}\nserver {"}, testAcct(), "/acme")
	if err == nil {
		t.Fatal("domain with config injection must be rejected")
	}
}

func TestRenderVhostPHP(t *testing.T) {
	out, err := RenderVhost(Vhost{Domain: "example.com", Aliases: []string{"www.example.com"},
		CertPath: "/c.pem", KeyPath: "/k.pem", ForceHTTPS: true}, testAcct(), "/acme")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"server_name example.com www.example.com;",
		"return 301 https://$host$request_uri;",
		"fastcgi_pass unix:/run/php/breefer-alice01-8.3.sock;",
		"root /home/alice01/public_html;",
		"ssl_protocols TLSv1.2 TLSv1.3;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("vhost missing %q", want)
		}
	}
}

func TestCreateAccountCommands(t *testing.T) {
	p := newTestProv()
	if err := p.CreateAccount(context.Background(), testAcct()); err != nil {
		t.Fatal(err)
	}
	h := strings.Join(p.Runner().History, "\n")
	if !strings.Contains(h, "useradd --uid 20001") || !strings.Contains(h, "--shell /usr/sbin/nologin") {
		t.Errorf("unexpected provisioning transcript:\n%s", h)
	}
	if !strings.Contains(h, "CPUQuota=100%") {
		t.Errorf("cgroup limits not applied:\n%s", h)
	}
}

func TestCronRejectsInjectedSchedule(t *testing.T) {
	p := newTestProv()
	err := p.WriteCron(context.Background(), testAcct(), []CronJob{{Schedule: "* * * * * root", Command: "id", Enabled: true}})
	if err == nil {
		t.Fatal("6-field schedule must be rejected")
	}
	err = p.WriteCron(context.Background(), testAcct(), []CronJob{{Schedule: "0 * * * *", Command: "id\n* * * * * root rm -rf /", Enabled: true}})
	if err == nil {
		t.Fatal("multi-line command must be rejected")
	}
}

func TestFirewallRendering(t *testing.T) {
	out, err := RenderNFT([]FirewallRule{{Action: "allow", Protocol: "tcp", Port: "3000:3010", Source: "10.0.0.0/8"}}, []string{"203.0.113.5"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ip saddr 10.0.0.0/8 tcp dport 3000-3010 accept") || !strings.Contains(out, "ip saddr 203.0.113.5 drop") {
		t.Errorf("ruleset wrong:\n%s", out)
	}
	if _, err := RenderNFT([]FirewallRule{{Action: "allow", Protocol: "tcp", Port: "80; drop"}}, nil); err == nil {
		t.Fatal("malformed port must be rejected")
	}
}

func TestAppUnitHardening(t *testing.T) {
	u, err := RenderAppUnit(testAcct(), App{Name: "api", Runtime: "node", Command: "server.js", Dir: "/home/alice01/apps/api", Port: 3000,
		Env: map[string]string{"NODE_ENV": "production"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"NoNewPrivileges=yes", "Slice=user-20001.slice", "User=alice01", `Environment="NODE_ENV=production"`} {
		if !strings.Contains(u, want) {
			t.Errorf("unit missing %q", want)
		}
	}
	if _, err := RenderAppUnit(testAcct(), App{Name: "a", Runtime: "node", Command: "x", Port: 80}); err == nil {
		t.Fatal("privileged port must be rejected")
	}
}
