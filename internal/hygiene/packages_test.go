package hygiene

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thimis/MelHttp/internal/testutil"
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

// TestNoGoSourceIsIgnored fails when git ignores a Go source file. Such a file
// builds and tests fine locally but never reaches the repository: a "dist/"
// rule once hid tools/dist this way.
func TestNoGoSourceIsIgnored(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		testutil.Skip(t, "git is not installed")
	}
	cmd := exec.Command("git", "ls-files", "--others", "--ignored", "--exclude-standard", "--",
		"*.go", "go.mod", "go.sum", ":!:**/node_modules/**")
	cmd.Dir = filepath.Join("..", "..")
	out, err := cmd.Output()
	if err != nil {
		testutil.Skip(t, "not a git checkout: "+err.Error())
	}
	for _, f := range strings.Fields(string(out)) {
		t.Errorf("%s is ignored by .gitignore, so it is never committed", f)
	}
}
