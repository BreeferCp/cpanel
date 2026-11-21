package system

import (
	"errors"
	"net"
	"regexp"
	"strings"
)

// Every value that reaches a template, a config file or a command line must
// pass through one of these validators first. They are deliberately strict
// allow-lists rather than escape routines.

var (
	reSysUser  = regexp.MustCompile(`^[a-z][a-z0-9]{2,15}$`)
	reDBName   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	reLabel    = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	reLocal    = regexp.MustCompile(`^[a-z0-9][a-z0-9._+-]{0,63}$`)
	rePHP      = regexp.MustCompile(`^[78]\.[0-9]$`)
	reCronPart = regexp.MustCompile(`^(\*|[0-9]{1,2}|\*/[0-9]{1,2}|[0-9]{1,2}-[0-9]{1,2}(/[0-9]{1,2})?|[0-9]{1,2}(,[0-9]{1,2})+)$`)
)

// reserved system names that must never become hosting accounts.
var reservedUsers = map[string]bool{
	"root": true, "daemon": true, "bin": true, "sys": true, "mysql": true, "postgres": true,
	"nginx": true, "www": true, "mail": true, "postfix": true, "dovecot": true, "admin": true,
	"nobody": true, "breefer": true, "pdns": true, "named": true, "ftp": true, "sshd": true,
}

// ErrInvalid marks validation failures; the message is safe to show users.
var ErrInvalid = errors.New("invalid value")

func bad(msg string) error { return errors.Join(ErrInvalid, errors.New(msg)) }

// SysUser validates a Linux account name.
func SysUser(s string) error {
	if !reSysUser.MatchString(s) {
		return bad("username must be 3-16 chars: lowercase letters and digits, starting with a letter")
	}
	if reservedUsers[s] {
		return bad("username is reserved")
	}
	return nil
}

// DBName validates a database or database-user name.
func DBName(s string) error {
	if !reDBName.MatchString(s) {
		return bad("name must be lowercase letters, digits or underscore, starting with a letter (max 32)")
	}
	return nil
}

// Domain validates an RFC 1035 hostname with at least two labels.
func Domain(s string) error {
	s = strings.ToLower(s)
	if len(s) > 253 || !strings.Contains(s, ".") {
		return bad("enter a fully qualified domain name")
	}
	if net.ParseIP(s) != nil {
		return bad("IP addresses are not accepted here")
	}
	for _, l := range strings.Split(s, ".") {
		if !reLabel.MatchString(l) {
			return bad("domain contains an invalid label")
		}
	}
	return nil
}

// LocalPart validates the part of an e-mail address before the @.
func LocalPart(s string) error {
	if !reLocal.MatchString(strings.ToLower(s)) {
		return bad("invalid mailbox name")
	}
	return nil
}

// PHPVersion validates a major.minor PHP version string.
func PHPVersion(s string) error {
	if !rePHP.MatchString(s) {
		return bad("invalid PHP version")
	}
	return nil
}

// Cron validates a five-field schedule. Only digits, *, ranges, steps and
// lists are allowed, so nothing in it can break out of the crontab line.
func Cron(s string) error {
	f := strings.Fields(s)
	if len(f) != 5 {
		return bad("schedule needs five fields")
	}
	for _, p := range f {
		if !reCronPart.MatchString(p) {
			return bad("unsupported schedule syntax")
		}
	}
	return nil
}

// OneLine rejects control characters; used for values written into
// line-oriented config files (cron commands, comments).
func OneLine(s string, max int) error {
	if s == "" || len(s) > max {
		return bad("value is empty or too long")
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return bad("control characters are not allowed")
		}
	}
	return nil
}

// Port validates a TCP/UDP port number string (or range a:b).
func Port(s string) error {
	if s == "" {
		return nil
	}
	if !regexp.MustCompile(`^[0-9]{1,5}(:[0-9]{1,5})?$`).MatchString(s) {
		return bad("invalid port")
	}
	return nil
}

// CIDR validates an IP or CIDR.
func CIDR(s string) error {
	if s == "" {
		return nil
	}
	if net.ParseIP(s) != nil {
		return nil
	}
	if _, _, err := net.ParseCIDR(s); err != nil {
		return bad("invalid IP or CIDR")
	}
	return nil
}
