package hygiene

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryPackageHasTests fails when any package in the module has no test
// files, so "[no test files]" can never hide untested code.
func TestEveryPackageHasTests(t *testing.T) {
	cmd := exec.Command("go", "list", "-f",
		`{{if and (eq (len .TestGoFiles) 0) (eq (len .XTestGoFiles) 0)}}{{.ImportPath}}{{end}}`, "./...")
	cmd.Dir = filepath.Join("..", "..")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		t.Errorf("package %s has no tests", pkg)
	}
}
