package provision

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/acme"

	"github.com/breefercp/breefercp/internal/system"
)

// Certificate is an issued certificate chain and its private key (PEM).
type Certificate struct {
	CertPEM  []byte
	KeyPEM   []byte
	NotAfter time.Time
}

const (
	letsEncryptProd    = "https://acme-v02.api.letsencrypt.org/directory"
	letsEncryptStaging = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// IssueLetsEncrypt obtains a certificate via ACME http-01 using the shared
// webroot that every vhost serves under /.well-known/acme-challenge/.
func (p *Provisioner) IssueLetsEncrypt(ctx context.Context, email string, names []string, staging bool) (*Certificate, error) {
	if len(names) == 0 {
		return nil, errors.New("no domain names given")
	}
	for _, n := range names {
		if err := system.Domain(n); err != nil {
			return nil, err
		}
	}
	if p.run.DryRun {
		return p.selfSigned(names)
	}
	acctKey, err := p.acmeAccountKey()
	if err != nil {
		return nil, err
	}
	dir := letsEncryptProd
	if staging {
		dir = letsEncryptStaging
	}
	cl := &acme.Client{Key: acctKey, DirectoryURL: dir, UserAgent: "BreeferCP"}
	if _, err := cl.Register(ctx, &acme.Account{Contact: []string{"mailto:" + email}}, acme.AcceptTOS); err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
		return nil, fmt.Errorf("acme register: %w", err)
	}
	ids := make([]acme.AuthzID, len(names))
	for i, n := range names {
		ids[i] = acme.AuthzID{Type: "dns", Value: n}
	}
	order, err := cl.AuthorizeOrder(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("acme order: %w", err)
	}
	webroot := filepath.Join(p.cfg.DataDir, "acme-webroot", ".well-known", "acme-challenge")
	if err := os.MkdirAll(webroot, 0o755); err != nil {
		return nil, err
	}
	for _, url := range order.AuthzURLs {
		if err := p.solveHTTP01(ctx, cl, url, webroot); err != nil {
			return nil, err
		}
	}
	if _, err := cl.WaitOrder(ctx, order.URI); err != nil {
		return nil, fmt.Errorf("acme wait order: %w", err)
	}
	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{DNSNames: names}, certKey)
	if err != nil {
		return nil, err
	}
	der, _, err := cl.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		return nil, fmt.Errorf("acme finalize: %w", err)
	}
	return buildCertificate(der, certKey)
}

func (p *Provisioner) solveHTTP01(ctx context.Context, cl *acme.Client, authzURL, webroot string) error {
	az, err := cl.GetAuthorization(ctx, authzURL)
	if err != nil {
		return err
	}
	if az.Status == acme.StatusValid {
		return nil
	}
	var chal *acme.Challenge
	for _, c := range az.Challenges {
		if c.Type == "http-01" {
			chal = c
		}
	}
	if chal == nil {
		return fmt.Errorf("no http-01 challenge offered for %s", az.Identifier.Value)
	}
	resp, err := cl.HTTP01ChallengeResponse(chal.Token)
	if err != nil {
		return err
	}
	file := filepath.Join(webroot, chal.Token)
	if err := os.WriteFile(file, []byte(resp), 0o644); err != nil {
		return err
	}
	defer os.Remove(file)
	if _, err := cl.Accept(ctx, chal); err != nil {
		return err
	}
	if _, err := cl.WaitAuthorization(ctx, az.URI); err != nil {
		return fmt.Errorf("challenge for %s failed: %w", az.Identifier.Value, err)
	}
	return nil
}

func (p *Provisioner) acmeAccountKey() (crypto.Signer, error) {
	path := filepath.Join(p.cfg.DataDir, "acme-account.key")
	if b, err := os.ReadFile(path); err == nil {
		blk, _ := pem.Decode(b)
		if blk != nil {
			return x509.ParseECPrivateKey(blk.Bytes)
		}
	}
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalECPrivateKey(k)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(p.cfg.DataDir, 0o700); err != nil {
		return nil, err
	}
	return k, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600)
}

func buildCertificate(der [][]byte, key *ecdsa.PrivateKey) (*Certificate, error) {
	leaf, err := x509.ParseCertificate(der[0])
	if err != nil {
		return nil, err
	}
	var chain []byte
	for _, d := range der {
		chain = append(chain, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: d})...)
	}
	kder, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	return &Certificate{
		CertPEM:  chain,
		KeyPEM:   pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kder}),
		NotAfter: leaf.NotAfter,
	}, nil
}

func (p *Provisioner) selfSigned(names []string) (*Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: bigSerial(),
		DNSNames:     names,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(0, 0, 90),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	return buildCertificate([][]byte{der}, key)
}

// InstallCertificate writes cert and key for a domain with strict modes.
func (p *Provisioner) InstallCertificate(domain string, c *Certificate) (certPath, keyPath string, err error) {
	if err := system.Domain(domain); err != nil {
		return "", "", err
	}
	dir := filepath.Join("/etc/breefercp/certs", domain)
	certPath = filepath.Join(dir, "fullchain.pem")
	keyPath = filepath.Join(dir, "privkey.pem")
	if err := p.run.WriteFile(certPath, c.CertPEM, 0o644); err != nil {
		return "", "", err
	}
	return certPath, keyPath, p.run.WriteFile(keyPath, c.KeyPEM, 0o600)
}
