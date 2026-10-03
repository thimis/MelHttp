package server

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	wasiOnce   sync.Once
	wasiModule []byte
	wasiErr    error
)

func wasiHandler(t *testing.T) []byte {
	t.Helper()
	wasiOnce.Do(func() {
		out := filepath.Join(t.TempDir(), "handler.wasi")
		cmd := exec.Command("go", "build", "-o", out, "./testdata/wasi/handler")
		cmd.Dir = filepath.Join("..", "..")
		cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
		if b, err := cmd.CombinedOutput(); err != nil {
			wasiErr = err
			t.Log(string(b))
			return
		}
		wasiModule, wasiErr = os.ReadFile(out)
	})
	if wasiErr != nil {
		t.Fatal(wasiErr)
	}
	return wasiModule
}

func TestWASIHandlers(t *testing.T) {
	root, _ := site(t)
	writeFile(t, root, "app/hello.txt.wasi", wasiHandler(t))
	writeFile(t, root, "raw.txt.raw.wasi", wasiHandler(t))
	writeFile(t, root, "notwasm.txt.wasi", []byte("nope"))
	// A generous time limit: under -race on a small CI machine, the oom case
	// can take seconds to reach its memory limit, and must fail as 500, not
	// as a timeout. The timeout itself is checked below with a short limit.
	s, ts := newTestServer(t, root, Config{WASI: true, Timeout: 30 * time.Second, MaxOutput: 1 << 20})
	if st := s.Warm(t.Context()); st.Failed != 4 { // 3 broken Malbolge fixtures + notwasm.txt.wasi
		t.Fatalf("warm-up: %+v", st)
	}

	r := do(t, http.MethodPost, ts.URL+"/app/hello.txt", strings.NewReader("hi"))
	if r.status != 200 || !strings.Contains(string(r.body), `Hello from WASI! method=POST script=/app/hello.txt https= body="hi" env=0`) ||
		r.header.Get("X-Powered-By") != "WebAssembly (MelHttp WASI)" || r.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("WASI handler: %d %q %v", r.status, r.body, r.header)
	}
	before := s.Runs()
	get(t, ts.URL+"/app/hello.txt")
	get(t, ts.URL+"/app/hello.txt")
	if s.Runs()-before != 2 {
		t.Error("WASI handlers must run on every request")
	}
	raw := get(t, ts.URL+"/raw.txt")
	if raw.status != 200 || !strings.HasPrefix(string(raw.body), "Content-Type: text/plain") {
		t.Errorf("raw WASI: %d %q", raw.status, raw.body)
	}
	for q, want := range map[string]int{"exit": 500, "oom": 500, "flood": 500} {
		if r := get(t, ts.URL+"/app/hello.txt?"+q); r.status != want {
			t.Errorf("?%s = %d, want %d", q, r.status, want)
		}
	}
	// A handler that never finishes hits the time limit: 503.
	fast, tsFast := newTestServer(t, root, Config{WASI: true, Timeout: 500 * time.Millisecond, MaxOutput: 1 << 20})
	fast.Warm(t.Context()) // compile first, so the limit times the handler, not the compiler
	if r := get(t, tsFast.URL+"/app/hello.txt?loop"); r.status != 503 {
		t.Errorf("?loop = %d, want 503", r.status)
	}
	if r := get(t, ts.URL+"/notwasm.txt"); r.status != 500 {
		t.Errorf("non-wasm module: %d", r.status)
	}
	for _, p := range []string{"/app/hello.txt.wasi", "/_source/app/hello.txt"} {
		if r := get(t, ts.URL+p); r.status != 404 {
			t.Errorf("%s = %d: module bytes must never be served", p, r.status)
		}
	}
}

func TestWASIDisabledByDefault(t *testing.T) {
	root, _ := site(t)
	writeFile(t, root, "hello.txt.wasi", wasiHandler(t))
	_, ts := newTestServer(t, root, Config{})
	if r := get(t, ts.URL+"/hello.txt"); r.status != 404 {
		t.Fatalf("WASI handler ran while disabled: %d", r.status)
	}
}
