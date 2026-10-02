// Command gen builds the browser assets that are generated rather than
// committed: melhttp.wasm (cmd/melwasm for js/wasm) and the matching
// wasm_exec.js from the Go toolchain. Run it with:
//
//	go generate ./internal/webvm
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	// go generate runs in the package directory, internal/webvm.
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join("assets", "melhttp.wasm"), "../../cmd/melwasm")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		fail(err)
	}
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		fail(err)
	}
	root := strings.TrimSpace(string(out))
	var js []byte
	for _, p := range []string{filepath.Join(root, "lib", "wasm", "wasm_exec.js"), filepath.Join(root, "misc", "wasm", "wasm_exec.js")} {
		if js, err = os.ReadFile(p); err == nil {
			break
		}
	}
	if err != nil {
		fail(fmt.Errorf("wasm_exec.js not found in %s", root))
	}
	if err := os.WriteFile(filepath.Join("assets", "wasm_exec.js"), js, 0o644); err != nil {
		fail(err)
	}
	fi, _ := os.Stat(filepath.Join("assets", "melhttp.wasm"))
	fmt.Printf("webvm: built assets/melhttp.wasm (%d bytes) and assets/wasm_exec.js\n", fi.Size())
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "webvm gen:", err)
	os.Exit(1)
}
