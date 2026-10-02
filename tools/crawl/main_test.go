package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrawlMode(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "index.html"), []byte("<p>hi</p>"), 0o644)
	os.WriteFile(filepath.Join(src, "page.mb"), []byte("program source: skipped"), 0o644)
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/index.html" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<p>hi</p>"))
	}))
	defer good.Close()
	var out bytes.Buffer
	if code := run([]string{"-base", good.URL, "-src", src}, &out, &out); code != 0 || !strings.Contains(out.String(), "checked 1 files") {
		t.Fatalf("good server: code %d\n%s", code, out.String())
	}
	bad := httptest.NewServer(http.NotFoundHandler())
	defer bad.Close()
	out.Reset()
	if code := run([]string{"-base", bad.URL, "-src", src}, &out, &out); code != 1 || !strings.Contains(out.String(), "status 404") {
		t.Fatalf("bad server: code %d\n%s", code, out.String())
	}
	if code := run([]string{"-src", filepath.Join(src, "missing")}, &out, &out); code != 2 {
		t.Fatalf("missing source: code %d", code)
	}
}

func TestLoadMode(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("x")) }))
	defer ok.Close()
	var out bytes.Buffer
	if code := run([]string{"-load", ok.URL, "-n", "200", "-c", "4", "-gzip"}, &out, &out); code != 0 ||
		!strings.Contains(out.String(), "200 requests, 4 workers") || !strings.Contains(out.String(), "failed 0") {
		t.Fatalf("load: code %d\n%s", code, out.String())
	}
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", 500) }))
	defer failing.Close()
	out.Reset()
	if code := run([]string{"-load", failing.URL, "-n", "20", "-c", "2"}, &out, &out); code != 1 || !strings.Contains(out.String(), "failed 20") {
		t.Fatalf("failing server: code %d\n%s", code, out.String())
	}
}

func TestUsage(t *testing.T) {
	var out bytes.Buffer
	if code := run(nil, &out, &out); code != 2 {
		t.Errorf("no mode: code %d", code)
	}
	if code := run([]string{"-bogus"}, &out, &out); code != 2 {
		t.Errorf("bad flag: code %d", code)
	}
	if code := run([]string{"-load", "http://127.0.0.1:1", "-n", "0"}, &out, &out); code != 2 {
		t.Errorf("-n 0: code %d", code)
	}
}
