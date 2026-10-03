package config

import (
	"io"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDefaults(t *testing.T) {
	c, err := Parse(nil, env(nil), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8080" || c.Server.Root != "site" || !c.Server.Warm || c.Server.SPA || c.Server.NoCache ||
		c.LogFormat != "text" || c.Server.Timeout != 30*time.Second {
		t.Fatalf("defaults: %+v", c)
	}
}

func TestFlagsOverrideEnv(t *testing.T) {
	c, err := Parse([]string{"-addr", "127.0.0.1:9000", "-spa", "-max-steps", "123"},
		env(map[string]string{"MELHTTP_ADDR": ":1", "MELHTTP_ROOT": "/srv/site", "MELHTTP_SPA": "false",
			"MELHTTP_NO_CACHE": "true", "MELHTTP_TIMEOUT": "5s", "MELHTTP_CACHE_MB": "64", "MELHTTP_WARM": "0"}),
		io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != "127.0.0.1:9000" || c.Server.Root != "/srv/site" || !c.Server.SPA || !c.Server.NoCache ||
		c.Server.MaxSteps != 123 || c.Server.Timeout != 5*time.Second || c.Server.CacheBytes != 64<<20 || c.Server.Warm {
		t.Fatalf("config: %+v", c)
	}
}

func TestErrors(t *testing.T) {
	for _, tt := range []struct {
		args []string
		env  map[string]string
	}{
		{[]string{"-nope"}, nil},
		{nil, map[string]string{"MELHTTP_TIMEOUT": "soon"}},
		{nil, map[string]string{"MELHTTP_SPA": "maybe"}},
		{[]string{"-log-format", "xml"}, nil},
		{[]string{"extra-arg"}, nil},
	} {
		if _, err := Parse(tt.args, env(tt.env), io.Discard); err == nil {
			t.Errorf("Parse(%v, %v) accepted bad input", tt.args, tt.env)
		}
	}
}

func TestHealthcheckURL(t *testing.T) {
	for addr, want := range map[string]string{
		":8080":          "http://127.0.0.1:8080/healthz",
		"0.0.0.0:80":     "http://127.0.0.1:80/healthz",
		"[::]:8080":      "http://127.0.0.1:8080/healthz",
		"10.0.0.5:9000":  "http://10.0.0.5:9000/healthz",
		"localhost:1234": "http://localhost:1234/healthz",
	} {
		c := Config{Addr: addr}
		if got := c.HealthcheckURL(); got != want {
			t.Errorf("HealthcheckURL(%q) = %q, want %q", addr, got, want)
		}
	}
}

func TestTLSFlags(t *testing.T) {
	c, err := Parse([]string{"-acme-domains", "a.test, b.test", "-hsts", "8760h", "-https-port", "443"},
		env(map[string]string{"MELHTTP_ACME_EMAIL": "me@example.com", "MELHTTP_TLS_ADDR": ":9443"}), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !c.TLS.Enabled() || len(c.TLS.ACMEDomains) != 2 || c.TLS.ACMEDomains[1] != "b.test" || c.TLS.ACMEEmail != "me@example.com" ||
		c.TLS.Addr != ":9443" || c.Server.HSTS != 8760*time.Hour || !c.TLS.Redirect || c.TLS.ACMECache != "autocert-cache" {
		t.Fatalf("tls config: %+v hsts %v", c.TLS, c.Server.HSTS)
	}
	if c.TLS.RedirectPort() != "" {
		t.Errorf("443 should be omitted from redirects, got %q", c.TLS.RedirectPort())
	}
	plain, _ := Parse(nil, env(nil), io.Discard)
	if plain.TLS.Enabled() || plain.TLS.RedirectPort() != "8443" {
		t.Errorf("default TLS: enabled=%v port=%q", plain.TLS.Enabled(), plain.TLS.RedirectPort())
	}
	for _, args := range [][]string{
		{"-tls-cert", "c.pem"},
		{"-tls-cert", "c.pem", "-tls-key", "k.pem", "-tls-self-signed"},
		{"-tls-self-signed", "-acme-domains", "a.test"},
		{"-tls-self-signed", "-tls-min", "1.1"},
	} {
		if _, err := Parse(args, env(nil), io.Discard); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}

func TestIsBoolFlag(t *testing.T) {
	for _, name := range []string{"spa", "obfuscate", "obfuscate-inject", "playground", "wasi", "warm", "tls-self-signed"} {
		if !IsBoolFlag(name) {
			t.Errorf("%s should be a boolean flag", name)
		}
	}
	for _, name := range []string{"root", "addr", "service", "obfuscate-variants", "acme-domains", "no-such-flag"} {
		if IsBoolFlag(name) {
			t.Errorf("%s should not be a boolean flag", name)
		}
	}
}

func TestObfuscateInject(t *testing.T) {
	c, err := Parse([]string{"-obfuscate-inject"}, env(nil), io.Discard)
	if err != nil || !c.Server.ObfuscateInject {
		t.Fatalf("flag: %v %+v", err, c.Server)
	}
	c, err = Parse(nil, env(map[string]string{"MELHTTP_OBFUSCATE_INJECT": "true"}), io.Discard)
	if err != nil || !c.Server.ObfuscateInject {
		t.Fatalf("env: %v %+v", err, c.Server)
	}
}
