package api

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeDNS struct {
	ns, txt, host map[string][]string
}

func (f fakeDNS) LookupNS(_ context.Context, n string) ([]*net.NS, error) {
	var out []*net.NS
	for _, v := range f.ns[n] {
		out = append(out, &net.NS{Host: v})
	}
	if out == nil {
		return nil, errors.New("not found")
	}
	return out, nil
}

func (f fakeDNS) LookupTXT(_ context.Context, n string) ([]string, error) {
	if v, ok := f.txt[n]; ok {
		return v, nil
	}
	return nil, errors.New("not found")
}

func (f fakeDNS) LookupHost(_ context.Context, n string) ([]string, error) {
	if v, ok := f.host[n]; ok {
		return v, nil
	}
	return nil, errors.New("not found")
}

func TestDomainOwnership(t *testing.T) {
	e := newEnv(t)
	aid := setupCustomer(t, e, e.login("admin", adminPW), "")
	ctx := context.Background()
	// No real lookups and no private or loopback matches.
	for k, v := range map[string]string{"server_ip": "8.8.4.4", "nameserver1": "ns1.hostco-dns.com", "nameserver2": "ns2.hostco-dns.com"} {
		if err := e.s.putSetting(ctx, k, v); err != nil {
			t.Fatal(err)
		}
	}
	e.s.resolver = fakeDNS{
		ns: map[string][]string{
			"delegated.com.": {"NS1.hostco-dns.com.", "ns2.hostco-dns.com."},
			"half.com.":      {"ns1.hostco-dns.com.", "ns1.someone-else.com."},
		},
		host: map[string][]string{"pointed.com.": {"8.8.4.4"}, "elsewhere.com.": {"1.1.1.1"}},
		txt: map[string][]string{
			"_breefercp.txt-proof.com.": {verifyValue(aid, "txt-proof.com")},
			"_breefercp.stolen.com.":    {verifyValue("another-account", "stolen.com")},
		},
	}
	for name, want := range map[string]bool{
		"delegated.com": true, "pointed.com": true, "txt-proof.com": true, "blog.carla-site.com": true,
		"half.com": false, "elsewhere.com": false, "stolen.com": false, "bank-example.com": false,
	} {
		if err := e.s.verifyDomain(ctx, nil, aid, name); (err == nil) != want {
			t.Errorf("%s: err=%v, want allowed=%v", name, err, want)
		}
	}
	if err := e.s.putSetting(ctx, "domain_verification", "off"); err != nil {
		t.Fatal(err)
	}
	if err := e.s.verifyDomain(ctx, nil, aid, "bank-example.com"); err != nil {
		t.Errorf("switched off but still checked: %v", err)
	}
}

func TestLeechCannotBeSpoofed(t *testing.T) {
	d := leechDir{path: "/private"}
	for _, c := range []struct {
		target, user string
		status       int
		want         bool
	}{
		{"/private/file", "bob", 200, true},
		{"/private/", "bob", 200, true},
		{"/private/file?x=1", "bob", 206, true},
		{"/private/file", "bob", 401, false},
		{"/private/.env", "bob", 404, false},
		{"/private/old", "bob", 301, false},
		{"/public/file", "bob", 200, false},
		{"/private/../public", "bob", 200, false},
		{"/private/%2e%2e/public", "bob", 200, false},
		{"/private-other/file", "bob", 200, false},
		{"/private", "bob", 200, false},
		{"/private/file", "-", 200, false},
	} {
		if got := d.protects(logEntry{target: c.target, user: c.user, status: c.status}); got != c.want {
			t.Errorf("%+v: got %v", c, got)
		}
	}
}

func TestBandwidthScan(t *testing.T) {
	e := newEnv(t)
	aid := setupCustomer(t, e, e.login("admin", adminPW), "")
	ctx := context.Background()
	a, err := scanAcct(e.db.QueryRow(ctx, acctSelect+` WHERE a.id = ?`, aid))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "carla-site.com.access.log")
	now := time.Now().UTC().Format("02/Jan/2006:15:04:05 -0700")
	line := func(ip, size string) string {
		return ip + ` - - [` + now + `] "GET / HTTP/1.1" 200 ` + size + ` "-" "test"` + "\n"
	}
	used := func() int {
		t.Helper()
		if err := e.s.scanAccountLogs(ctx, a, dir); err != nil {
			t.Fatal(err)
		}
		var mb int
		if err := e.db.QueryRow(ctx, `SELECT bw_used_mb FROM accounts WHERE id = ?`, aid).Scan(&mb); err != nil {
			t.Fatal(err)
		}
		return mb
	}
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	first := line("198.51.100.1", "3145728")
	partial := `198.51.100.2 - - [` + now + `] "GET / HTT`
	write(first + partial)
	if got := used(); got != 3 {
		t.Fatalf("first scan: %d MB", got)
	}
	if got := used(); got != 3 {
		t.Fatalf("counted twice: %d MB", got)
	}
	write(first + line("198.51.100.2", "2097152"))
	if got := used(); got != 5 {
		t.Fatalf("completed line: %d MB", got)
	}
	// Rotated, and already bigger than the old offset.
	write(line("198.51.100.9", "1048576") + line("198.51.100.9", "1048576") + line("198.51.100.9", "1048576"))
	if got := used(); got != 8 {
		t.Fatalf("after rotation: %d MB", got)
	}
}

func TestArchiveNames(t *testing.T) {
	doms := []domain{{Name: "carla-site.com"}}
	for name, want := range map[string]bool{
		"carla-site.com.access.log.1":    true,
		"carla-site.com.error.log.4.gz":  true,
		"other-site.com.access.log.1":    false,
		"../carla-site.com.access.log.1": false,
		"carla-site.com.access.log":      false,
	} {
		if archiveAllowed(doms, name) != want {
			t.Errorf("%s: want %v", name, want)
		}
	}
}
