package provision

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/breefercp/breefercp/internal/system"
)

// ProtectedDir puts HTTP basic authentication on a URL path.
type ProtectedDir struct {
	ID    string
	Path  string // "/private"
	Realm string
}

// Hotlink refuses embedding of static files from other sites.
type Hotlink struct {
	Allowed    []string // extra referrer host names
	Extensions []string
	AllowEmpty bool // direct requests without a Referer
}

// HtpasswdDir holds password files outside every home, so a customer can
// neither read other customers' hashes nor point nginx at arbitrary files.
const HtpasswdDir = "/etc/nginx/breefercp-htpasswd"

var (
	reProtectPath = regexp.MustCompile(`^/[A-Za-z0-9._~@%+=,-]+(/[A-Za-z0-9._~@%+=,-]+)*/?$`)
	reRealm       = regexp.MustCompile(`^[A-Za-z0-9 ._,:()-]{1,64}$`)
	reHtUser      = regexp.MustCompile(`^[A-Za-z0-9._-]{1,32}$`)
	reExt         = regexp.MustCompile(`^[a-z0-9]{1,8}$`)
	reID          = regexp.MustCompile(`^[a-z0-9]{8,40}$`)
)

// ValidateProtectedDir checks a path and realm before they reach nginx.
func ValidateProtectedDir(d ProtectedDir) error {
	if !reProtectPath.MatchString(d.Path) || strings.Contains(d.Path, "..") || strings.HasPrefix(d.Path, "/.") {
		return fmt.Errorf("%w: path must look like /private or /members/area", system.ErrInvalid)
	}
	if !reRealm.MatchString(d.Realm) {
		return fmt.Errorf("%w: the name may use letters, digits, spaces and . , : ( ) - (max 64)", system.ErrInvalid)
	}
	return nil
}

// HtUser validates a basic-auth user name (it is written to a file nginx parses).
func HtUser(s string) error {
	if !reHtUser.MatchString(s) {
		return fmt.Errorf("%w: user names use letters, digits, . _ - (max 32)", system.ErrInvalid)
	}
	return nil
}

// BlockedCIDR validates an address or range for the IP blocker and returns
// its canonical form.
func BlockedCIDR(s string) (string, error) {
	s = strings.TrimSpace(s)
	if ip := net.ParseIP(s); ip != nil {
		return ip.String(), nil
	}
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		return "", fmt.Errorf("%w: enter an IP address or a range such as 203.0.113.0/24", system.ErrInvalid)
	}
	if ones, _ := n.Mask.Size(); ones < 8 {
		return "", fmt.Errorf("%w: ranges wider than /8 are not allowed", system.ErrInvalid)
	}
	return n.String(), nil
}

// ValidateHotlink checks host names and extensions.
func ValidateHotlink(h Hotlink) error {
	for _, d := range h.Allowed {
		if err := system.Domain(strings.TrimPrefix(d, "*.")); err != nil {
			return err
		}
	}
	if len(h.Extensions) == 0 {
		return fmt.Errorf("%w: list at least one file extension", system.ErrInvalid)
	}
	for _, e := range h.Extensions {
		if !reExt.MatchString(e) || reservedExt[e] {
			return fmt.Errorf("%w: extension %q (use letters and digits, without the dot)", system.ErrInvalid, e)
		}
	}
	return nil
}

func htpasswdPath(user, id string) string {
	return filepath.ToSlash(filepath.Join(HtpasswdDir, user, id))
}

// WriteHtpasswd replaces the password file of one protected directory.
// lines are "user:$6$..." entries whose user names were validated.
func (p *Provisioner) WriteHtpasswd(ctx context.Context, a Account, id string, users map[string]string) error {
	if err := system.SysUser(a.SysUser); err != nil {
		return err
	}
	if !reID.MatchString(id) {
		return fmt.Errorf("%w: id", system.ErrInvalid)
	}
	var b strings.Builder
	for u, h := range users {
		if HtUser(u) != nil || !strings.HasPrefix(h, "$6$") || strings.ContainsAny(h, ":\n") {
			return fmt.Errorf("%w: password entry", system.ErrInvalid)
		}
		b.WriteString(u + ":" + h + "\n")
	}
	dir := filepath.Join(HtpasswdDir, a.SysUser)
	if _, err := p.run.Run(ctx, "install", "-d", "-m", "0750", "-o", "root", "-g", p.cfg.WebUser, dir); err != nil {
		return err
	}
	path := filepath.Join(dir, id)
	if err := p.run.WriteFile(path, []byte(b.String()), 0o640); err != nil {
		return err
	}
	_, err := p.run.Run(ctx, "chgrp", p.cfg.WebUser, path)
	return err
}

// RemoveHtpasswd deletes one password file.
func (p *Provisioner) RemoveHtpasswd(a Account, id string) error {
	if system.SysUser(a.SysUser) != nil || !reID.MatchString(id) {
		return fmt.Errorf("%w: id", system.ErrInvalid)
	}
	return p.run.Remove(filepath.Join(HtpasswdDir, a.SysUser, id))
}

// hotlinkView is the template form of Hotlink.
type hotlinkView struct {
	Referers string // space separated valid_referers entries
	ExtRegex string // jpg|png|...
}

func (h *Hotlink) view(domain string, aliases []string) hotlinkView {
	refs := []string{}
	if h.AllowEmpty {
		refs = append(refs, "none", "blocked")
	}
	refs = append(refs, "server_names", domain, "*."+domain)
	refs = append(refs, aliases...)
	for _, d := range h.Allowed {
		refs = append(refs, d)
	}
	return hotlinkView{Referers: strings.Join(refs, " "), ExtRegex: strings.Join(h.Extensions, "|")}
}
