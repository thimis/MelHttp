//go:build acceptance

// Package acceptance holds the goal-ladder tests (G1–G11). They drive the real
// melc and melhttpd binaries as a user would, so they compile from day one and
// turn green phase by phase. Run with:
//
//	go test -tags acceptance ./acceptance -v
package acceptance

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var (
	repoRoot  string
	binDir    string
	melcBin   string
	serverBin string
	buildErr  error
)

func TestMain(m *testing.M) {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	repoRoot = filepath.Dir(wd)
	binDir, err = os.MkdirTemp("", "melhttp-acceptance-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	melcBin = filepath.Join(binDir, "melc"+exe)
	serverBin = filepath.Join(binDir, "melhttpd"+exe)
	buildErr = goBuild(melcBin, "./cmd/melc")
	if err := goBuild(serverBin, "./cmd/melhttpd"); err != nil && buildErr == nil {
		buildErr = err
	}
	code := m.Run()
	os.RemoveAll(binDir)
	os.Exit(code)
}

func goBuild(out, pkg string) error {
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build %s: %v\n%s", pkg, err, b)
	}
	return nil
}

// requireBinaries fails the test if the binaries could not be built.
func requireBinaries(t *testing.T) {
	t.Helper()
	if buildErr != nil {
		t.Fatalf("binaries not built yet: %v", buildErr)
	}
}

// melc runs the melc binary and returns stdout; it fails the test on a non-zero exit.
func melc(t *testing.T, stdin []byte, args ...string) []byte {
	t.Helper()
	out, errOut, err := melcErr(stdin, args...)
	if err != nil {
		t.Fatalf("melc %s: %v\nstderr: %s", strings.Join(args, " "), err, errOut)
	}
	return out
}

func melcErr(stdin []byte, args ...string) (stdout, stderr []byte, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, melcBin, args...)
	cmd.Dir = repoRoot
	cmd.Stdin = bytes.NewReader(stdin)
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err = cmd.Run()
	return o.Bytes(), e.Bytes(), err
}

// startServer launches melhttpd on a free loopback port and returns its base URL.
func startServer(t *testing.T, root string, flags ...string) string {
	t.Helper()
	requireBinaries(t)
	addr := freeAddr(t)
	args := append([]string{"-addr", addr, "-root", root}, flags...)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, serverBin, args...)
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start melhttpd: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		cmd.Wait()
		if t.Failed() {
			t.Logf("melhttpd logs:\n%s", logs.String())
		}
	})
	base := "http://" + addr
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return base
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("melhttpd did not become healthy on %s\nlogs:\n%s", addr, logs.String())
	return ""
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

type response struct {
	Status int
	Header http.Header
	Body   []byte
}

func do(t *testing.T, method, url string, hdr map[string]string) response {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	// Disable transparent gzip so tests see exactly what the server sent.
	tr := &http.Transport{DisableCompression: true}
	resp, err := (&http.Client{Transport: tr, Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{resp.StatusCode, resp.Header, body}
}

func get(t *testing.T, url string) response { return do(t, http.MethodGet, url, nil) }

func have(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// writeFile writes data to root/rel, creating directories.
func writeFile(t *testing.T, root, rel string, data []byte) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
