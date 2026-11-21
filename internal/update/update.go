// Package update implements BreeferCP's self-update: it finds the latest
// GitHub release, downloads the tarball for this architecture, verifies it
// against the release's SHA256SUMS, and swaps the binary and web UI in place
// (keeping the previous version for rollback).
package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DefaultRepo is where official releases are published.
const DefaultRepo = "BreeferCp/cpanel"

// Release describes an available version.
type Release struct {
	Version     string    `json:"version"`
	Name        string    `json:"name"`
	Notes       string    `json:"notes"`
	PublishedAt time.Time `json:"published_at"`
	URL         string    `json:"url"`
	tarball     string
	sums        string
}

// Paths are the install locations the updater replaces.
type Paths struct {
	Binary  string // /usr/local/bin/breeferd
	WebDir  string // /usr/share/breefercp/web
	WorkDir string // /var/lib/breefercp/updates
}

// Client talks to the release API.
type Client struct {
	Repo string
	HTTP *http.Client
	// APIBase is overridable for tests.
	APIBase string
}

// New returns a client for repo (DefaultRepo when empty).
func New(repo string) *Client {
	if repo == "" {
		repo = DefaultRepo
	}
	return &Client{Repo: repo, HTTP: &http.Client{Timeout: 5 * time.Minute}, APIBase: "https://api.github.com"}
}

// Latest fetches the newest published (non-draft, non-prerelease) release.
func (c *Client) Latest(ctx context.Context) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.APIBase+"/repos/"+c.Repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "breefercp-updater")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("no release has been published yet")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release check failed: %s", resp.Status)
	}
	var raw struct {
		TagName     string    `json:"tag_name"`
		Name        string    `json:"name"`
		Body        string    `json:"body"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
		Assets      []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&raw); err != nil {
		return nil, err
	}
	r := &Release{Version: strings.TrimPrefix(raw.TagName, "v"), Name: raw.Name, Notes: raw.Body, PublishedAt: raw.PublishedAt, URL: raw.HTMLURL}
	suffix := "-linux-" + runtime.GOARCH + ".tar.gz"
	for _, a := range raw.Assets {
		switch {
		case strings.HasSuffix(a.Name, suffix):
			r.tarball = a.URL
		case a.Name == "SHA256SUMS":
			r.sums = a.URL
		}
	}
	return r, nil
}

// Newer reports whether version a is newer than b (semantic x.y.z; a
// development build "dev" is never considered newer or older).
func Newer(a, b string) bool {
	pa, oka := parse(a)
	pb, okb := parse(b)
	if !oka || !okb {
		return false
	}
	for i := range 3 {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	v, _, _ = strings.Cut(v, "-")
	parts := strings.Split(v, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Apply downloads, verifies and installs r. It does not restart the daemon.
func (c *Client) Apply(ctx context.Context, r *Release, p Paths) error {
	if r.tarball == "" || r.sums == "" {
		return fmt.Errorf("release %s has no %s build or no SHA256SUMS", r.Version, runtime.GOARCH)
	}
	if err := os.MkdirAll(p.WorkDir, 0o700); err != nil {
		return err
	}
	sums, err := c.fetch(ctx, r.sums, 1<<20)
	if err != nil {
		return err
	}
	tarPath := filepath.Join(p.WorkDir, "release.tar.gz")
	sum, err := c.download(ctx, r.tarball, tarPath)
	if err != nil {
		return err
	}
	defer os.Remove(tarPath)
	want := expectedSum(string(sums), filepath.Base(r.tarball))
	if want == "" || !strings.EqualFold(want, sum) {
		return errors.New("checksum mismatch: the download does not match SHA256SUMS; nothing was changed")
	}
	stage := filepath.Join(p.WorkDir, "stage-"+r.Version)
	_ = os.RemoveAll(stage)
	if err := extract(tarPath, stage); err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	root, err := singleDir(stage)
	if err != nil {
		return err
	}
	if err := installBinary(filepath.Join(root, "breeferd"), p.Binary); err != nil {
		return err
	}
	return swapDir(filepath.Join(root, "web"), p.WebDir)
}

// Rollback restores the binary and web UI kept by the previous Apply.
func Rollback(p Paths) error {
	if _, err := os.Stat(p.Binary + ".prev"); err != nil {
		return errors.New("no previous version is available")
	}
	if err := os.Rename(p.Binary+".prev", p.Binary); err != nil {
		return err
	}
	if _, err := os.Stat(p.WebDir + ".prev"); err == nil {
		_ = os.RemoveAll(p.WebDir)
		return os.Rename(p.WebDir+".prev", p.WebDir)
	}
	return nil
}

func (c *Client) fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "breefercp-updater")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

func (c *Client) download(ctx context.Context, url, dst string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "breefercp-updater")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download: %s", resp.Status)
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, 512<<20)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), f.Close()
}

// expectedSum finds name in sha256sum output ("<hex>  <name>").
func expectedSum(sums, name string) string {
	for _, l := range strings.Split(sums, "\n") {
		f := strings.Fields(l)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name && len(f[0]) == 64 {
			return f[0]
		}
	}
	return ""
}

// extract unpacks regular files and directories only, refusing absolute
// paths, "..", links and anything that would land outside dst.
func extract(archive, dst string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	var total int64
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(filepath.FromSlash(h.Name))
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe path in release: %s", h.Name)
		}
		target := filepath.Join(dst, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			total += h.Size
			if total > 1<<30 {
				return errors.New("release archive is too large")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if h.Mode&0o111 != 0 {
				mode = 0o755
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, io.LimitReader(tr, h.Size)); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			// Links and devices are never needed in a release; skip them.
		}
	}
}

func singleDir(stage string) (string, error) {
	entries, err := os.ReadDir(stage)
	if err != nil {
		return "", err
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		return "", errors.New("unexpected release layout")
	}
	root := filepath.Join(stage, entries[0].Name())
	if _, err := os.Stat(filepath.Join(root, "breeferd")); err != nil {
		return "", errors.New("release does not contain breeferd")
	}
	return root, nil
}

// installBinary replaces dst atomically, keeping dst.prev.
func installBinary(src, dst string) error {
	tmp := dst + ".new"
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		if err := copyFile(dst, dst+".prev"); err != nil {
			return err
		}
	}
	return os.Rename(tmp, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// swapDir moves src into place at dst, keeping the old copy as dst.prev.
func swapDir(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		return nil // release without a UI (not expected)
	}
	_ = os.RemoveAll(dst + ".prev")
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, dst+".prev"); err != nil {
			return err
		}
	}
	return os.Rename(src, dst)
}
