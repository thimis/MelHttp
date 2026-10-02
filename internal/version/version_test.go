package version

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestNumberIsSemver(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(Number) {
		t.Fatalf("Number %q is not MAJOR.MINOR.PATCH", Number)
	}
	if Tag != "v"+Number || Dev != Number+"-dev" {
		t.Fatalf("Tag %q, Dev %q", Tag, Dev)
	}
}

// TestDocsMatchNumber: the changelog has an entry for this release and the
// install guide's download commands use it, so a version bump cannot leave
// them behind.
func TestDocsMatchNumber(t *testing.T) {
	root := filepath.Join("..", "..")
	read := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if !strings.Contains(read("CHANGELOG.md"), "## ["+Number+"]") {
		t.Errorf("CHANGELOG.md has no \"## [%s]\" entry", Number)
	}
	install := read(filepath.Join("docs", "install.md"))
	for _, want := range []string{"VERSION=" + Number, "melhttp_" + Number + "_windows_amd64.zip"} {
		if !strings.Contains(install, want) {
			t.Errorf("docs/install.md does not contain %q", want)
		}
	}
	if old := regexp.MustCompile(`VERSION=(\d+\.\d+\.\d+)`).FindAllStringSubmatch(install, -1); len(old) == 0 {
		t.Error("docs/install.md has no VERSION= lines")
	} else {
		for _, m := range old {
			if m[1] != Number {
				t.Errorf("docs/install.md uses VERSION=%s, want %s", m[1], Number)
			}
		}
	}
}
