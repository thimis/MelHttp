package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func melc(t *testing.T, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	code = run(context.Background(), args, strings.NewReader(stdin), &o, &e)
	return o.String(), e.String(), code
}

func program(name string) string { return filepath.Join("..", "..", "testdata", "programs", name) }

func TestRunHello(t *testing.T) {
	out, errOut, code := melc(t, "", "run", "-stats", program("hello.mb"))
	if code != exitOK || out != "Hello, world." {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
	if !strings.Contains(errOut, "halted=true") || !strings.Contains(errOut, "read_input=false") {
		t.Fatalf("stats: %q", errOut)
	}
}

func TestRunCatStepLimit(t *testing.T) {
	out, _, code := melc(t, "abc", "run", "-steps", "100000", program("cat.mb"))
	if code != exitSteps || !strings.HasPrefix(out, "abc\xa8") {
		t.Fatalf("code %d out %q", code, out[:min(10, len(out))])
	}
}

func TestRunTerminatingCat(t *testing.T) {
	in := "Malbolge cat, terminating!\nline 2\n"
	out, errOut, code := melc(t, in, "run", "-steps", "50000000", program("cat-terminating.mb"))
	if code != exitOK || out != in {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
}

func TestRunQuine(t *testing.T) {
	src, err := os.ReadFile(program("quine.mb"))
	if err != nil {
		t.Fatal(err)
	}
	out, errOut, code := melc(t, "", "run", "-steps", "500000000", program("quine.mb"))
	if code != exitOK || out != string(src) {
		t.Fatalf("code %d, quine output %d bytes vs source %d bytes; err %q", code, len(out), len(src), errOut)
	}
}

func TestRunAdder(t *testing.T) {
	out, errOut, code := melc(t, "123 456", "run", "-steps", "500000000", program("adder.mb"))
	if code != exitOK || strings.TrimSpace(out) != "579" {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
}

func TestRunDigitalRoot(t *testing.T) {
	out, errOut, code := melc(t, "987654321", "run", "-steps", "500000000", program("digital_root.mb"))
	if code != exitOK || strings.TrimSpace(out) != "9" {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
}

func TestRunUsageAndErrors(t *testing.T) {
	if _, _, code := melc(t, "", "run"); code != exitUsage {
		t.Errorf("no args: code %d", code)
	}
	if _, _, code := melc(t, "", "run", "missing.mb"); code != exitError {
		t.Errorf("missing file: code %d", code)
	}
	if _, _, code := melc(t, ""); code != exitUsage {
		t.Errorf("no command: code %d", code)
	}
	if _, _, code := melc(t, "", "frobnicate"); code != exitUsage {
		t.Errorf("unknown command: code %d", code)
	}
	if out, _, code := melc(t, "", "version"); code != exitOK || out != "melc "+version+"\n" {
		t.Errorf("version: %d %q", code, out)
	}
}

func TestCheck(t *testing.T) {
	out, _, code := melc(t, "", "check", program("hello.mb"), program("quine.mb"))
	if code != exitOK || strings.Count(out, ": ok") != 2 {
		t.Fatalf("code %d out %q", code, out)
	}
	bad := filepath.Join(t.TempDir(), "bad.mb")
	os.WriteFile(bad, []byte("hello there"), 0o644)
	_, errOut, code := melc(t, "", "check", bad)
	if code != exitError || !strings.Contains(errOut, "invalid") {
		t.Fatalf("code %d err %q", code, errOut)
	}
}
