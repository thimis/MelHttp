package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePair(t *testing.T, dir, cn string) (string, string) {
	t.Helper()
	_, certPEM, keyPEM, err := SelfSigned([]string{cn, "127.0.0.1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	c, k := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(c, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(k, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return c, k
}

func leafCN(t *testing.T, c *tls.Certificate) string {
	t.Helper()
	x, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return x.Subject.CommonName
}

func TestSelfSigned(t *testing.T) {
	cert, _, _, err := SelfSigned([]string{"example.test", "::1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	x, _ := x509.ParseCertificate(cert.Certificate[0])
	if x.DNSNames[0] != "example.test" || !x.IPAddresses[0].Equal([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}) {
		t.Fatalf("SANs: %v %v", x.DNSNames, x.IPAddresses)
	}
	if err := x.VerifyHostname("example.test"); err != nil {
		t.Fatal(err)
	}
}

func TestReloaderPicksUpRenewals(t *testing.T) {
	dir := t.TempDir()
	c, k := writePair(t, dir, "first.test")
	r, err := NewReloader(c, k, 0)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetCertificate(nil)
	if leafCN(t, got) != "first.test" {
		t.Fatalf("CN %s", leafCN(t, got))
	}
	time.Sleep(20 * time.Millisecond)
	writePair(t, dir, "second.test")
	got, _ = r.GetCertificate(nil)
	if leafCN(t, got) != "second.test" {
		t.Fatalf("after renewal CN %s", leafCN(t, got))
	}
	// A broken renewal keeps the old certificate.
	time.Sleep(20 * time.Millisecond)
	os.WriteFile(k, []byte("half-written"), 0o600)
	got, err = r.GetCertificate(nil)
	if err != nil || leafCN(t, got) != "second.test" {
		t.Fatalf("broken renewal: %v %v", err, got)
	}
	if _, err := NewReloader(c, filepath.Join(dir, "missing.pem"), 0); err == nil {
		t.Fatal("accepted a missing key")
	}
}

func TestNewACME(t *testing.T) {
	if _, err := NewACME(ACMEConfig{CacheDir: t.TempDir()}); err == nil {
		t.Error("accepted no domains")
	}
	if _, err := NewACME(ACMEConfig{Domains: []string{"a.test"}}); err == nil {
		t.Error("accepted no cache dir")
	}
	m, err := NewACME(ACMEConfig{Domains: []string{"a.test"}, CacheDir: filepath.Join(t.TempDir(), "cache")})
	if err != nil || m.Client != nil {
		t.Fatalf("default manager: %v %v", err, m.Client)
	}
	if err := m.HostPolicy(t.Context(), "evil.test"); err == nil {
		t.Error("host policy allowed an unlisted domain")
	}
	dir := t.TempDir()
	c, _ := writePair(t, dir, "ca.test")
	m, err = NewACME(ACMEConfig{Domains: []string{"a.test"}, CacheDir: dir, Directory: "https://ca.test/dir", CARoot: c})
	if err != nil || m.Client == nil || m.Client.DirectoryURL != "https://ca.test/dir" {
		t.Fatalf("custom directory: %v", err)
	}
	bad := filepath.Join(dir, "bad.pem")
	os.WriteFile(bad, []byte("nope"), 0o600)
	if _, err := NewACME(ACMEConfig{Domains: []string{"a.test"}, CacheDir: dir, CARoot: bad}); err == nil {
		t.Error("accepted a CA root without certificates")
	}
}

func TestMinVersion(t *testing.T) {
	for s, want := range map[string]uint16{"": tls.VersionTLS12, "1.2": tls.VersionTLS12, "1.3": tls.VersionTLS13} {
		if v, err := MinVersion(s); err != nil || v != want {
			t.Errorf("MinVersion(%q) = %v, %v", s, v, err)
		}
	}
	if _, err := MinVersion("1.0"); err == nil {
		t.Error("accepted TLS 1.0")
	}
}
