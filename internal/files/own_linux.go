//go:build linux

package files

// own assigns the account's uid/gid to a path created by the daemon (which
// runs as root), so the customer can always manage their own files.
func (f *FS) own(rel string) error {
	if f.uid < 0 {
		return nil
	}
	return f.root.Lchown(rel, f.uid, f.gid)
}
