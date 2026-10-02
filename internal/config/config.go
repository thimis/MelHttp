// Package config parses melhttpd's command line. Every flag can also be set
// with an environment variable: MELHTTP_ plus the flag name in upper case
// with '-' replaced by '_' (for example -max-steps → MELHTTP_MAX_STEPS).
// Flags take precedence over the environment.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/thimis/MelHttp/internal/server"
)

// Config is the complete melhttpd configuration.
type Config struct {
	Addr        string
	MetricsAddr string // private listener for /metrics (empty = off)
	LogFile     string // append logs to this file instead of stderr
	Service     string // Windows service action: install, uninstall, start, stop
	ServiceName string
	LogFormat   string // text or json
	Healthcheck bool   // probe a running server and exit
	Version     bool
	Server      server.Config
	TLS         TLS
}

// TLS configures HTTPS. HTTPS is enabled by -tls-cert/-tls-key, -tls-self-signed
// or -acme-domains; the plain HTTP listener on -addr then redirects to HTTPS
// (and answers ACME challenges and /healthz).
type TLS struct {
	Addr        string   // HTTPS listen address
	CertFile    string   // PEM certificate (chain)
	KeyFile     string   // PEM private key
	SelfSigned  bool     // generate a throwaway certificate (development)
	ACMEDomains []string // automatic certificates for these names
	ACMEEmail   string
	ACMECache   string
	ACMEDir     string // ACME directory URL (default: Let's Encrypt)
	ACMECARoot  string // extra CA roots (PEM) to trust for the ACME directory
	Redirect    bool   // redirect plain HTTP to HTTPS
	PublicPort  string // HTTPS port used in redirects (default: the -tls-addr port)
	MinVersion  string // "1.2" or "1.3"
}

// Enabled reports whether HTTPS is configured.
func (t TLS) Enabled() bool {
	return t.CertFile != "" || t.KeyFile != "" || t.SelfSigned || len(t.ACMEDomains) > 0
}

// flagSet defines melhttpd's flags, storing their values in c.
func flagSet(c *Config, cacheMB *int64) *flag.FlagSet {
	fs := flag.NewFlagSet("melhttpd", flag.ContinueOnError)
	fs.StringVar(&c.Addr, "addr", ":8080", "listen `address`")
	fs.StringVar(&c.Server.Root, "root", "site", "site `directory` (built with melc build)")
	fs.BoolVar(&c.Server.SPA, "spa", false, "serve /index.html for unknown extension-less paths (single-page apps)")
	fs.BoolVar(&c.Server.ExposeSource, "expose-source", false, "serve Malbolge source under /_source/")
	fs.BoolVar(&c.Server.NoCache, "no-cache", false, "run the Malbolge VM on every request")
	fs.BoolVar(&c.Server.Warm, "warm", true, "run every program once at startup")
	fs.Int64Var(&c.Server.MaxSteps, "max-steps", 2_000_000_000, "VM instructions per request")
	fs.Int64Var(&c.Server.MaxOutput, "max-output", 256<<20, "bytes a program may print per request")
	fs.DurationVar(&c.Server.Timeout, "timeout", 30*time.Second, "time limit per program run")
	fs.Int64Var(&c.Server.MaxBody, "max-body", 1<<20, "request body bytes passed to programs")
	fs.IntVar(&c.Server.MaxConcurrent, "concurrency", 0, "simultaneous VM runs (0 = 2×CPUs)")
	fs.Int64Var(cacheMB, "cache-mb", 512, "response cache size in MiB")
	fs.BoolVar(&c.Server.AllowSensitiveHeaders, "allow-sensitive-headers", false, "pass Cookie and Authorization to programs")
	fs.StringVar(&c.LogFormat, "log-format", "text", "log format: text or json")
	fs.StringVar(&c.MetricsAddr, "metrics-addr", "", "serve Prometheus metrics at /metrics on this private `address` (e.g. 127.0.0.1:9090)")
	fs.StringVar(&c.LogFile, "log-file", "", "append logs to this `file` instead of stderr (services have no console)")
	fs.StringVar(&c.Service, "service", "", "Windows: `install`, uninstall, start or stop the melhttpd service (other flags are stored for the service)")
	fs.StringVar(&c.ServiceName, "service-name", "melhttpd", "Windows service `name`")
	fs.BoolVar(&c.Healthcheck, "healthcheck", false, "check that a server on -addr is healthy, then exit (for Docker)")
	fs.BoolVar(&c.Version, "version", false, "print the version and exit")
	fs.StringVar(&c.TLS.Addr, "tls-addr", ":8443", "HTTPS listen `address` (when HTTPS is enabled)")
	fs.StringVar(&c.TLS.CertFile, "tls-cert", "", "TLS certificate `file` (PEM; reloaded when it changes)")
	fs.StringVar(&c.TLS.KeyFile, "tls-key", "", "TLS private key `file` (PEM)")
	fs.BoolVar(&c.TLS.SelfSigned, "tls-self-signed", false, "serve HTTPS with a throwaway self-signed certificate (development only)")
	fs.Func("acme-domains", "comma-separated `names` to get Let's Encrypt certificates for", func(v string) error {
		c.TLS.ACMEDomains = nil
		for _, d := range strings.Split(v, ",") {
			if d = strings.TrimSpace(d); d != "" {
				c.TLS.ACMEDomains = append(c.TLS.ACMEDomains, d)
			}
		}
		return nil
	})
	fs.StringVar(&c.TLS.ACMEEmail, "acme-email", "", "contact `email` for the certificate authority")
	fs.StringVar(&c.TLS.ACMECache, "acme-cache", "autocert-cache", "`directory` to keep ACME certificates and account keys in")
	fs.StringVar(&c.TLS.ACMEDir, "acme-directory", "", "ACME directory `URL` (default: Let's Encrypt production)")
	fs.StringVar(&c.TLS.ACMECARoot, "acme-ca-root", "", "PEM `file` of extra roots to trust for the ACME directory (private CAs, testing)")
	fs.BoolVar(&c.TLS.Redirect, "https-redirect", true, "redirect plain HTTP requests to HTTPS when HTTPS is enabled")
	fs.StringVar(&c.TLS.PublicPort, "https-port", "", "public HTTPS `port` for redirects (default: the -tls-addr port; 443 is omitted)")
	fs.StringVar(&c.TLS.MinVersion, "tls-min", "1.2", "minimum TLS `version`: 1.2 or 1.3")
	fs.DurationVar(&c.Server.HSTS, "hsts", 0, "send Strict-Transport-Security with this max-age on HTTPS (e.g. 8760h; 0 = off)")
	fs.BoolVar(&c.Server.Obfuscate, "obfuscate", false, "Malbolge transport: send bodies as Malbolge programs to the /_melhttp/ service worker (obfuscation, not encryption)")
	fs.BoolVar(&c.Server.ObfuscateInject, "obfuscate-inject", false, "add the transport script to every HTML page, so any site uses the Malbolge transport unchanged (implies -obfuscate)")
	fs.IntVar(&c.Server.ObfuscateVariants, "obfuscate-variants", 2, "differently-seeded encodings kept per page")
	fs.BoolVar(&c.Server.Playground, "playground", false, "serve the in-browser Malbolge playground at /_melhttp/")
	fs.BoolVar(&c.Server.WASI, "wasi", false, "run WebAssembly MelCGI handlers (*.wasi files), sandboxed: no files, environment or network")
	fs.IntVar(&c.Server.WASIMemoryMB, "wasi-memory-mb", 64, "memory limit per WASI handler run, in MiB")
	return fs
}

// IsBoolFlag reports whether name is one of melhttpd's boolean flags
// (which take no separate value argument).
func IsBoolFlag(name string) bool {
	f := flagSet(&Config{}, new(int64)).Lookup(name)
	if f == nil {
		return false
	}
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// Parse reads flags from args and environment variables via getenv.
func Parse(args []string, getenv func(string) string, output io.Writer) (Config, error) {
	var c Config
	var cacheMB int64
	fs := flagSet(&c, &cacheMB)
	fs.SetOutput(output)
	set := map[string]bool{}
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if fs.NArg() > 0 {
		return c, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	var errs []error
	fs.VisitAll(func(f *flag.Flag) {
		if set[f.Name] {
			return
		}
		name := "MELHTTP_" + strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		if v := getenv(name); v != "" {
			if err := f.Value.Set(v); err != nil {
				errs = append(errs, fmt.Errorf("%s=%q: %w", name, v, err))
			}
		}
	})
	if err := errors.Join(errs...); err != nil {
		return c, err
	}
	if c.LogFormat != "text" && c.LogFormat != "json" {
		return c, fmt.Errorf("-log-format must be text or json, not %q", c.LogFormat)
	}
	c.Server.CacheBytes = cacheMB << 20
	if err := c.TLS.validate(); err != nil {
		return c, err
	}
	return c, nil
}

// HealthcheckURL is the URL -healthcheck probes: the listen address with an
// unspecified host replaced by the loopback address.
func (c Config) HealthcheckURL() string {
	host, port, err := net.SplitHostPort(c.Addr)
	if err != nil {
		host, port = "", c.Addr
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/healthz"
}

func (t TLS) validate() error {
	sources := 0
	if t.CertFile != "" || t.KeyFile != "" {
		if t.CertFile == "" || t.KeyFile == "" {
			return errors.New("-tls-cert and -tls-key must be given together")
		}
		sources++
	}
	if t.SelfSigned {
		sources++
	}
	if len(t.ACMEDomains) > 0 {
		sources++
	}
	if sources > 1 {
		return errors.New("choose one of -tls-cert/-tls-key, -tls-self-signed or -acme-domains")
	}
	if t.MinVersion != "1.2" && t.MinVersion != "1.3" {
		return fmt.Errorf("-tls-min must be 1.2 or 1.3, not %q", t.MinVersion)
	}
	return nil
}

// RedirectPort is the HTTPS port to put in redirect URLs ("" for 443).
func (t TLS) RedirectPort() string {
	port := t.PublicPort
	if port == "" {
		if _, p, err := net.SplitHostPort(t.Addr); err == nil {
			port = p
		}
	}
	if port == "443" {
		return ""
	}
	return port
}
