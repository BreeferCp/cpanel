package files

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newJail(t *testing.T) (*FS, string) {
	t.Helper()
	home := t.TempDir()
	f, err := Open(home, -1, -1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f, home
}

func TestTraversalIsConfined(t *testing.T) {
	f, home := newJail(t)
	outside := filepath.Join(filepath.Dir(home), "secret.txt")
	if err := os.WriteFile(outside, []byte("top secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outside)

	// ".." is collapsed against the jail root, so this addresses home/secret.txt, not the real file.
	if _, err := f.ReadText("../secret.txt"); err == nil {
		t.Fatal("traversal read must not reach the file outside the jail")
	}
	if _, err := f.ReadText("/../../secret.txt"); err == nil {
		t.Fatal("absolute traversal must not reach the file outside the jail")
	}
}

func TestSymlinkEscapeBlocked(t *testing.T) {
	f, home := newJail(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "passwd"), []byte("root:x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, "evil")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if _, err := f.ReadText("evil/passwd"); err == nil {
		t.Fatal("symlink pointing outside the jail must not be followed")
	}
	if err := f.WriteFile("evil/new.txt", []byte("x")); err == nil {
		t.Fatal("write through escaping symlink must fail")
	}
}

func TestRoundTrip(t *testing.T) {
	f, _ := newJail(t)
	if err := f.Mkdir("a/b"); err != nil {
		t.Fatal(err)
	}
	if err := f.WriteFile("a/b/x.txt", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := f.Copy("a", "c"); err != nil {
		t.Fatal(err)
	}
	b, err := f.ReadText("c/b/x.txt")
	if err != nil || string(b) != "hello" {
		t.Fatalf("copy failed: %q %v", b, err)
	}
	if err := f.Rename("c", "d"); err != nil {
		t.Fatal(err)
	}
	if err := f.Remove("."); !errors.Is(err, ErrInvalidPath) {
		t.Fatal("removing the jail root must be refused")
	}
	if err := f.Remove("d"); err != nil {
		t.Fatal(err)
	}
	if err := f.Copy("a", "a/inside"); err == nil {
		t.Fatal("copying a directory into itself must fail")
	}
}

func TestExtractZipSlip(t *testing.T) {
	f, home := newJail(t)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("../../evil.txt")
	w.Write([]byte("pwn"))
	w, _ = zw.Create("ok/fine.txt")
	w.Write([]byte("fine"))
	zw.Close()
	if err := os.WriteFile(filepath.Join(home, "a.zip"), buf.Bytes(), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := f.Extract("a.zip", "out"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(home), "evil.txt")); err == nil {
		t.Fatal("zip-slip escaped the jail")
	}
	if _, err := os.Stat(filepath.Join(home, "out", "evil.txt")); err != nil {
		t.Fatal("slip entry should be neutralised inside the target directory:", err)
	}
	if b, err := f.ReadText("out/ok/fine.txt"); err != nil || string(b) != "fine" {
		t.Fatal("regular entry missing")
	}
}

func TestChmodStripsSpecialBits(t *testing.T) {
	f, home := newJail(t)
	if err := f.WriteFile("x", []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := f.Chmod("x", 0o4755); err != nil {
		t.Skip("chmod unsupported:", err)
	}
	st, _ := os.Stat(filepath.Join(home, "x"))
	if st.Mode()&os.ModeSetuid != 0 {
		t.Fatal("setuid bit must never be set")
	}
}
