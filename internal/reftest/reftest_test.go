package reftest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for stderr, want := range map[string]Outcome{
		"HALT 47\n":                          {Status: "HALT", Steps: 47},
		"INVALID 2\n":                        {Status: "INVALID", Steps: 2},
		"LIMIT 100000":                       {Status: "LIMIT", Steps: 100000},
		"invalid character in source file\n": {Status: "ERROR", Detail: "invalid character in source file\n"},
		"HALT many":                          {Status: "ERROR", Detail: "HALT many"},
		"BOOM 3":                             {Status: "ERROR", Detail: "BOOM 3"},
		"":                                   {Status: "ERROR"},
	} {
		got := parse([]byte("out"), stderr)
		if got.Status != want.Status || got.Steps != want.Steps || got.Detail != want.Detail || string(got.Output) != "out" {
			t.Errorf("parse(%q) = %+v, want %+v", stderr, got, want)
		}
	}
}

// TestHarnessFiles checks the files the Docker image is built from are
// where Dir says, and that the harness keeps the reference's exec loop.
func TestHarnessFiles(t *testing.T) {
	for _, f := range []string{"Dockerfile", "run.sh", "harness.c", "malbolge.c", "harness.patch"} {
		if _, err := os.Stat(filepath.Join(Dir(), f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	harness, _ := os.ReadFile(filepath.Join(Dir(), "harness.c"))
	for _, want := range []string{"MAL_STEPS", "HALT %lld", "INVALID %lld", "LIMIT %lld", "xlat2[mem[c] - 33]"} {
		if !strings.Contains(string(harness), want) {
			t.Errorf("harness.c lacks %q", want)
		}
	}
}
