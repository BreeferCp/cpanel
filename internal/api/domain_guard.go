package api

import (
	"context"
	"os"
	"strings"

	"github.com/breefercp/breefercp/internal/rbac"
)

// Whatever is in the domains table is delivered locally by Postfix and served
// by nginx, so adding a name is a claim on that name's mail and traffic.

// commonDomains can never be hosted here.
var commonDomains = strings.Fields(`gmail.com googlemail.com google.com youtube.com outlook.com hotmail.com live.com msn.com
	office.com office365.com microsoft.com microsoftonline.com yahoo.com ymail.com aol.com icloud.com me.com mac.com apple.com
	proton.me protonmail.com zoho.com gmx.com gmx.net mail.com mail.ru yandex.ru yandex.com qq.com 163.com fastmail.com
	facebook.com instagram.com whatsapp.com x.com twitter.com linkedin.com github.com gitlab.com amazon.com amazonaws.com
	paypal.com stripe.com cloudflare.com letsencrypt.org wordpress.org bkash.com nagad.com.bd`)

func sameOrBelow(name, parent string) bool {
	return name == parent || strings.HasSuffix(name, "."+parent)
}

// countryRegistry matches registry names such as co.uk and com.bd.
func countryRegistry(name string) bool {
	first, tld, _ := strings.Cut(name, ".")
	switch first {
	case "com", "net", "org", "gov", "edu", "co", "ac", "mil":
		return len(tld) == 2
	}
	return false
}

// claimDomain refuses names that are not the caller's to host. accountID is
// empty while the account is still being created.
func (s *Server) claimDomain(ctx context.Context, p *Principal, accountID, name string) error {
	for _, c := range commonDomains {
		if sameOrBelow(name, c) {
			return errConflict("that domain cannot be hosted on this server")
		}
	}
	if countryRegistry(name) {
		return errConflict("that is a registry suffix, not a domain")
	}
	root := p != nil && p.Role == rbac.Root
	own := []string{s.setting(ctx, "hostname"), s.setting(ctx, "nameserver1"), s.setting(ctx, "nameserver2")}
	if h, err := os.Hostname(); err == nil && strings.Contains(h, ".") {
		own = append(own, h)
	}
	for _, h := range own {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		// Only root may host the parent domain of the server's own names.
		if sameOrBelow(name, h) || (!root && sameOrBelow(h, name)) {
			return errConflict("that name belongs to this server")
		}
	}

	var n int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM domains WHERE account_id <> ? AND name LIKE ?`, accountID, "%."+name).Scan(&n); err != nil {
		return err
	}
	for parent := name; n == 0 && strings.Contains(parent, "."); _, parent, _ = strings.Cut(parent, ".") {
		if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM domains WHERE account_id <> ? AND name = ?`, accountID, parent).Scan(&n); err != nil {
			return err
		}
	}
	if n > 0 {
		return errConflict("that domain is already in use, or is part of another account's domain")
	}
	return nil
}
