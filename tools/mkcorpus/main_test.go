package main

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestCorpusMatchesCommittedFiles: the generator is deterministic and the
// committed testdata/corpus is exactly what it produces (no silent drift).
func TestCorpusMatchesCommittedFiles(t *testing.T) {
	a, b := corpusFiles(), corpusFiles()
	dir := filepath.Join("..", "..", "testdata", "corpus")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(a) {
		t.Errorf("testdata/corpus has %d files, the generator makes %d", len(entries), len(a))
	}
	for name, data := range a {
		if !bytes.Equal(data, b[name]) {
			t.Errorf("%s differs between two runs", name)
		}
		committed, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !bytes.Equal(committed, data) {
			t.Errorf("%s: committed file differs from the generator (run go run ./tools/mkcorpus)", name)
		}
	}
}

// TestCorpusFilesAreValid: the binary samples are real files of their type.
func TestCorpusFilesAreValid(t *testing.T) {
	files := corpusFiles()
	for _, name := range []string{"image.png", "photo.jpg", "anim.gif"} {
		if _, format, err := image.Decode(bytes.NewReader(files[name])); err != nil {
			t.Errorf("%s does not decode: %v", name, err)
		} else {
			t.Logf("%s: %s", name, format)
		}
	}
	if !bytes.HasPrefix(files["module.wasm"], []byte("\x00asm\x01\x00\x00\x00")) {
		t.Error("module.wasm lacks the WebAssembly header")
	}
	if !bytes.HasPrefix(files["utf16le.txt"], []byte{0xFF, 0xFE}) || !bytes.HasPrefix(files["bom-utf8.txt"], []byte{0xEF, 0xBB, 0xBF}) {
		t.Error("byte-order marks missing")
	}
	for _, c := range files["high-bytes.bin"] {
		if c < 154 || c > 208 {
			t.Fatalf("high-bytes.bin contains %d, outside 154..208", c)
		}
	}
	if len(files["empty.txt"]) != 0 || len(files["one-byte.bin"]) != 1 {
		t.Error("size fixtures wrong")
	}
}
