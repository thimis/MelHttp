package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thimis/MelHttp/internal/gen"
	"github.com/thimis/MelHttp/internal/mbfile"
	"github.com/thimis/MelHttp/internal/melcgi"
)

func noenv(string) string { return "" }

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestServeWarmAndShutdown(t *testing.T) {
	root := t.TempDir()
	chunks, err := gen.Compile(append(melcgi.HeaderBlock("text/html; charset=utf-8"), "<h1>melhttpd</h1>"...), gen.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := mbfile.Write(filepath.Join(root, "index.html.mb"), chunks); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	addrc := make(chan net.Addr, 1)
	done := make(chan int, 1)
	var logs syncBuffer
	go func() {
		done <- run(ctx, []string{"-addr", "127.0.0.1:0", "-root", root, "-log-format", "json"}, noenv,
			io.Discard, &logs, func(a, _ net.Addr) { addrc <- a })
	}()
	var addr net.Addr
	select {
	case addr = <-addrc:
	case code := <-done:
		t.Fatalf("exited early with %d: %s", code, logs.String())
	case <-time.After(30 * time.Second):
		t.Fatal("server did not start")
	}
	resp, err := http.Get("http://" + addr.String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "<h1>melhttpd</h1>" || resp.Header.Get("X-Malbolge-Cache") != "hit" {
		t.Fatalf("GET / = %q, cache %q (warm-up should have cached it)", body, resp.Header.Get("X-Malbolge-Cache"))
	}
	if code := run(context.Background(), []string{"-healthcheck", "-addr", addr.String()}, noenv, io.Discard, io.Discard, nil); code != 0 {
		t.Fatalf("healthcheck on a running server = %d", code)
	}
	cancel()
	if code := <-done; code != 0 {
		t.Fatalf("shutdown exit code %d: %s", code, logs.String())
	}
	for _, want := range []string{`"msg":"warm-up"`, `"cached":1`, `"msg":"melhttpd listening"`, `"msg":"request"`, `"msg":"shutting down"`} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("logs missing %s:\n%s", want, logs.String())
		}
	}
}

func TestHealthcheckFailsWithoutServer(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	var errOut bytes.Buffer
	if code := run(context.Background(), []string{"-healthcheck", "-addr", addr}, noenv, io.Discard, &errOut, nil); code != 1 {
		t.Fatalf("healthcheck with no server = %d", code)
	}
}

func TestStartupErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"-version"}, noenv, &out, &errOut, nil); code != 0 || !strings.HasPrefix(out.String(), "melhttpd ") {
		t.Errorf("version: %d %q", code, out.String())
	}
	if code := run(context.Background(), []string{"-bogus"}, noenv, &out, &errOut, nil); code != 2 {
		t.Errorf("bad flag: %d", code)
	}
	missing := filepath.Join(t.TempDir(), "nope")
	if code := run(context.Background(), []string{"-root", missing}, noenv, &out, &errOut, nil); code != 1 {
		t.Errorf("missing root: %d", code)
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "x.txt"), []byte("x"), 0o644)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	if code := run(context.Background(), []string{"-root", root, "-addr", ln.Addr().String(), "-warm=false"}, noenv, &out, &errOut, nil); code != 1 {
		t.Errorf("port in use: %d", code)
	}
}
