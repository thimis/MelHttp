package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thimis/MelHttp/internal/malbolge"
	"github.com/thimis/MelHttp/internal/mbfile"
	"github.com/thimis/MelHttp/internal/melcgi"
)

func runSet(t *testing.T, path string) []byte {
	t.Helper()
	set, err := mbfile.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := set.RunBytes(context.Background(), nil, malbolge.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGenRawRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "x.bin")
	data := []byte("raw bytes \x00\xff\xa9 ✓")
	os.WriteFile(src, data, 0o644)
	if _, errOut, code := melc(t, "", "gen", "-raw", src); code != exitOK {
		t.Fatalf("code %d: %s", code, errOut)
	}
	if got := runSet(t, src+".mb"); !bytes.Equal(got, data) {
		t.Fatalf("got %q", got)
	}
}

func TestGenAddsHeaderBlock(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "index.html")
	os.WriteFile(src, []byte("<h1>hi</h1>"), 0o644)
	out := filepath.Join(dir, "site", "index.html.mb")
	os.MkdirAll(filepath.Dir(out), 0o755)
	if _, errOut, code := melc(t, "", "gen", "-o", out, src); code != exitOK {
		t.Fatalf("code %d: %s", code, errOut)
	}
	resp, err := melcgi.ParseResponse(runSet(t, out))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != 200 || resp.Header.Get("Content-Type") != "text/html; charset=utf-8" || string(resp.Body) != "<h1>hi</h1>" {
		t.Fatalf("response %+v body %q", resp, resp.Body)
	}
	// -type overrides the extension.
	melc(t, "", "gen", "-type", "application/x-custom", "-o", out, src)
	resp, _ = melcgi.ParseResponse(runSet(t, out))
	if resp.Header.Get("Content-Type") != "application/x-custom" {
		t.Fatalf("type override: %v", resp.Header)
	}
}

func TestGenChunkDirForLargeInput(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "big.html")
	data := bytes.Repeat([]byte("<p>A page large enough to need several Malbolge programs.</p>\n"), 1500) // ~95 KB
	os.WriteFile(src, data, 0o644)
	_, errOut, code := melc(t, "", "gen", "-stats", src)
	if code != exitOK || !strings.Contains(errOut, "program(s)") {
		t.Fatalf("code %d: %s", code, errOut)
	}
	if fi, err := os.Stat(src + ".mb"); err != nil || !fi.IsDir() {
		t.Fatalf("expected a chunk directory: %v", err)
	}
	resp, err := melcgi.ParseResponse(runSet(t, src+".mb"))
	if err != nil || !bytes.Equal(resp.Body, data) {
		t.Fatalf("chunked page did not round-trip: %v", err)
	}
	// Stdout only works for a single program.
	if _, _, code := melc(t, "", "gen", "-o", "-", src); code != exitError {
		t.Fatalf("stdout with chunks: code %d", code)
	}
}

func TestGenStdoutAndSeed(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	os.WriteFile(src, []byte("seeded"), 0o644)
	a, _, _ := melc(t, "", "gen", "-raw", "-o", "-", src)
	b, _, _ := melc(t, "", "gen", "-raw", "-seed", "7", "-o", "-", src)
	if a == b || a == "" {
		t.Fatal("seed should change the program")
	}
	for _, prog := range []string{a, b} {
		p, err := malbolge.Load([]byte(prog))
		if err != nil {
			t.Fatal(err)
		}
		out, _, _ := p.RunBytes(context.Background(), nil, malbolge.Limits{})
		if string(out) != "seeded" {
			t.Fatalf("printed %q", out)
		}
	}
}

func TestGenErrors(t *testing.T) {
	if _, _, code := melc(t, "", "gen"); code != exitUsage {
		t.Errorf("no file: code %d", code)
	}
	if _, _, code := melc(t, "", "gen", "missing.txt"); code != exitError {
		t.Errorf("missing file: code %d", code)
	}
}

func TestBuildCommand(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "index.html"), []byte("<h1>built</h1>"), 0o644)
	out := filepath.Join(t.TempDir(), "site")
	stdout, errOut, code := melc(t, "", "build", "-o", out, "--spa=true", src)
	if code != exitOK || !strings.Contains(stdout, "1 files") {
		t.Fatalf("code %d out %q err %q", code, stdout, errOut)
	}
	resp, err := melcgi.ParseResponse(runSet(t, filepath.Join(out, "index.html.mb")))
	if err != nil || string(resp.Body) != "<h1>built</h1>" {
		t.Fatalf("built page: %v", err)
	}
	cfg, _ := os.ReadFile(filepath.Join(out, "melhttp.json"))
	if !strings.Contains(string(cfg), `"spa": true`) {
		t.Errorf("melhttp.json = %s", cfg)
	}
	if _, _, code := melc(t, "", "build", "--preset", "cobol", src); code != exitUsage {
		t.Errorf("unknown preset: code %d", code)
	}
	if _, _, code := melc(t, "", "build", "--preset", "vite", "-o", out, src); code != exitError {
		t.Errorf("vite preset without dist: code %d", code)
	}
	if _, _, code := melc(t, "", "build", "--spa=maybe", "-o", out, src); code != exitUsage {
		t.Errorf("bad --spa: code %d", code)
	}
	if _, _, code := melc(t, "", "build"); code != exitUsage {
		t.Errorf("no args: code %d", code)
	}
}
