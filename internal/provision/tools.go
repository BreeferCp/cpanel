package provision

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/breefercp/breefercp/internal/system"
)

// ---- email filters ----------------------------------------------------------

// MailFilter is one user-defined Sieve rule.
type MailFilter struct {
	Name   string
	Field  string // from | to | subject | any
	Op     string // contains | not_contains | is | begins | ends
	Value  string
	Action string // fileinto | discard | redirect | keep
	Dest   string // folder for fileinto, address for redirect
}

var (
	reFolder   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,63}$`)
	reAddress  = regexp.MustCompile(`^[A-Za-z0-9._%+-]{1,64}@[A-Za-z0-9-]{1,63}(\.[A-Za-z0-9-]{1,63})+$`)
	reFilterNm = regexp.MustCompile(`^[\pL\pN ._,:()'-]{1,64}$`)
)

// ValidAddress reports whether s is a plain email address.
func ValidAddress(s string) bool { return reAddress.MatchString(s) && len(s) <= 254 }

// ValidateMailFilter checks a rule before it is stored or rendered.
func ValidateMailFilter(f MailFilter) error {
	if !reFilterNm.MatchString(f.Name) {
		return fmt.Errorf("%w: give the filter a short name (letters, digits, spaces)", system.ErrInvalid)
	}
	switch f.Field {
	case "from", "to", "subject", "any":
	default:
		return fmt.Errorf("%w: field must be from, to, subject or any", system.ErrInvalid)
	}
	switch f.Op {
	case "contains", "not_contains", "is", "begins", "ends":
	default:
		return fmt.Errorf("%w: unknown comparison", system.ErrInvalid)
	}
	if f.Value == "" || len(f.Value) > 200 || strings.ContainsAny(f.Value, "\r\n\x00") {
		return fmt.Errorf("%w: the value must be one line of up to 200 characters", system.ErrInvalid)
	}
	switch f.Action {
	case "fileinto":
		if !reFolder.MatchString(f.Dest) || strings.Contains(f.Dest, "..") {
			return fmt.Errorf("%w: folder names use letters, digits, spaces and . _ -", system.ErrInvalid)
		}
	case "redirect":
		if !ValidAddress(f.Dest) {
			return fmt.Errorf("%w: enter a valid address to redirect to", system.ErrInvalid)
		}
	case "discard", "keep":
	default:
		return fmt.Errorf("%w: action must be fileinto, redirect, discard or keep", system.ErrInvalid)
	}
	return nil
}

// sieveMatchEscape escapes the Sieve :matches wildcards in a literal.
func sieveMatchEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `*`, `\*`)
	return strings.ReplaceAll(s, `?`, `\?`)
}

func (f MailFilter) sieve() string {
	var test string
	headers := map[string]string{"from": `"from"`, "to": `["to", "cc"]`, "subject": `"subject"`, "any": `["from", "to", "cc", "subject", "reply-to"]`}[f.Field]
	switch f.Op {
	case "contains", "not_contains":
		test = "header :contains " + headers + " " + sieveString(f.Value)
	case "is":
		test = "header :is " + headers + " " + sieveString(f.Value)
	case "begins":
		test = "header :matches " + headers + " " + sieveString(sieveMatchEscape(f.Value)+"*")
	case "ends":
		test = "header :matches " + headers + " " + sieveString("*"+sieveMatchEscape(f.Value))
	}
	if f.Op == "not_contains" {
		test = "not " + test
	}
	var act string
	switch f.Action {
	case "fileinto":
		act = "fileinto :create " + sieveString(f.Dest) + ";"
	case "redirect":
		act = "redirect " + sieveString(f.Dest) + ";"
	case "discard":
		act = "discard;"
	case "keep":
		act = "keep;"
	}
	return "# " + strings.ReplaceAll(f.Name, "\n", " ") + "\nif " + test + " {\n    " + act + "\n    stop;\n}\n"
}

// ---- directory indexes, MIME types, compression ----------------------------

var (
	reMimeExt  = regexp.MustCompile(`^[a-z0-9]{1,16}$`)
	reMimeType = regexp.MustCompile(`^[a-z]{1,30}/[a-z0-9][a-z0-9.+-]{0,100}$`)
	// Extensions that would change how code is executed or hidden.
	reservedExt = map[string]bool{"php": true, "phtml": true, "php3": true, "php4": true, "php5": true, "php7": true, "php8": true, "phar": true, "cgi": true, "pl": true, "py": true, "sh": true, "htaccess": true, "htpasswd": true}
)

// MimeType maps a file extension to a Content-Type.
type MimeType struct{ Ext, Type string }

// ValidateMime checks a custom MIME type.
func ValidateMime(m MimeType) error {
	if !reMimeExt.MatchString(m.Ext) || reservedExt[m.Ext] {
		return fmt.Errorf("%w: extension must be 1-16 lowercase letters or digits (scripts are not allowed)", system.ErrInvalid)
	}
	if !reMimeType.MatchString(m.Type) {
		return fmt.Errorf("%w: type must look like application/x-example", system.ErrInvalid)
	}
	return nil
}

// ValidateIndexPath checks a directory path for an index listing.
func ValidateIndexPath(p string) error {
	if p == "/" {
		return nil
	}
	if !reProtectPath.MatchString(p) || strings.Contains(p, "..") || strings.HasPrefix(p, "/.") {
		return fmt.Errorf("%w: path must look like / or /downloads", system.ErrInvalid)
	}
	return nil
}

// Compression is the gzip policy for an account's sites.
type Compression struct {
	Mode  string   // off | all | custom
	Types []string // custom MIME types
}

// DefaultGzipTypes are compressed in "all" mode (text/html is always on).
var DefaultGzipTypes = []string{"text/plain", "text/css", "text/xml", "text/javascript", "application/javascript", "application/json", "application/xml", "application/rss+xml", "application/atom+xml", "image/svg+xml", "font/ttf", "font/otf", "application/vnd.ms-fontobject"}

// ValidateCompression checks the policy.
func ValidateCompression(c Compression) error {
	switch c.Mode {
	case "off", "all":
	case "custom":
		if len(c.Types) == 0 || len(c.Types) > 50 {
			return fmt.Errorf("%w: list between 1 and 50 MIME types", system.ErrInvalid)
		}
		for _, t := range c.Types {
			if !reMimeType.MatchString(t) {
				return fmt.Errorf("%w: %q is not a MIME type", system.ErrInvalid, t)
			}
		}
	default:
		return fmt.Errorf("%w: mode must be off, all or custom", system.ErrInvalid)
	}
	return nil
}

func (c Compression) types() string {
	t := c.Types
	if c.Mode == "all" {
		t = DefaultGzipTypes
	}
	out := make([]string, 0, len(t))
	for _, x := range t {
		if x != "text/html" { // nginx always compresses text/html; listing it warns
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

type mimeView struct{ Ext, Type string }

// mimeGroups groups extensions by type so each type renders one location.
func mimeGroups(ms []MimeType) ([]mimeView, error) {
	by := map[string][]string{}
	for _, m := range ms {
		if err := ValidateMime(m); err != nil {
			return nil, err
		}
		by[m.Type] = append(by[m.Type], m.Ext)
	}
	out := make([]mimeView, 0, len(by))
	for t, exts := range by {
		sort.Strings(exts)
		out = append(out, mimeView{Ext: strings.Join(exts, "|"), Type: t})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out, nil
}
