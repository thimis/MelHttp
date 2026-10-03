package main

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
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
		if tol, ok := encoded[name]; ok {
			// Go's image encoders may change their output between Go
			// versions, so these compare as pictures, not bytes.
			if err := sameImage(committed, data, tol); err != nil {
				t.Errorf("%s: committed image differs from the generator (run go run ./tools/mkcorpus): %v", name, err)
			}
		} else if !bytes.Equal(committed, data) {
			t.Errorf("%s: committed file differs from the generator (run go run ./tools/mkcorpus)", name)
		}
	}
}

// encoded lists the corpus files made by Go's image encoders, with the largest
// difference allowed per color channel (JPEG is lossy, so its encoder may
// round differently in another Go version; PNG and GIF are lossless).
var encoded = map[string]uint32{"image.png": 0, "anim.gif": 0, "photo.jpg": 8 << 8}

// sameImage reports whether two encoded images decode to the same picture,
// within tol per 16-bit color channel.
func sameImage(a, b []byte, tol uint32) error {
	ia, _, err := image.Decode(bytes.NewReader(a))
	if err != nil {
		return err
	}
	ib, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return err
	}
	if ia.Bounds() != ib.Bounds() {
		return fmt.Errorf("size %v, want %v", ia.Bounds(), ib.Bounds())
	}
	diff := func(x, y uint32) uint32 {
		if x > y {
			return x - y
		}
		return y - x
	}
	r := ia.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			r1, g1, b1, a1 := ia.At(x, y).RGBA()
			r2, g2, b2, a2 := ib.At(x, y).RGBA()
			if diff(r1, r2) > tol || diff(g1, g2) > tol || diff(b1, b2) > tol || diff(a1, a2) > tol {
				return fmt.Errorf("pixel (%d,%d) differs", x, y)
			}
		}
	}
	return nil
}

func TestSameImage(t *testing.T) {
	files := corpusFiles()
	if err := sameImage(files["image.png"], files["image.png"], 0); err != nil {
		t.Fatal(err)
	}
	// A different picture of the same size is caught.
	if err := sameImage(files["image.png"], encodeOther(t, files["image.png"]), 0); err == nil {
		t.Error("a changed pixel was not detected")
	}
}

// encodeOther returns a PNG of the same picture with one pixel changed.
func encodeOther(t *testing.T, data []byte) []byte {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	m := image.NewRGBA(img.Bounds())
	draw.Draw(m, m.Bounds(), img, img.Bounds().Min, draw.Src)
	p := m.Bounds().Min
	c := m.RGBAAt(p.X, p.Y)
	c.R ^= 0xFF
	m.SetRGBA(p.X, p.Y, c)
	var b bytes.Buffer
	if err := png.Encode(&b, m); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
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
