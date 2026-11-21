package provision

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"path/filepath"

	"github.com/breefercp/breefercp/internal/system"
)

// EnsureMaildir creates the Maildir for one mailbox inside the owning
// account's home, so mail storage counts against and is isolated by the
// account's own uid.
func (p *Provisioner) EnsureMaildir(ctx context.Context, a Account, domain, local string) error {
	if err := system.Domain(domain); err != nil {
		return err
	}
	if err := system.LocalPart(local); err != nil {
		return err
	}
	base := filepath.Join(a.Home, "mail", domain, local)
	for _, sub := range []string{"", "cur", "new", "tmp"} {
		if _, err := p.run.Run(ctx, "install", "-d", "-m", "0700", "-o", a.SysUser, "-g", a.SysUser, filepath.Join(base, sub)); err != nil {
			return err
		}
	}
	return nil
}

// RemoveMaildir deletes a mailbox's stored mail.
func (p *Provisioner) RemoveMaildir(ctx context.Context, a Account, domain, local string) error {
	if err := system.Domain(domain); err != nil {
		return err
	}
	if err := system.LocalPart(local); err != nil {
		return err
	}
	_, err := p.run.Run(ctx, "rm", "-rf", "--", filepath.Join(a.Home, "mail", domain, local))
	return err
}

// DKIM holds a generated signing key pair.
type DKIM struct {
	Selector string
	// DNSName is the TXT record name relative to the zone.
	DNSName string
	// DNSValue is the TXT record content (already quoted for PowerDNS).
	DNSValue string
}

// GenerateDKIM creates a 2048-bit key for domain, installs the private key
// for OpenDKIM and returns the DNS record to publish.
func (p *Provisioner) GenerateDKIM(ctx context.Context, domain string) (DKIM, error) {
	if err := system.Domain(domain); err != nil {
		return DKIM{}, err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return DKIM{}, err
	}
	priv := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return DKIM{}, err
	}
	const selector = "breefer"
	dir := filepath.Join("/etc/opendkim/keys", domain)
	if err := p.run.WriteFile(filepath.Join(dir, selector+".private"), priv, 0o600); err != nil {
		return DKIM{}, err
	}
	if _, err := p.run.Run(ctx, "chown", "-R", "opendkim:opendkim", dir); err != nil {
		return DKIM{}, err
	}
	return DKIM{
		Selector: selector,
		DNSName:  selector + "._domainkey",
		DNSValue: fmt.Sprintf(`"v=DKIM1; k=rsa; p=%s"`, base64.StdEncoding.EncodeToString(pubDER)),
	}, nil
}
