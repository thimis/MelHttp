// Package reftest runs Malbolge programs through Ben Olmstead's reference
// interpreter (testdata/reference, built in Docker) so tests can compare our
// VM and generator against an independent oracle.
package reftest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const image = "melhttp-malbolge-ref:latest"

// Case is one program to run.
type Case struct {
	Src      []byte
	Input    []byte // nil means no input (EOF)
	MaxSteps int64  // 0 means the harness default (1,000,000)
}

// Outcome is what the reference interpreter did.
type Outcome struct {
	Output []byte
	Status string // HALT, INVALID, LIMIT, or ERROR
	Steps  int64
	Detail string // raw stderr for ERROR
}

// Dir returns the absolute path of testdata/reference.
func Dir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "reference")
}

var buildOnce sync.Once
var buildErr error

func build() error {
	buildOnce.Do(func() {
		cmd := exec.Command("docker", "build", "-q", "-t", image, Dir())
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("docker build reference harness: %v\n%s", err, out)
		}
	})
	return buildErr
}

// Run executes all cases in a single container and returns their outcomes in order.
func Run(t testing.TB, cases []Case) []Outcome {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatal("docker is required for reference tests")
	}
	if err := build(); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	for i, c := range cases {
		base := filepath.Join(work, fmt.Sprintf("p%06d", i))
		must(t, os.WriteFile(base+".mb", c.Src, 0o644))
		if c.Input != nil {
			must(t, os.WriteFile(base+".in", c.Input, 0o644))
		}
		if c.MaxSteps > 0 {
			must(t, os.WriteFile(base+".steps", []byte(strconv.FormatInt(c.MaxSteps, 10)), 0o644))
		}
	}
	cmd := exec.Command("docker", "run", "--rm", "--network=none", "-v", work+":/work", image)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("docker run reference harness: %v\n%s", err, out)
	}
	outs := make([]Outcome, len(cases))
	for i := range cases {
		base := filepath.Join(work, fmt.Sprintf("p%06d", i))
		out, err := os.ReadFile(base + ".refout")
		must(t, err)
		errText, err := os.ReadFile(base + ".referr")
		must(t, err)
		outs[i] = parse(out, string(errText))
	}
	return outs
}

func parse(out []byte, stderr string) Outcome {
	o := Outcome{Output: out, Status: "ERROR", Detail: stderr}
	fields := strings.Fields(stderr)
	if len(fields) == 2 {
		if n, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
			switch fields[0] {
			case "HALT", "INVALID", "LIMIT":
				o.Status, o.Steps, o.Detail = fields[0], n, ""
			}
		}
	}
	return o
}

func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
