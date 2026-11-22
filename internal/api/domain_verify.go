package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/breefercp/breefercp/internal/rbac"
)

// dnsLookup is the part of net.Resolver that ownership checks use.
type dnsLookup interface {
	LookupNS(ctx context.Context, name string) ([]*net.NS, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupHost(ctx context.Context, name string) ([]string, error)
}

const verifyLabel = "_breefercp."

// verifyValue differs per account, so a record published for one account
// proves nothing for another. It does not need to be secret.
func verifyValue(accountID, name string) string {
	sum := sha256.Sum256([]byte(accountID + ":" + name))
	return "breefercp-verify=" + hex.EncodeToString(sum[:16])
}

// verifyDomain makes a customer show control of a domain before it is hosted:
// it uses this server's nameservers, points at this server, or carries a TXT
// record. Administrators can switch the check off with domain_verification.
func (s *Server) verifyDomain(ctx context.Context, p *Principal, accountID, name string) error {
	if s.resolver == nil || (p != nil && p.Role == rbac.Root) || s.setting(ctx, "domain_verification") == "off" {
		return nil
	}
	owned, err := s.accountDomains(ctx, accountID)
	if err != nil {
		return err
	}
	for _, d := range owned {
		if strings.HasSuffix(name, "."+d) {
			return nil // below a domain the account already has
		}
	}

	lctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	fqdn := name + "." // the dot keeps search domains out
	norm := func(v string) string { return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(v), ".")) }

	// A lookup that fails proves nothing, so errors just move to the next proof.
	want := verifyValue(accountID, name)
	if recs, err := s.resolver.LookupTXT(lctx, verifyLabel+fqdn); err == nil {
		for _, v := range recs {
			if strings.TrimSpace(v) == want {
				return nil
			}
		}
	}
	ours := map[string]bool{}
	for _, n := range s.nameservers(ctx) {
		ours[norm(n)] = true
	}
	if len(ours) > 0 {
		if recs, err := s.resolver.LookupNS(lctx, fqdn); err == nil && len(recs) > 0 {
			all := true
			for _, r := range recs {
				all = all && ours[norm(r.Host)]
			}
			if all {
				return nil
			}
		}
	}
	// A loopback or private server_ip would match names anybody can create.
	if ip := net.ParseIP(s.setting(ctx, "server_ip")); ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() {
		if addrs, err := s.resolver.LookupHost(lctx, fqdn); err == nil {
			for _, v := range addrs {
				if ip.Equal(net.ParseIP(v)) {
					return nil
				}
			}
		}
	}
	return &apiError{409, "domain_unverified", fmt.Sprintf(
		"Show that you control %s first: point it at this server (nameservers or A record), or add a TXT record named %s%s with the value %s. Then try again; DNS changes can take a while.",
		name, verifyLabel, name, want)}
}
