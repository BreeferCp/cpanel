package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.3.0", "0.2.9", true}, {"v1.0.0", "0.9.9", true}, {"0.2.0", "0.2.0", false},
		{"0.2.0", "0.10.0", false}, {"dev", "0.1.0", false}, {"0.3.0", "dev", false}, {"0.3", "0.2.5", true},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func tarball(t *testing.T, files map[string]string, extra func(*tar.Writer)) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(body))
	}
	if extra != nil {
		extra(tw)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func serve(t *testing.T, tgz []byte, sum string) *Client {
	name := "breefercp-0.9.0-linux-" + runtime.GOARCH + ".tar.gz"
	mux := http.NewServeMux()
	var base string
	mux.HandleFunc("/repos/x/y/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v0.9.0","name":"0.9.0","body":"notes","assets":[{"name":%q,"browser_download_url":%q},{"name":"SHA256SUMS","browser_download_url":%q}]}`,
			name, base+"/dl/"+name, base+"/dl/SHA256SUMS")
	})
	mux.HandleFunc("/dl/"+name, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tgz) })
	mux.HandleFunc("/dl/SHA256SUMS", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprintf(w, "%s  %s\n", sum, name) })
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	base = ts.URL
	c := New("x/y")
	c.APIBase = ts.URL
	return c
}

func TestApplyAndRollback(t *testing.T) {
	tgz := tarball(t, map[string]string{"breefercp-0.9.0/breeferd": "NEW", "breefercp-0.9.0/web/index.html": "<new>"}, nil)
	h := sha256.Sum256(tgz)
	c := serve(t, tgz, hex.EncodeToString(h[:]))
	dir := t.TempDir()
	p := Paths{Binary: filepath.Join(dir, "breeferd"), WebDir: filepath.Join(dir, "web"), WorkDir: filepath.Join(dir, "work")}
	_ = os.WriteFile(p.Binary, []byte("OLD"), 0o755)
	_ = os.MkdirAll(p.WebDir, 0o755)
	_ = os.WriteFile(filepath.Join(p.WebDir, "index.html"), []byte("<old>"), 0o644)

	r, err := c.Latest(context.Background())
	if err != nil || r.Version != "0.9.0" {
		t.Fatalf("latest: %+v %v", r, err)
	}
	if err := c.Apply(context.Background(), r, p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p.Binary); string(b) != "NEW" {
		t.Fatalf("binary not replaced: %s", b)
	}
	if b, _ := os.ReadFile(filepath.Join(p.WebDir, "index.html")); string(b) != "<new>" {
		t.Fatalf("web not replaced: %s", b)
	}
	if err := Rollback(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p.Binary); string(b) != "OLD" {
		t.Fatalf("rollback failed: %s", b)
	}
}

func TestChecksumMismatchChangesNothing(t *testing.T) {
	tgz := tarball(t, map[string]string{"breefercp-0.9.0/breeferd": "EVIL"}, nil)
	c := serve(t, tgz, "0000000000000000000000000000000000000000000000000000000000000000")
	dir := t.TempDir()
	p := Paths{Binary: filepath.Join(dir, "breeferd"), WebDir: filepath.Join(dir, "web"), WorkDir: filepath.Join(dir, "work")}
	_ = os.WriteFile(p.Binary, []byte("OLD"), 0o755)
	r, _ := c.Latest(context.Background())
	if err := c.Apply(context.Background(), r, p); err == nil {
		t.Fatal("expected checksum failure")
	}
	if b, _ := os.ReadFile(p.Binary); string(b) != "OLD" {
		t.Fatal("binary changed despite checksum mismatch")
	}
}

func TestExtractRejectsTraversal(t *testing.T) {
	tgz := tarball(t, map[string]string{"../../escape": "x"}, nil)
	dir := t.TempDir()
	path := filepath.Join(dir, "a.tgz")
	_ = os.WriteFile(path, tgz, 0o600)
	if err := extract(path, filepath.Join(dir, "out")); err == nil {
		t.Fatal("traversal accepted")
	}
}
