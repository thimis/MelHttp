// Command melhttpd serves a site whose pages are Malbolge programs.
//
//	melc build ./my-site -o ./site
//	melhttpd -root ./site -addr :8080
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/thimis/MelHttp/internal/config"
	"github.com/thimis/MelHttp/internal/server"
	"github.com/thimis/MelHttp/internal/tlsutil"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if isWindowsService() {
		name := "melhttpd"
		if c, err := config.Parse(os.Args[1:], os.Getenv, io.Discard); err == nil {
			name = c.ServiceName
		}
		if err := runAsService(name, func(ctx context.Context) int {
			return run(ctx, os.Args[1:], os.Getenv, io.Discard, io.Discard, nil)
		}); err != nil {
			os.Exit(1)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr, nil)
	stop()
	os.Exit(code)
}

// run is main without process globals. If ready is non-nil it receives the
// listening addresses (https is nil without HTTPS) once they accept connections.
func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer, ready func(http, https net.Addr)) int {
	cfg, err := config.Parse(args, getenv, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(stderr, "melhttpd:", err)
		return 2
	}
	if cfg.Version {
		fmt.Fprintln(stdout, "melhttpd", version)
		return 0
	}
	if cfg.Healthcheck {
		return healthcheck(ctx, cfg.HealthcheckURL(), stderr)
	}
	if cfg.Service != "" {
		stored := args
		if cfg.Service == "install" && cfg.LogFile == "" {
			if exe, err := os.Executable(); err == nil {
				stored = append(stored, "-log-file", filepath.Join(filepath.Dir(exe), cfg.ServiceName+".log"))
			}
		}
		if err := controlService(cfg.Service, cfg.ServiceName, stored); err != nil {
			fmt.Fprintln(stderr, "melhttpd:", err)
			return 1
		}
		fmt.Fprintf(stdout, "melhttpd: service %q: %s done\n", cfg.ServiceName, cfg.Service)
		return 0
	}
	if cfg.LogFile != "" {
		f, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		if err != nil {
			fmt.Fprintln(stderr, "melhttpd:", err)
			return 1
		}
		defer f.Close()
		stderr = f
	}

	var handler slog.Handler = slog.NewTextHandler(stderr, nil)
	if cfg.LogFormat == "json" {
		handler = slog.NewJSONHandler(stderr, nil)
	}
	log := slog.New(handler)
	cfg.Server.Logger = log

	srv, err := server.New(cfg.Server)
	if err != nil {
		log.Error("cannot open site", "root", cfg.Server.Root, "error", err)
		return 1
	}
	defer srv.Close()
	if cfg.Server.Warm {
		st := srv.Warm(ctx)
		log.Info("warm-up", "programs", st.Programs, "cached", st.Cached, "dynamic", st.Dynamic,
			"failed", st.Failed, "steps", st.Steps, "bytes", st.Bytes, "duration", st.Duration.Round(time.Millisecond))
	}

	tlsConfig, acmeMgr, err := setupTLS(cfg.TLS, log)
	if err != nil {
		log.Error("tls", "error", err)
		return 1
	}
	newServer := func(h http.Handler) *http.Server {
		return &http.Server{
			Handler:           h,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       60 * time.Second,
			WriteTimeout:      cfg.Server.Timeout + 60*time.Second, // longer than any program run
			IdleTimeout:       120 * time.Second,
			MaxHeaderBytes:    64 << 10,
			ErrorLog:          slog.NewLogLogger(handler, slog.LevelWarn),
		}
	}

	var tln net.Listener
	if tlsConfig != nil {
		if tln, err = net.Listen("tcp", cfg.TLS.Addr); err != nil {
			log.Error("cannot listen", "addr", cfg.TLS.Addr, "error", err)
			return 1
		}
	}
	// Plain HTTP: the site itself, or (with HTTPS) a redirector that still
	// answers health checks and ACME HTTP-01 challenges.
	var plain http.Handler = srv
	if tlsConfig != nil && cfg.TLS.Redirect {
		port := cfg.TLS.RedirectPort()
		if cfg.TLS.PublicPort == "" {
			// The real port, in case -tls-addr asked for any free port.
			port = config.TLS{Addr: tln.Addr().String()}.RedirectPort()
		}
		plain = redirectToHTTPS(srv, port)
	}
	if acmeMgr != nil {
		plain = acmeMgr.HTTPHandler(plain)
	}
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		if tln != nil {
			tln.Close()
		}
		log.Error("cannot listen", "addr", cfg.Addr, "error", err)
		return 1
	}
	plainServer := newServer(plain)
	servers := []*http.Server{plainServer}
	errc := make(chan error, 3)
	go func() { errc <- plainServer.Serve(ln) }()
	var tlsAddr net.Addr
	if tln != nil {
		hs := newServer(srv)
		hs.TLSConfig = tlsConfig // ServeTLS adds HTTP/2
		servers = append(servers, hs)
		tlsAddr = tln.Addr()
		go func() { errc <- hs.ServeTLS(tln, "", "") }()
	}
	if cfg.MetricsAddr != "" {
		mln, err := net.Listen("tcp", cfg.MetricsAddr)
		if err != nil {
			log.Error("cannot listen", "addr", cfg.MetricsAddr, "error", err)
			return 1
		}
		mux := http.NewServeMux()
		mux.Handle("/metrics", srv.MetricsHandler(version))
		ms := newServer(mux)
		servers = append(servers, ms)
		go func() { errc <- ms.Serve(mln) }()
		log.Info("metrics listening", "addr", mln.Addr().String())
	}
	attrs := []any{"version", version, "addr", ln.Addr().String(), "root", cfg.Server.Root,
		"spa", srv.SPA(), "cache", !cfg.Server.NoCache}
	if tlsAddr != nil {
		attrs = append(attrs, "https", tlsAddr.String())
	}
	log.Info("melhttpd listening", attrs...)
	if ready != nil {
		ready(ln.Addr(), tlsAddr)
	}
	select {
	case err := <-errc:
		log.Error("server stopped", "error", err)
		return 1
	case <-ctx.Done():
	}
	log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	code := 0
	for _, hs := range servers {
		if err := hs.Shutdown(sctx); err != nil {
			log.Error("shutdown", "error", err)
			code = 1
		}
	}
	return code
}

// setupTLS builds the TLS configuration for whichever HTTPS mode is enabled.
func setupTLS(c config.TLS, log *slog.Logger) (*tls.Config, *autocert.Manager, error) {
	if !c.Enabled() {
		return nil, nil, nil
	}
	minVersion, err := tlsutil.MinVersion(c.MinVersion)
	if err != nil {
		return nil, nil, err
	}
	switch {
	case c.CertFile != "":
		r, err := tlsutil.NewReloader(c.CertFile, c.KeyFile, 10*time.Second)
		if err != nil {
			return nil, nil, err
		}
		return &tls.Config{GetCertificate: r.GetCertificate, MinVersion: minVersion}, nil, nil
	case c.SelfSigned:
		hosts := []string{"localhost", "127.0.0.1", "::1"}
		if h, err := os.Hostname(); err == nil && h != "" {
			hosts = append(hosts, h)
		}
		cert, _, _, err := tlsutil.SelfSigned(hosts, 30*24*time.Hour)
		if err != nil {
			return nil, nil, err
		}
		log.Warn("serving HTTPS with a self-signed certificate; browsers will warn (development only)")
		return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: minVersion}, nil, nil
	}
	m, err := tlsutil.NewACME(tlsutil.ACMEConfig{
		Domains: c.ACMEDomains, Email: c.ACMEEmail, CacheDir: c.ACMECache, Directory: c.ACMEDir, CARoot: c.ACMECARoot,
	})
	if err != nil {
		return nil, nil, err
	}
	directory := c.ACMEDir
	if directory == "" {
		directory = acme.LetsEncryptURL
	}
	log.Info("automatic certificates via ACME (accepting the CA's terms of service)",
		"domains", strings.Join(c.ACMEDomains, ","), "directory", directory, "cache", c.ACMECache)
	tc := m.TLSConfig()
	tc.MinVersion = minVersion
	return tc, m, nil
}

// redirectToHTTPS sends every request except health checks to HTTPS with a
// permanent, method-preserving redirect.
func redirectToHTTPS(site http.Handler, port string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			site.ServeHTTP(w, r)
			return
		}
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host == "" || strings.ContainsAny(host, "/\\@") {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if strings.Contains(host, ":") {
			host = "[" + host + "]" // IPv6 literal
		}
		if port != "" {
			host += ":" + port
		}
		http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), http.StatusPermanentRedirect)
	})
}

// healthcheck probes a running server, for container health checks (the
// distroless image has no shell or curl).
func healthcheck(ctx context.Context, url string, stderr io.Writer) int {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(stderr, "unhealthy:", err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(stderr, "unhealthy: status", resp.StatusCode)
		return 1
	}
	return 0
}
