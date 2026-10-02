package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestServiceArgs(t *testing.T) {
	abs := func(p string) string { a, _ := filepath.Abs(p); return a }
	got := serviceArgs([]string{
		"-service", "install", "-service-name=web", "-root", "site", "-addr", ":80",
		"-spa", "-log-file=logs/m.log", "--tls-cert", "c.pem", "-tls-key=k.pem", "-wasi",
	})
	want := []string{
		"-root=" + abs("site"), "-addr", ":80", "-spa", "-log-file=" + abs("logs/m.log"),
		"-tls-cert=" + abs("c.pem"), "-tls-key=" + abs("k.pem"), "-wasi",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("serviceArgs:\n got %q\nwant %q", got, want)
	}
}

func TestLogFile(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "melhttpd.log")
	var stderr bytes.Buffer
	code := run(context.Background(), []string{"-root", filepath.Join(t.TempDir(), "missing"), "-log-file", logFile}, noenv, io.Discard, &stderr, nil)
	if code != 1 {
		t.Fatalf("code %d", code)
	}
	b, _ := os.ReadFile(logFile)
	if !strings.Contains(string(b), "cannot open site") || stderr.Len() != 0 {
		t.Fatalf("log file %q, stderr %q", b, stderr.String())
	}
}

func TestServiceControlErrors(t *testing.T) {
	var stderr bytes.Buffer
	code := run(context.Background(), []string{"-service", "frobnicate"}, noenv, io.Discard, &stderr, nil)
	if code != 1 || stderr.Len() == 0 {
		t.Fatalf("bad action: code %d %q", code, stderr.String())
	}
	if runtime.GOOS != "windows" && !strings.Contains(stderr.String(), "systemd") {
		t.Errorf("non-Windows message should point to systemd/launchd: %q", stderr.String())
	}
	if isWindowsService() {
		t.Error("a test process is not a Windows service")
	}
}
