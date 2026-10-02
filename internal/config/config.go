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
	LogFormat   string // text or json
	Healthcheck bool   // probe a running server and exit
	Version     bool
	Server      server.Config
}

// Parse reads flags from args and environment variables via getenv.
func Parse(args []string, getenv func(string) string, output io.Writer) (Config, error) {
	var c Config
	fs := flag.NewFlagSet("melhttpd", flag.ContinueOnError)
	fs.SetOutput(output)
	var cacheMB int64
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
	fs.Int64Var(&cacheMB, "cache-mb", 512, "response cache size in MiB")
	fs.BoolVar(&c.Server.AllowSensitiveHeaders, "allow-sensitive-headers", false, "pass Cookie and Authorization to programs")
	fs.StringVar(&c.LogFormat, "log-format", "text", "log format: text or json")
	fs.BoolVar(&c.Healthcheck, "healthcheck", false, "check that a server on -addr is healthy, then exit (for Docker)")
	fs.BoolVar(&c.Version, "version", false, "print the version and exit")

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
