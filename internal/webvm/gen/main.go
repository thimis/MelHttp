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
	if err := generate(filepath.Join("..", ".."), "assets"); err != nil {
		fmt.Fprintln(os.Stderr, "webvm gen:", err)
		os.Exit(1)
	}
}

// generate builds melhttp.wasm and copies wasm_exec.js into outDir. root is
// the module root.
func generate(root, outDir string) error {
	wasm, err := filepath.Abs(filepath.Join(outDir, "melhttp.wasm"))
	if err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", wasm, "./cmd/melwasm")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return err
	}
	goroot := strings.TrimSpace(string(out))
	var js []byte
	for _, p := range []string{filepath.Join(goroot, "lib", "wasm", "wasm_exec.js"), filepath.Join(goroot, "misc", "wasm", "wasm_exec.js")} {
		if js, err = os.ReadFile(p); err == nil {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("wasm_exec.js not found in %s", goroot)
	}
	if err := os.WriteFile(filepath.Join(outDir, "wasm_exec.js"), js, 0o644); err != nil {
		return err
	}
	fi, _ := os.Stat(wasm)
	fmt.Printf("webvm: built %s (%d bytes) and wasm_exec.js\n", wasm, fi.Size())
	return nil
}
