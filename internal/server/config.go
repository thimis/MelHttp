// Package server is the MelHttp HTTP server: it maps URLs onto a site
// directory of Malbolge programs (and, optionally, static files), runs the
// programs in the sandboxed VM through MelCGI, and serves the results with
// caching, compression and conditional requests.
package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"regexp"
	"runtime"
	"time"
)

// Config configures a Server. Zero values get sensible defaults.
type Config struct {
	Root          string        // site directory
	SPA           bool          // serve /index.html for unknown extension-less paths
	ExposeSource  bool          // serve program source under /_source/
	NoCache       bool          // run the VM on every request
	Warm          bool          // run deterministic programs at startup
	MaxSteps      int64         // per request (default 2e9)
	MaxOutput     int64         // bytes per request (default 256 MiB)
	Timeout       time.Duration // per program run (default 30s)
	MaxBody       int64         // request body bytes passed to programs (default 1 MiB)
	MaxConcurrent int           // simultaneous VM runs (default 2×GOMAXPROCS)
	CacheBytes    int64         // response cache size (default 512 MiB)
	// Revalidate is how often a cached entry re-checks its files on disk
	// (default 1s; negative means on every request).
	Revalidate            time.Duration
	AllowSensitiveHeaders bool          // pass Cookie/Authorization to programs
	HSTS                  time.Duration // Strict-Transport-Security max-age on HTTPS responses (0 = off)
	// Obfuscate enables the Malbolge transport: requests carrying
	// "X-Malbolge-Accept: program" get the body as Malbolge programs, and
	// /_melhttp/ serves the service worker that decodes them in browsers.
	Obfuscate bool
	// ObfuscateInject adds the transport's opt-in script to every HTML
	// response, so any site uses the transport unchanged. Implies Obfuscate.
	ObfuscateInject   bool
	ObfuscateVariants int  // differently-seeded encodings kept per page (default 2)
	Playground        bool // serve the in-browser Malbolge playground at /_melhttp/
	// WASI enables WebAssembly MelCGI handlers (*.wasi files), sandboxed by
	// wazero: no file system, environment or network; WASIMemoryMB per run.
	WASI         bool
	WASIMemoryMB int // default 64
	Logger       *slog.Logger
}

func (c Config) withDefaults() Config {
	if c.MaxSteps <= 0 {
		c.MaxSteps = 2_000_000_000
	}
	if c.MaxOutput <= 0 {
		c.MaxOutput = 256 << 20
	}
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	if c.MaxBody <= 0 {
		c.MaxBody = 1 << 20
	}
	if c.MaxConcurrent <= 0 {
		c.MaxConcurrent = 2 * runtime.GOMAXPROCS(0)
	}
	if c.CacheBytes <= 0 {
		c.CacheBytes = 512 << 20
	}
	if c.ObfuscateInject {
		c.Obfuscate = true
	}
	if c.ObfuscateVariants <= 0 {
		c.ObfuscateVariants = 2
	}
	if c.Revalidate == 0 {
		c.Revalidate = time.Second
	}
	if c.Logger == nil {
		c.Logger = slog.New(slog.DiscardHandler)
	}
	return c
}

// SiteConfigFile is the optional per-site configuration file at the site root.
// melc build writes it; it is never served.
const SiteConfigFile = "melhttp.json"

// SiteConfig is the content of melhttp.json.
type SiteConfig struct {
	SPA bool `json:"spa"`
	// Immutable lists regular expressions matched against the URL path;
	// matching responses get a one-year immutable Cache-Control.
	Immutable []string `json:"immutable,omitempty"`
	// Headers are added to every response.
	Headers map[string]string `json:"headers,omitempty"`
}

// DefaultImmutable matches the content-hashed file names that bundlers
// (Angular, Vite, webpack, …) emit, e.g. main-B3YI7OQ3.js or index-Cx2_ab9f.css.
const DefaultImmutable = `[.-][A-Za-z0-9_-]{8,}\.(js|mjs|css|woff2?|ttf|otf|png|jpe?g|gif|webp|avif|svg|wasm|ico)$`

type siteConfig struct {
	SiteConfig
	immutable []*regexp.Regexp
}

func loadSiteConfig(fsys fs.FS) (siteConfig, error) {
	var sc siteConfig
	data, err := fs.ReadFile(fsys, SiteConfigFile)
	if err == nil {
		if err := json.Unmarshal(data, &sc.SiteConfig); err != nil {
			return sc, fmt.Errorf("%s: %w", SiteConfigFile, err)
		}
	}
	patterns := sc.Immutable
	if len(patterns) == 0 {
		patterns = []string{DefaultImmutable}
	}
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return sc, fmt.Errorf("%s: immutable pattern %q: %w", SiteConfigFile, p, err)
		}
		sc.immutable = append(sc.immutable, re)
	}
	for k := range sc.Headers {
		if !validHeaderName(k) {
			return sc, fmt.Errorf("%s: invalid header name %q", SiteConfigFile, k)
		}
	}
	return sc, nil
}

func validHeaderName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}
