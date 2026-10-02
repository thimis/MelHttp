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
