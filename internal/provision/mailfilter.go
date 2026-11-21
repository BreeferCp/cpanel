package provision

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/breefercp/breefercp/internal/system"
)

// SpamPolicy is an account's spam filter, applied to every mailbox.
// Rspamd scores each message and adds an X-Spam-Level header of asterisks
// (one per point); the Sieve script acts on it.
type SpamPolicy struct {
	Enabled   bool
	Threshold int    // 1..20 points
	Action    string // junk | discard
	Allow     []string
	Deny      []string
}

// Autoresponder is a vacation reply for one mailbox.
type Autoresponder struct {
	Subject      string
	Body         string
	IntervalDays int
	StartsOn     string // YYYY-MM-DD, optional
	EndsOn       string
}

var (
	reSender = regexp.MustCompile(`^([A-Za-z0-9._%+-]{1,64})?@[A-Za-z0-9.-]{1,253}$|^[A-Za-z0-9._%+-]{1,64}@[A-Za-z0-9.-]{1,253}$`)
	reDate   = regexp.MustCompile(`^20[0-9]{2}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$`)
)

// ValidateSpamPolicy checks the policy and its sender lists.
func ValidateSpamPolicy(p SpamPolicy) error {
	if p.Threshold < 1 || p.Threshold > 20 {
		return fmt.Errorf("%w: the score threshold must be between 1 and 20", system.ErrInvalid)
	}
	if p.Action != "junk" && p.Action != "discard" {
		return fmt.Errorf("%w: action must be junk or discard", system.ErrInvalid)
	}
	if len(p.Allow) > 500 || len(p.Deny) > 500 {
		return fmt.Errorf("%w: at most 500 entries per list", system.ErrInvalid)
	}
	for _, s := range append(append([]string{}, p.Allow...), p.Deny...) {
		if !reSender.MatchString(s) {
			return fmt.Errorf("%w: %q is not an address or @domain", system.ErrInvalid, s)
		}
	}
	return nil
}

// ValidateAutoresponder checks lengths and dates.
func ValidateAutoresponder(a Autoresponder) error {
	if strings.TrimSpace(a.Subject) == "" || len(a.Subject) > 200 || strings.ContainsAny(a.Subject, "\r\n") {
		return fmt.Errorf("%w: a one-line subject of up to 200 characters is required", system.ErrInvalid)
	}
	if strings.TrimSpace(a.Body) == "" || len(a.Body) > 8000 {
		return fmt.Errorf("%w: the message is required (max 8000 characters)", system.ErrInvalid)
	}
	if a.IntervalDays < 1 || a.IntervalDays > 30 {
		return fmt.Errorf("%w: reply interval must be 1 to 30 days", system.ErrInvalid)
	}
	for _, d := range []string{a.StartsOn, a.EndsOn} {
		if d != "" && !reDate.MatchString(d) {
			return fmt.Errorf("%w: dates use YYYY-MM-DD", system.ErrInvalid)
		}
	}
	if a.StartsOn != "" && a.EndsOn != "" && a.EndsOn < a.StartsOn {
		return fmt.Errorf("%w: the end date is before the start date", system.ErrInvalid)
	}
	return nil
}

// sieveString quotes s as a Sieve quoted string (RFC 5228 §2.4.2).
func sieveString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return `"` + s + `"`
}

func sieveList(items []string) string {
	q := make([]string, len(items))
	for i, it := range items {
		q[i] = sieveString(it)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// RenderSieve builds the managed Sieve script for one mailbox. Order:
// allow list (keep), deny list (discard), user filters, spam rule, then the
// autoresponder,
// so spam never triggers an automatic reply.
func RenderSieve(spam SpamPolicy, ar *Autoresponder, filters ...MailFilter) (string, error) {
	for _, f := range filters {
		if err := ValidateMailFilter(f); err != nil {
			return "", err
		}
	}
	if spam.Enabled {
		if err := ValidateSpamPolicy(spam); err != nil {
			return "", err
		}
	}
	if ar != nil {
		if err := ValidateAutoresponder(*ar); err != nil {
			return "", err
		}
	}
	req := []string{"fileinto", "mailbox"}
	if ar != nil {
		req = append(req, "vacation")
		if ar.StartsOn != "" || ar.EndsOn != "" {
			req = append(req, "date", "relational")
		}
	}
	var b strings.Builder
	b.WriteString("# Managed by BreeferCP. Changes are overwritten.\n")
	b.WriteString("require " + sieveList(req) + ";\n\n")
	if spam.Enabled {
		if len(spam.Allow) > 0 {
			b.WriteString("if address :all :contains \"from\" " + sieveList(spam.Allow) + " {\n    keep;\n    stop;\n}\n")
		}
		if len(spam.Deny) > 0 {
			b.WriteString("if address :all :contains \"from\" " + sieveList(spam.Deny) + " {\n    discard;\n    stop;\n}\n")
		}
	}
	// User filters run after the sender lists and before the spam score, in
	// the order the customer arranged them.
	for _, f := range filters {
		b.WriteString(f.sieve())
	}
	if spam.Enabled {
		stars := strings.Repeat("*", spam.Threshold)
		b.WriteString("if header :contains \"X-Spam-Level\" " + sieveString(stars) + " {\n")
		if spam.Action == "discard" {
			b.WriteString("    discard;\n")
		} else {
			b.WriteString("    fileinto :create \"Junk\";\n")
		}
		b.WriteString("    stop;\n}\n")
	}
	if ar != nil {
		var conds []string
		if ar.StartsOn != "" {
			conds = append(conds, `currentdate :value "ge" "date" `+sieveString(ar.StartsOn))
		}
		if ar.EndsOn != "" {
			conds = append(conds, `currentdate :value "le" "date" `+sieveString(ar.EndsOn))
		}
		vac := fmt.Sprintf("vacation :days %d :subject %s %s;\n", ar.IntervalDays, sieveString(ar.Subject), sieveString(ar.Body))
		if len(conds) > 0 {
			b.WriteString("if allof(" + strings.Join(conds, ", ") + ") {\n    " + vac + "}\n")
		} else {
			b.WriteString(vac)
		}
	}
	return b.String(), nil
}
