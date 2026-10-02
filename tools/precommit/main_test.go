package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thimis/MelHttp/internal/testutil"
)

// repo creates a throwaway git repository.
func repo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		testutil.Skip(t, "git not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "test@example.com"}, {"config", "user.name", "test"},
		{"config", "core.autocrlf", "false"},
	} {
		if _, err := gitOutput(dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func stage(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := gitOutput(dir, "add", "-f", name); err != nil {
		t.Fatal(err)
	}
}

func check(dir string, args ...string) (int, string) {
	var out, errb bytes.Buffer
	code := run(args, dir, &out, &errb)
	return code, out.String() + errb.String()
}

func TestStagedFiles(t *testing.T) {
	dir := repo(t)
	stage(t, dir, "main.go", "package main\n")
	stage(t, dir, "docs/readme.md", "# hello\n")
	if code, out := check(dir); code != 0 {
		t.Fatalf("clean repo refused: %d %s", code, out)
	}
	// Built by concatenation so this file never contains a real key header.
	stage(t, dir, "notes.txt", "-----BEGIN "+"PRIVATE KEY-----\nabc\n")
	if code, out := check(dir); code != 1 || !strings.Contains(out, "notes.txt") {
		t.Fatalf("private key not caught: %d %s", code, out)
	}
	gitOutput(dir, "rm", "--cached", "-q", "notes.txt")
	stage(t, dir, ".env", "TOKEN=1\n")
	stage(t, dir, ".claude/settings.json", "{}")
	code, out := check(dir)
	if code != 1 || !strings.Contains(out, ".env") || !strings.Contains(out, ".claude/settings.json") {
		t.Fatalf("forbidden files not caught: %d %s", code, out)
	}
}

func TestAllTrackedFiles(t *testing.T) {
	dir := repo(t)
	stage(t, dir, "ok.go", "package ok\n")
	gitOutput(dir, "commit", "-q", "-m", "init")
	if code, out := check(dir, "-all"); code != 0 {
		t.Fatalf("clean: %d %s", code, out)
	}
	stage(t, dir, "server.key", "x")
	gitOutput(dir, "commit", "-q", "-m", "oops")
	if code, out := check(dir); code != 0 {
		t.Fatalf("nothing staged should pass: %d %s", code, out)
	}
	if code, out := check(dir, "-all"); code != 1 || !strings.Contains(out, "server.key") {
		t.Fatalf("-all missed a committed key: %d %s", code, out)
	}
}

func TestInstallHook(t *testing.T) {
	dir := repo(t)
	code, out := check(dir, "-install")
	if code != 0 {
		t.Fatalf("install: %d %s", code, out)
	}
	b, err := os.ReadFile(filepath.Join(dir, ".git", "hooks", "pre-commit"))
	if err != nil || string(b) != hook {
		t.Fatalf("hook: %q %v", b, err)
	}
}

func TestErrors(t *testing.T) {
	if code, _ := check(t.TempDir()); code != 2 {
		t.Errorf("not a repository: code %d", code)
	}
	if code, _ := check(t.TempDir(), "-bogus"); code != 2 {
		t.Errorf("bad flag: code %d", code)
	}
}
