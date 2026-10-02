package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thimis/MelHttp/internal/gen"
	"github.com/thimis/MelHttp/internal/mbfile"
	"github.com/thimis/MelHttp/internal/melcgi"
	"github.com/thimis/MelHttp/internal/tlsutil"
)

func testSite(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	chunks, err := gen.Compile(append(melcgi.HeaderBlock("text/html; charset=utf-8"), "<h1>secure malbolge</h1>"...), gen.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := mbfile.Write(filepath.Join(root, "index.html.mb"), chunks); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeCert(t *testing.T, dir, cn string) (certFile, keyFile string, pool *x509.CertPool) {
	t.Helper()
	_, certPEM, keyPEM, err := tlsutil.SelfSigned([]string{cn, "127.0.0.1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	certFile, keyFile = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	os.WriteFile(certFile, certPEM, 0o600)
	os.WriteFile(keyFile, keyPEM, 0o600)
	pool = x509.NewCertPool()
	pool.AppendCertsFromPEM(certPEM)
	return certFile, keyFile, pool
}

// startTLS runs melhttpd with extra args and returns its HTTP and HTTPS addresses.
func startTLS(t *testing.T, args ...string) (httpAddr, httpsAddr string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	type addrs struct{ h, s net.Addr }
	ready := make(chan addrs, 1)
	done := make(chan int, 1)
	var logs syncBuffer
	all := append([]string{"-addr", "127.0.0.1:0", "-tls-addr", "127.0.0.1:0"}, args...)
	go func() {
		done <- run(ctx, all, noenv, io.Discard, &logs, func(h, s net.Addr) { ready <- addrs{h, s} })
	}()
	t.Cleanup(func() {
		cancel()
		if code := <-done; code != 0 {
			t.Errorf("exit code %d: %s", code, logs.String())
		}
	})
	select {
	case a := <-ready:
		if a.s == nil {
			t.Fatal("no HTTPS listener")
		}
		return a.h.String(), a.s.String()
	case code := <-done:
		t.Fatalf("exited with %d: %s", code, logs.String())
	case <-time.After(30 * time.Second):
		t.Fatal("did not start")
	}
	return "", ""
}

func tlsClient(pool *x509.CertPool, maxVersion uint16) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{RootCAs: pool, MaxVersion: maxVersion},
			ForceAttemptHTTP2: true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       10 * time.Second,
	}
}

func TestHTTPSWithCertificateFiles(t *testing.T) {
	root := testSite(t)
	certFile, keyFile, pool := writeCert(t, t.TempDir(), "first.test")
	httpAddr, httpsAddr := startTLS(t, "-root", root, "-tls-cert", certFile, "-tls-key", keyFile, "-hsts", "8760h")

	resp, err := tlsClient(pool, 0).Get("https://" + httpsAddr + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "<h1>secure malbolge</h1>" || resp.ProtoMajor != 2 {
		t.Fatalf("HTTPS GET: %q over HTTP/%d", body, resp.ProtoMajor)
	}
	if got := resp.Header.Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Errorf("HSTS = %q", got)
	}

	// Plain HTTP redirects, keeping path and query; health checks still answer.
	plain := tlsClient(nil, 0)
	r, err := plain.Get("http://" + httpAddr + "/about.html?x=1")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	_, port, _ := net.SplitHostPort(httpsAddr)
	if r.StatusCode != http.StatusPermanentRedirect || r.Header.Get("Location") != "https://127.0.0.1:"+port+"/about.html?x=1" {
		t.Errorf("redirect: %d %q", r.StatusCode, r.Header.Get("Location"))
	}
	if r.Header.Get("Strict-Transport-Security") != "" {
		t.Error("HSTS must not be sent over plain HTTP")
	}
	if h, err := plain.Get("http://" + httpAddr + "/healthz"); err != nil || h.StatusCode != 200 {
		t.Errorf("healthz over HTTP: %v", err)
	}

	// TLS 1.1 is refused.
	if _, err := tlsClient(pool, tls.VersionTLS11).Get("https://" + httpsAddr + "/"); err == nil {
		t.Error("TLS 1.1 handshake succeeded")
	}
}

func TestHTTPSSelfSignedNoRedirect(t *testing.T) {
	root := testSite(t)
	httpAddr, httpsAddr := startTLS(t, "-root", root, "-tls-self-signed", "-https-redirect=false", "-tls-min", "1.3")
	insecure := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	resp, err := insecure.Get("https://" + httpsAddr + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.TLS == nil || resp.TLS.Version != tls.VersionTLS13 {
		t.Fatalf("TLS state %+v", resp.TLS)
	}
	cert := resp.TLS.PeerCertificates[0]
	if !strings.Contains(cert.Subject.Organization[0], "self-signed") {
		t.Errorf("certificate %v", cert.Subject)
	}
	// Without redirect the plain listener serves the site too.
	r, err := http.Get("http://" + httpAddr + "/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if string(b) != "<h1>secure malbolge</h1>" {
		t.Errorf("plain HTTP body %q", b)
	}
}

func TestRedirectToHTTPS(t *testing.T) {
	site := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "site") })
	for _, tt := range []struct{ host, port, want string }{
		{"example.com", "", "https://example.com/p?q=1"},
		{"example.com:8080", "8443", "https://example.com:8443/p?q=1"},
		{"[::1]:8080", "", "https://[::1]/p?q=1"},
	} {
		req, _ := http.NewRequest(http.MethodGet, "http://"+tt.host+"/p?q=1", nil)
		req.Host = tt.host
		rec := &recorderW{header: http.Header{}}
		redirectToHTTPS(site, tt.port).ServeHTTP(rec, req)
		if rec.code != http.StatusPermanentRedirect || rec.header.Get("Location") != tt.want {
			t.Errorf("%s: %d %q, want %q", tt.host, rec.code, rec.header.Get("Location"), tt.want)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "http://x/", nil)
	req.Host = ""
	rec := &recorderW{header: http.Header{}}
	redirectToHTTPS(site, "").ServeHTTP(rec, req)
	if rec.code != http.StatusBadRequest {
		t.Errorf("empty host: %d", rec.code)
	}
}

type recorderW struct {
	header http.Header
	code   int
}

func (r *recorderW) Header() http.Header         { return r.header }
func (r *recorderW) Write(b []byte) (int, error) { return len(b), nil }
func (r *recorderW) WriteHeader(c int)           { r.code = c }
