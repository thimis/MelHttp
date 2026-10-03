package main

import (
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
)

type lockedBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *lockedBuf) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

func TestWatchRebuildsAndServes(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "index.html"), []byte("<p>v1</p>"), 0o644)
	out := filepath.Join(t.TempDir(), "site")
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	var stdout, stderr lockedBuf
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, []string{"watch", "-o", out, "-serve", addr, "-interval", "50ms", src}, nil, &stdout, &stderr)
	}()
	fetch := func() string {
		res, err := http.Get("http://" + addr + "/")
		if err != nil {
			return ""
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return string(b)
	}
	waitFor := func(want string) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			if fetch() == want {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("never served %q; got %q\nstdout: %s\nstderr: %s", want, fetch(), stdout.String(), stderr.String())
	}
	waitFor("<p>v1</p>")
	time.Sleep(20 * time.Millisecond)
	os.WriteFile(filepath.Join(src, "index.html"), []byte("<p>v2 — rebuilt</p>"), 0o644)
	waitFor("<p>v2 — rebuilt</p>")
	cancel()
	if code := <-done; code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if strings.Count(stdout.String(), "compiled") < 2 || !strings.Contains(stdout.String(), "serving") {
		t.Errorf("output:\n%s", stdout.String())
	}
}

func TestWatchWaitsForOutputAndUsage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	var stderr lockedBuf
	code := run(ctx, []string{"watch", "--preset", "vite", "-interval", "50ms", "-o", filepath.Join(t.TempDir(), "o"), t.TempDir()}, nil, io.Discard, &stderr)
	if code != exitOK || !strings.Contains(stderr.String(), "waiting for build output") {
		t.Fatalf("code %d: %s", code, stderr.String())
	}
	if code := run(context.Background(), []string{"watch"}, nil, io.Discard, io.Discard); code != exitUsage {
		t.Errorf("no args: %d", code)
	}
}
