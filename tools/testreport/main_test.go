package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `{"Action":"start","Package":"github.com/thimis/MelHttp/internal/a"}
{"Action":"run","Package":"github.com/thimis/MelHttp/internal/a","Test":"TestOK"}
{"Action":"output","Package":"github.com/thimis/MelHttp/internal/a","Test":"TestOK","Output":"=== RUN   TestOK\n"}
{"Action":"pass","Package":"github.com/thimis/MelHttp/internal/a","Test":"TestOK","Elapsed":0.01}
{"Action":"run","Package":"github.com/thimis/MelHttp/internal/a","Test":"TestNeedsDocker"}
{"Action":"output","Package":"github.com/thimis/MelHttp/internal/a","Test":"TestNeedsDocker","Output":"    a_test.go:12: docker is not running\n"}
{"Action":"output","Package":"github.com/thimis/MelHttp/internal/a","Test":"TestNeedsDocker","Output":"--- SKIP: TestNeedsDocker (0.00s)\n"}
{"Action":"skip","Package":"github.com/thimis/MelHttp/internal/a","Test":"TestNeedsDocker"}
{"Action":"output","Package":"github.com/thimis/MelHttp/internal/a","Output":"ok  \tgithub.com/thimis/MelHttp/internal/a\t0.02s\n"}
{"Action":"pass","Package":"github.com/thimis/MelHttp/internal/a","Elapsed":0.02}
`

const failing = `{"Action":"run","Package":"github.com/thimis/MelHttp/internal/b","Test":"TestBroken"}
{"Action":"output","Package":"github.com/thimis/MelHttp/internal/b","Test":"TestBroken","Output":"    b_test.go:9: got 1, want 2\n"}
{"Action":"fail","Package":"github.com/thimis/MelHttp/internal/b","Test":"TestBroken","Elapsed":0}
{"Action":"run","Package":"github.com/thimis/MelHttp/internal/b","Test":"TestBroken/sub"}
{"Action":"pass","Package":"github.com/thimis/MelHttp/internal/b","Test":"TestBroken/sub"}
{"Action":"output","Package":"github.com/thimis/MelHttp/internal/b","Output":"FAIL\tgithub.com/thimis/MelHttp/internal/b\t0.01s\n"}
{"Action":"fail","Package":"github.com/thimis/MelHttp/internal/b","Elapsed":0.01}
`

func runReport(t *testing.T, input string, each bool) (int, string) {
	t.Helper()
	var out bytes.Buffer
	code := run(strings.NewReader(input), &out, each)
	return code, out.String()
}

func TestPassingRunListsSkips(t *testing.T) {
	code, out := runReport(t, sample, false)
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	for _, want := range []string{
		"ok  \tgithub.com/thimis/MelHttp/internal/a\t0.02s",
		"Tests: 1 passed, 0 failed, 1 skipped in 1 packages",
		"Skipped (not run, so not verified):",
		"internal/a TestNeedsDocker: docker is not running",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestFailureShowsOutputAndExits1(t *testing.T) {
	code, out := runReport(t, failing, false)
	if code != 1 {
		t.Fatalf("code %d", code)
	}
	for _, want := range []string{"Tests: 1 passed, 1 failed, 0 skipped", "internal/b TestBroken", "b_test.go:9: got 1, want 2", "Package failed:"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// TestByteOrderMark: Windows PowerShell 5.1 can prefix piped text with a
// byte-order mark; the first event must still be parsed.
func TestByteOrderMark(t *testing.T) {
	code, out := runReport(t, "\xef\xbb\xbf"+sample, false)
	if code != 0 || strings.Contains(out, `"Action"`) || !strings.Contains(out, "1 passed") {
		t.Fatalf("code %d\n%s", code, out)
	}
}

func TestEachPrintsTopLevelTests(t *testing.T) {
	_, out := runReport(t, sample+failing, true)
	if !strings.Contains(out, "PASS TestOK") || !strings.Contains(out, "SKIP TestNeedsDocker") ||
		!strings.Contains(out, "FAIL TestBroken") || strings.Contains(out, "TestBroken/sub (") {
		t.Fatalf("each:\n%s", out)
	}
}

func TestBuildErrorsAndEmptyInput(t *testing.T) {
	code, out := runReport(t, "# github.com/x\n./a.go:1: syntax error\n"+
		`{"Action":"build-fail","Package":"github.com/x"}`+"\n", false)
	if code != 1 || !strings.Contains(out, "syntax error") || !strings.Contains(out, "build failed") {
		t.Fatalf("build failure: %d\n%s", code, out)
	}
	// Go 1.24+ names the package in ImportPath on build events.
	code, out = runReport(t, `{"ImportPath":"github.com/thimis/MelHttp/x.test","Action":"build-fail"}`+"\n", false)
	if code != 1 || !strings.Contains(out, "Package failed: github.com/thimis/MelHttp/x (build failed)") {
		t.Fatalf("ImportPath build failure: %d\n%s", code, out)
	}
	if code, out := runReport(t, "", false); code != 1 || !strings.Contains(out, "No test results") {
		t.Fatalf("empty input: %d\n%s", code, out)
	}
}

// TestRealGoTestOutput runs go test -json on a tiny throwaway module, so the
// parser is checked against the real format, not just hand-written samples.
func TestRealGoTestOutput(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.24\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "demo_test.go"), []byte(`package demo

import "testing"

func TestPass(t *testing.T) {}
func TestSkip(t *testing.T) { t.Skip("tool missing") }
func TestFail(t *testing.T) { t.Error("boom") }
`), 0o644)
	cmd := exec.Command("go", "test", "-json", "./...")
	cmd.Dir = dir
	raw, _ := cmd.Output() // exits 1 because TestFail fails
	code, out := runReport(t, string(raw), false)
	if code != 1 {
		t.Fatalf("code %d", code)
	}
	for _, want := range []string{"Tests: 1 passed, 1 failed, 1 skipped", "demo TestSkip: tool missing", "demo TestFail", "boom"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
