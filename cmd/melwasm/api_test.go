package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thimis/MelHttp/internal/obfs"
)

func program(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "programs", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRunSource(t *testing.T) {
	r, err := runSource(program(t, "hello.mb"), nil, 1000)
	if err != nil || string(r.Output) != "Hello, world." || !r.Halted || r.ReadInput || r.Err != "" {
		t.Fatalf("hello: %+v %v", r, err)
	}
	r, _ = runSource(program(t, "cat-terminating.mb"), []byte("echo me"), 10_000_000)
	if string(r.Output) != "echo me" || !r.ReadInput {
		t.Fatalf("cat: %+v", r)
	}
	r, _ = runSource(program(t, "cat.mb"), nil, 500) // never halts
	if r.Halted || !strings.Contains(r.Err, "step limit") || r.Steps != 500 {
		t.Fatalf("step limit: %+v", r)
	}
	if _, err := runSource("not malbolge", nil, 10); err == nil {
		t.Fatal("accepted invalid source")
	}
}

func TestCompileTextRunsBack(t *testing.T) {
	text := []byte("Malbolge in your browser ✓ \x00\xa9")
	programs, count, cells, err := compileText(text, 0)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || cells == 0 {
		t.Fatalf("count %d cells %d", count, cells)
	}
	r, err := runSource(programs, nil, 10_000_000)
	if err != nil || !bytes.Equal(r.Output, text) {
		t.Fatalf("compiled program printed %q, %v", r.Output, err)
	}
	other, _, _, _ := compileText(text, 99)
	if other == programs {
		t.Error("a seed should change the program")
	}
	big, count, _, err := compileText(bytes.Repeat([]byte("many chunks "), 2000), 0)
	if err != nil || count < 2 || !strings.Contains(big, "\n\n") {
		t.Fatalf("multi-program output: count %d err %v", count, err)
	}
}

func TestDecodeContainer(t *testing.T) {
	body := []byte("decoded ✓")
	enc, err := obfs.Encode(body, 4)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeContainer(enc)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("decode: %q %v", got, err)
	}
	if _, err := decodeContainer([]byte("garbage")); err == nil {
		t.Fatal("decoded garbage")
	}
}
