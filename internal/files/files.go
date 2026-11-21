// Package files implements the file manager backend. Every operation is
// confined to an account's home directory with os.Root, which resolves paths
// beneath a directory handle and refuses ".." and symlink escapes at the
// kernel level, closing the classic check-then-use races of string-based
// path jails.
package files

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

// Limits protecting the host from abusive archives and uploads.
const (
	MaxEditSize      = 5 << 20 // largest file the in-browser editor opens
	MaxExtractBytes  = 4 << 30 // uncompressed bytes per extraction
	MaxExtractFiles  = 100_000 // entries per extraction
	MaxCompressBytes = 4 << 30
)

// ErrInvalidPath is returned for paths that cannot be mapped into the jail.
var ErrInvalidPath = errors.New("invalid path")

// ErrTooLarge is returned when a size limit would be exceeded.
var ErrTooLarge = errors.New("file too large")

// FS is a jailed view of one account's home directory.
type FS struct {
	root     *os.Root
	uid, gid int // owner applied to created files; <0 disables chown
}

// Open jails home. uid/gid are applied to everything the panel creates.
func Open(home string, uid, gid int) (*FS, error) {
	r, err := os.OpenRoot(home)
	if err != nil {
		return nil, err
	}
	return &FS{root: r, uid: uid, gid: gid}, nil
}

// Close releases the directory handle.
func (f *FS) Close() error { return f.root.Close() }

// Entry describes one directory entry.
type Entry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Dir     bool      `json:"dir"`
	Symlink bool      `json:"symlink"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	ModTime time.Time `json:"mod_time"`
}

// Clean normalises a client path into a jail-relative path. Absolute paths
// are interpreted relative to the jail root ("/" == home).
func Clean(p string) (string, error) {
	if strings.ContainsRune(p, 0) || len(p) > 4096 {
		return "", ErrInvalidPath
	}
	c := path.Clean("/" + p)
	rel := strings.TrimPrefix(c, "/")
	if rel == "" {
		return ".", nil
	}
	return rel, nil
}

// List returns directory entries, directories first.
func (f *FS) List(p string) ([]Entry, error) {
	rel, err := Clean(p)
	if err != nil {
		return nil, err
	}
	d, err := f.root.Open(rel)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	des, err := d.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(des))
	for _, de := range des {
		info, err := de.Info()
		if err != nil {
			continue
		}
		out = append(out, Entry{
			Name: de.Name(), Path: "/" + strings.TrimPrefix(path.Join(rel, de.Name()), "./"),
			Dir: de.IsDir(), Symlink: info.Mode()&fs.ModeSymlink != 0,
			Size: info.Size(), Mode: fmt.Sprintf("%04o", info.Mode().Perm()), ModTime: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// Stat describes a path without following a final symlink.
func (f *FS) Stat(p string) (fs.FileInfo, error) {
	rel, err := Clean(p)
	if err != nil {
		return nil, err
	}
	return f.root.Lstat(rel)
}

// OpenRead opens a regular file for reading. Directories are rejected.
func (f *FS) OpenRead(p string) (*os.File, fs.FileInfo, error) {
	rel, err := Clean(p)
	if err != nil {
		return nil, nil, err
	}
	fl, err := f.root.Open(rel)
	if err != nil {
		return nil, nil, err
	}
	info, err := fl.Stat()
	if err != nil {
		fl.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		fl.Close()
		return nil, nil, fmt.Errorf("%w: not a regular file", ErrInvalidPath)
	}
	return fl, info, nil
}

// ReadText returns a file's content for the editor (size-capped).
func (f *FS) ReadText(p string) ([]byte, error) {
	fl, info, err := f.OpenRead(p)
	if err != nil {
		return nil, err
	}
	defer fl.Close()
	if info.Size() > MaxEditSize {
		return nil, ErrTooLarge
	}
	return io.ReadAll(io.LimitReader(fl, MaxEditSize+1))
}

// WriteFile creates or truncates a file with the given content.
func (f *FS) WriteFile(p string, data []byte) error {
	rel, err := Clean(p)
	if err != nil || rel == "." {
		return ErrInvalidPath
	}
	if len(data) > MaxEditSize {
		return ErrTooLarge
	}
	mode := os.FileMode(0o640)
	if st, err := f.root.Lstat(rel); err == nil {
		if !st.Mode().IsRegular() {
			return fmt.Errorf("%w: not a regular file", ErrInvalidPath)
		}
		mode = st.Mode().Perm()
	}
	if err := f.root.WriteFile(rel, data, mode); err != nil {
		return err
	}
	return f.own(rel)
}

// Create opens a new file for streaming writes (uploads). The caller closes it
// and then calls Finish.
func (f *FS) Create(p string) (*os.File, string, error) {
	rel, err := Clean(p)
	if err != nil || rel == "." {
		return nil, "", ErrInvalidPath
	}
	fl, err := f.root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o640)
	return fl, rel, err
}

// Finish applies account ownership to an uploaded file.
func (f *FS) Finish(rel string) error { return f.own(rel) }

// Mkdir creates a directory (and parents).
func (f *FS) Mkdir(p string) error {
	rel, err := Clean(p)
	if err != nil || rel == "." {
		return ErrInvalidPath
	}
	if err := f.root.MkdirAll(rel, 0o750); err != nil {
		return err
	}
	// Own every component created.
	parts := strings.Split(rel, "/")
	for i := range parts {
		_ = f.own(strings.Join(parts[:i+1], "/"))
	}
	return nil
}

// Rename moves a file or directory inside the jail.
func (f *FS) Rename(from, to string) error {
	a, err1 := Clean(from)
	b, err2 := Clean(to)
	if err1 != nil || err2 != nil || a == "." || b == "." {
		return ErrInvalidPath
	}
	if _, err := f.root.Lstat(b); err == nil {
		return fs.ErrExist
	}
	return f.root.Rename(a, b)
}

// Remove deletes a file or a directory tree. The home directory itself is
// protected.
func (f *FS) Remove(p string) error {
	rel, err := Clean(p)
	if err != nil || rel == "." {
		return ErrInvalidPath
	}
	return f.root.RemoveAll(rel)
}

// Chmod changes permission bits. setuid/setgid/sticky bits are never set.
func (f *FS) Chmod(p string, mode os.FileMode) error {
	rel, err := Clean(p)
	if err != nil || rel == "." {
		return ErrInvalidPath
	}
	return f.root.Chmod(rel, mode&0o777)
}

// Copy duplicates a file or directory tree.
func (f *FS) Copy(from, to string) error {
	a, err1 := Clean(from)
	b, err2 := Clean(to)
	if err1 != nil || err2 != nil || a == "." || b == "." {
		return ErrInvalidPath
	}
	if _, err := f.root.Lstat(b); err == nil {
		return fs.ErrExist
	}
	if b == a || strings.HasPrefix(b, a+"/") {
		return fmt.Errorf("%w: cannot copy into itself", ErrInvalidPath)
	}
	return f.copyTree(a, b, 0)
}

func (f *FS) copyTree(src, dst string, depth int) error {
	if depth > 64 {
		return ErrInvalidPath
	}
	info, err := f.root.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		return nil // symlinks are skipped, never followed
	case info.IsDir():
		if err := f.root.Mkdir(dst, info.Mode().Perm()); err != nil {
			return err
		}
		_ = f.own(dst)
		d, err := f.root.Open(src)
		if err != nil {
			return err
		}
		names, err := d.Readdirnames(-1)
		d.Close()
		if err != nil {
			return err
		}
		for _, n := range names {
			if err := f.copyTree(path.Join(src, n), path.Join(dst, n), depth+1); err != nil {
				return err
			}
		}
		return nil
	case info.Mode().IsRegular():
		in, err := f.root.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := f.root.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
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
		return f.own(dst)
	}
	return nil
}

// Compress writes a zip of the given paths to dest.
func (f *FS) Compress(paths []string, dest string) error {
	d, err := Clean(dest)
	if err != nil || d == "." {
		return ErrInvalidPath
	}
	out, err := f.root.OpenFile(d, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(out)
	var total int64
	var addErr error
	for _, p := range paths {
		rel, err := Clean(p)
		if err != nil {
			addErr = err
			break
		}
		if addErr = f.addToZip(zw, rel, &total, 0); addErr != nil {
			break
		}
	}
	if err := zw.Close(); addErr == nil {
		addErr = err
	}
	if err := out.Close(); addErr == nil {
		addErr = err
	}
	if addErr != nil {
		_ = f.root.Remove(d)
		return addErr
	}
	return f.own(d)
}

func (f *FS) addToZip(zw *zip.Writer, rel string, total *int64, depth int) error {
	if depth > 64 {
		return ErrInvalidPath
	}
	info, err := f.root.Lstat(rel)
	if err != nil {
		return err
	}
	name := rel
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		return nil
	case info.IsDir():
		if rel != "." {
			if _, err := zw.Create(name + "/"); err != nil {
				return err
			}
		}
		d, err := f.root.Open(rel)
		if err != nil {
			return err
		}
		names, err := d.Readdirnames(-1)
		d.Close()
		if err != nil {
			return err
		}
		for _, n := range names {
			child := n
			if rel != "." {
				child = path.Join(rel, n)
			}
			if err := f.addToZip(zw, child, total, depth+1); err != nil {
				return err
			}
		}
	case info.Mode().IsRegular():
		*total += info.Size()
		if *total > MaxCompressBytes {
			return ErrTooLarge
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name, hdr.Method = name, zip.Deflate
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		in, err := f.root.Open(rel)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(w, in)
		return err
	}
	return nil
}

// Extract unpacks a zip archive into dir. Entry names are confined by
// os.Root; counts and sizes are capped against zip bombs.
func (f *FS) Extract(archive, dir string) error {
	a, err := Clean(archive)
	if err != nil {
		return err
	}
	d, err := Clean(dir)
	if err != nil {
		return err
	}
	fl, err := f.root.Open(a)
	if err != nil {
		return err
	}
	defer fl.Close()
	st, err := fl.Stat()
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(fl, st.Size())
	if err != nil {
		return fmt.Errorf("%w: not a valid zip archive", ErrInvalidPath)
	}
	if len(zr.File) > MaxExtractFiles {
		return ErrTooLarge
	}
	if err := f.root.MkdirAll(d, 0o750); err != nil {
		return err
	}
	var total int64
	for _, zf := range zr.File {
		name := path.Clean("/" + zf.Name)
		if name == "/" {
			continue
		}
		target := path.Join(d, strings.TrimPrefix(name, "/"))
		if zf.Mode()&fs.ModeSymlink != 0 {
			continue // never materialise symlinks from archives
		}
		if zf.FileInfo().IsDir() {
			if err := f.root.MkdirAll(target, 0o750); err != nil {
				return err
			}
			_ = f.own(target)
			continue
		}
		if err := f.root.MkdirAll(path.Dir(target), 0o750); err != nil {
			return err
		}
		total += int64(zf.UncompressedSize64)
		if total > MaxExtractBytes {
			return ErrTooLarge
		}
		if err := f.extractOne(zf, target, &total); err != nil {
			return err
		}
	}
	return nil
}

func (f *FS) extractOne(zf *zip.File, target string, _ *int64) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	mode := zf.Mode().Perm() & 0o666 // strip exec bits from archives by default
	if mode == 0 {
		mode = 0o640
	}
	out, err := f.root.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	// Enforce the real stream size, not the (forgeable) header value.
	n, err := io.Copy(out, io.LimitReader(rc, int64(zf.UncompressedSize64)+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > int64(zf.UncompressedSize64) {
		err = ErrTooLarge
	}
	if err != nil {
		_ = f.root.Remove(target)
		return err
	}
	return f.own(target)
}
