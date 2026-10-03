package wasi

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	buildOnce sync.Once
	module    []byte
	buildErr  error
)

// handler builds testdata/wasi/handler for wasip1 once per test run.
func handler(t *testing.T) []byte {
	t.Helper()
	buildOnce.Do(func() {
		out := filepath.Join(os.TempDir(), "melhttp-test-handler.wasi")
		cmd := exec.Command("go", "build", "-o", out, "./testdata/wasi/handler")
		cmd.Dir = filepath.Join("..", "..")
		cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
		if b, err := cmd.CombinedOutput(); err != nil {
			buildErr = errors.New(string(b))
			return
		}
		module, buildErr = os.ReadFile(out)
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return module
}

func run(t *testing.T, r *Runner, query string, timeout time.Duration, maxOut int64) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	in := strings.NewReader("REQUEST_METHOD=POST\nSCRIPT_NAME=/hi.txt\nQUERY_STRING=" + query + "\nHTTPS=on\n\nbody!")
	var out bytes.Buffer
	err := r.Run(ctx, "hi.txt.wasi", "v1", func() ([]byte, error) { return handler(t), nil }, in, &out, maxOut)
	return out.String(), err
}

// newRunner returns a Runner with the test handler already compiled, as
// melhttpd does at warm-up: compiling is slow (very slow under -race on a
// small CI machine) and must not count against the requests' time limits.
func newRunner(t *testing.T, memoryMB int) *Runner {
	t.Helper()
	r, err := New(context.Background(), memoryMB)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close(context.Background()) })
	mod := handler(t)
	if err := r.Precompile(context.Background(), "hi.txt.wasi", "v1", func() ([]byte, error) { return mod, nil }); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRunHandler(t *testing.T) {
	r := newRunner(t, 64)
	out, err := run(t, r, "", 30*time.Second, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	want := "Hello from WASI! method=POST script=/hi.txt https=on body=\"body!\" env=0\n"
	if !strings.HasSuffix(out, want) || !strings.HasPrefix(out, "Content-Type: text/plain") {
		t.Fatalf("output %q", out)
	}
	// Second run reuses the compiled module and is fast.
	start := time.Now()
	if _, err := run(t, r, "", 30*time.Second, 1<<20); err != nil {
		t.Fatal(err)
	}
	t.Logf("warm run: %v", time.Since(start))
}

func TestSandbox(t *testing.T) {
	r := newRunner(t, 32)
	if out, err := run(t, r, "fs", 30*time.Second, 1<<20); err != nil || !strings.Contains(out, "fs: open /etc/passwd") {
		t.Errorf("file system must be unavailable: %q %v", out, err)
	}
	if _, err := run(t, r, "loop", 500*time.Millisecond, 1<<20); !errors.Is(err, ErrTimeout) {
		t.Errorf("loop: %v, want ErrTimeout", err)
	}
	var exit *ExitError
	if _, err := run(t, r, "exit", 30*time.Second, 1<<20); !errors.As(err, &exit) || exit.Code != 3 || !strings.Contains(exit.Stderr, "on purpose") {
		t.Errorf("exit: %v", err)
	}
	start := time.Now()
	if _, err := run(t, r, "oom", 30*time.Second, 1<<20); err == nil {
		t.Error("oom: the memory limit did not stop the module")
	}
	t.Logf("oom stopped after %v", time.Since(start))
	if _, err := run(t, r, "flood", 30*time.Second, 64<<10); !errors.Is(err, ErrOutputLimit) {
		t.Errorf("flood: %v, want ErrOutputLimit", err)
	}
	// Still healthy afterwards.
	if _, err := run(t, r, "", 30*time.Second, 1<<20); err != nil {
		t.Fatal(err)
	}
}

func TestRecompileOnChangeAndBadModule(t *testing.T) {
	r, _ := New(context.Background(), 0)
	defer r.Close(context.Background())
	var out bytes.Buffer
	err := r.Run(context.Background(), "bad", "1", func() ([]byte, error) { return []byte("not wasm"), nil }, nil, &out, 0)
	if err == nil {
		t.Fatal("accepted a bad module")
	}
	err = r.Run(context.Background(), "bad", "2", func() ([]byte, error) { return handler(t), nil },
		strings.NewReader("\n"), &out, 0)
	if err != nil || !strings.Contains(out.String(), "Hello from WASI") {
		t.Fatalf("new version not compiled: %v %q", err, out.String())
	}
}
