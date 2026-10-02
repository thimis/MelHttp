//go:build acceptance

package acceptance

import (
	"os"
	"os/exec"
	"testing"
)

// TestSuiteCatchesInjectedFaults proves the unit tests are sharp: with each
// deliberate VM bug compiled in (-tags faults), they must fail.
func TestSuiteCatchesInjectedFaults(t *testing.T) {
	for _, fault := range []string{"crz-swap", "no-encrypt", "eof-zero", "fill-swap"} {
		t.Run(fault, func(t *testing.T) {
			cmd := exec.Command("go", "test", "-count=1", "-tags", "faults", "./internal/malbolge", "./internal/gen", "./cmd/melc")
			cmd.Dir = repoRoot
			cmd.Env = append(os.Environ(), "MELHTTP_FAULT="+fault)
			if out, err := cmd.CombinedOutput(); err == nil {
				t.Fatalf("tests passed with fault %q injected — they would not catch this bug:\n%s", fault, out)
			}
		})
	}
	// Sanity: with the tag but no fault, everything passes.
	cmd := exec.Command("go", "test", "-count=1", "-tags", "faults", "./internal/malbolge", "./internal/gen", "./cmd/melc")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "MELHTTP_FAULT=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tests fail without an injected fault:\n%s", out)
	}
}
