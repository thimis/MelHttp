// Package tlsutil provides melhttpd's TLS setups: certificate files that are
// reloaded when they change, automatic certificates via ACME (Let's Encrypt
// or any RFC 8555 CA), and self-signed certificates for local development.
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// MinVersion parses "1.2" or "1.3".
func MinVersion(s string) (uint16, error) {
	switch s {
	case "", "1.2":
		return tls.VersionTLS12, nil
	case "1.3":
		return tls.VersionTLS13, nil
	}
	return 0, fmt.Errorf("unsupported minimum TLS version %q (use 1.2 or 1.3)", s)
}

// Reloader serves a certificate from files and reloads it when the files
// change, so renewals (certbot, cert-manager, …) need no restart.
type Reloader struct {
	certFile, keyFile string
	interval          time.Duration

	mu      sync.Mutex
	cert    *tls.Certificate
	stamp   string
	checked time.Time
}

// NewReloader loads the certificate and key; interval is how often the files
// are checked for changes (0 means every handshake).
func NewReloader(certFile, keyFile string, interval time.Duration) (*Reloader, error) {
	r := &Reloader{certFile: certFile, keyFile: keyFile, interval: interval}
	if err := r.reload(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Reloader) fileStamp() (string, error) {
	var s string
	for _, f := range []string{r.certFile, r.keyFile} {
		fi, err := os.Stat(f)
		if err != nil {
			return "", err
		}
		s += fmt.Sprintf("%d:%d;", fi.Size(), fi.ModTime().UnixNano())
	}
	return s, nil
}

func (r *Reloader) reload() error {
	stamp, err := r.fileStamp()
	if err != nil {
		return err
	}
	cert, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return err
	}
	r.cert, r.stamp, r.checked = &cert, stamp, time.Now()
	return nil
}

// GetCertificate implements tls.Config.GetCertificate. If a reload fails
// (for example while a renewal is half written) the previous certificate
// keeps being served.
func (r *Reloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.checked) >= r.interval {
		r.checked = time.Now()
		if stamp, err := r.fileStamp(); err == nil && stamp != r.stamp {
			r.reload()
		}
	}
	return r.cert, nil
}

// ACMEConfig configures automatic certificates.
type ACMEConfig struct {
	Domains   []string
	Email     string
	CacheDir  string
	Directory string // ACME directory URL; empty means Let's Encrypt production
	CARoot    string // PEM file of extra roots to trust for the directory (private CAs, testing)
}

// NewACME returns an autocert manager that only requests certificates for
// the configured domains. Using it implies accepting the CA's terms of service.
func NewACME(c ACMEConfig) (*autocert.Manager, error) {
	if len(c.Domains) == 0 {
		return nil, errors.New("acme: no domains configured")
	}
	if c.CacheDir == "" {
		return nil, errors.New("acme: a certificate cache directory is required")
	}
	if err := os.MkdirAll(c.CacheDir, 0o700); err != nil {
		return nil, fmt.Errorf("acme cache: %w", err)
	}
	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(c.CacheDir),
		HostPolicy: autocert.HostWhitelist(c.Domains...),
		Email:      c.Email,
	}
	if c.Directory != "" || c.CARoot != "" {
		client := &acme.Client{DirectoryURL: c.Directory}
		if c.CARoot != "" {
			pemData, err := os.ReadFile(c.CARoot)
			if err != nil {
				return nil, fmt.Errorf("acme CA root: %w", err)
			}
			pool, err := x509.SystemCertPool()
			if err != nil || pool == nil {
				pool = x509.NewCertPool()
			}
			if !pool.AppendCertsFromPEM(pemData) {
				return nil, fmt.Errorf("acme CA root %s: no certificates found", c.CARoot)
			}
			client.HTTPClient = &http.Client{
				Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}},
				Timeout:   60 * time.Second,
			}
		}
		m.Client = client
	}
	return m, nil
}

// SelfSigned returns a fresh self-signed ECDSA certificate (and its PEM
// encodings) valid for hosts, which may be DNS names or IP addresses.
func SelfSigned(hosts []string, validFor time.Duration) (tls.Certificate, []byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{Organization: []string{"MelHttp self-signed"}, CommonName: hosts[0]},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(validFor),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return tls.Certificate{}, nil, nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	return cert, certPEM, keyPEM, err
}
