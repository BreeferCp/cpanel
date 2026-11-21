//go:build !linux

package files

// own is a no-op off Linux (development only).
func (f *FS) own(string) error { return nil }
