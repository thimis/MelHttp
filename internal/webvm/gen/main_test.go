package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerate(t *testing.T) {
	out := t.TempDir()
	if err := generate(filepath.Join("..", "..", ".."), out); err != nil {
		t.Fatal(err)
	}
	wasm, err := os.ReadFile(filepath.Join(out, "melhttp.wasm"))
	if err != nil || !bytes.HasPrefix(wasm, []byte("\x00asm")) || len(wasm) < 1<<20 {
		t.Fatalf("melhttp.wasm: %d bytes, err %v", len(wasm), err)
	}
	js, err := os.ReadFile(filepath.Join(out, "wasm_exec.js"))
	if err != nil || !bytes.Contains(js, []byte("globalThis.Go = class")) {
		t.Fatalf("wasm_exec.js: %v", err)
	}
}

func TestGenerateErrors(t *testing.T) {
	if err := generate(t.TempDir(), t.TempDir()); err == nil {
		t.Error("generate succeeded outside the module")
	}
}
