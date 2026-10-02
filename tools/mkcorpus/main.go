// Command mkcorpus regenerates testdata/corpus: deterministic sample files of
// the kinds a website serves (text in several encodings, images, wasm, data),
// used to test that every byte survives the trip through Malbolge.
//
//	go run ./tools/mkcorpus
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

func main() {
	dir := filepath.Join("testdata", "corpus")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(err)
	}
	rng := rand.New(rand.NewPCG(2026, 10))
	files := map[string][]byte{
		"empty.txt":      {},
		"one-byte.bin":   {0xA9},
		"page.html":      []byte(page),
		"crlf.txt":       []byte(strings.ReplaceAll(prose, "\n", "\r\n")),
		"bom-utf8.txt":   append([]byte{0xEF, 0xBB, 0xBF}, prose...),
		"utf16le.txt":    utf16le("Malbolge — ünïcødé ✓ 🎉\r\n" + prose),
		"app.min.js":     []byte(minJS),
		"styles.css":     []byte(css),
		"data.json":      []byte(`{"name":"MelHttp","bytes":[0,1,2],"nested":{"ok":true,"emoji":"🎉","quote":"\"q\""}}` + "\n"),
		"app.js.map":     []byte(`{"version":3,"sources":["app.ts"],"names":[],"mappings":"AAAA,SAASA,IAAI;AAClB"}`),
		"logo.svg":       []byte(svg),
		"image.png":      encodePNG(rng),
		"photo.jpg":      encodeJPEG(rng),
		"anim.gif":       encodeGIF(rng),
		"module.wasm":    wasmModule(),
		"random-4k.bin":  randomBytes(rng, 4096),
		"ascii-8k.txt":   asciiOfLen(rng, 8192),
		"ascii-9k.txt":   asciiOfLen(rng, 9000),
		"high-bytes.bin": highBytes(rng, 3000),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			fail(err)
		}
	}
	fmt.Printf("wrote %d files to %s\n", len(files), dir)
}

const prose = `MelHttp serves websites whose every byte is printed by a Malbolge program.
Malbolge was designed in 1998 by Ben Olmstead to be the most difficult language.
Ünïcødé, accents (café, naïve, jalapeño), symbols (∑ ∞ ≠), and emoji 🎉✓.
`

const page = "<!doctype html>\n<html lang=\"en\">\n<head><meta charset=\"utf-8\"><title>Corpus — Malbolge</title>\n" +
	"<link rel=\"stylesheet\" href=\"styles.css\"></head>\n<body>\n<h1>Hello from Malbolge ✓</h1>\n<p>" + prose + "</p>\n" +
	"<script src=\"app.min.js\"></script>\n</body>\n</html>\n"

const minJS = `"use strict";(()=>{const t=document.querySelector("h1");let e=0;const n=()=>{e++,t&&(t.textContent=` +
	"`Clicked ${e}×`" + `)};t?.addEventListener("click",n),console.log("malbolge",[1,2,3].map(o=>o*3).join(","))})();` + "\n"

const css = `:root{--fg:#1d1d1f;--bg:#fbfbfd}@media (prefers-color-scheme:dark){:root{--fg:#f5f5f7;--bg:#111}}` +
	`body{margin:0;font:16px/1.5 system-ui,sans-serif;color:var(--fg);background:var(--bg)}h1::after{content:" ✓"}` + "\n"

const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="12" fill="#2b2d42"/>` +
	`<text x="32" y="42" font-size="28" text-anchor="middle" fill="#edf2f4">M</text></svg>` + "\n"

func utf16le(s string) []byte {
	var b bytes.Buffer
	b.Write([]byte{0xFF, 0xFE})
	for _, u := range utf16.Encode([]rune(s)) {
		binary.Write(&b, binary.LittleEndian, u)
	}
	return b.Bytes()
}

func testImage(rng *rand.Rand) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 48, 32))
	for y := range 32 {
		for x := range 48 {
			img.Set(x, y, color.RGBA{uint8(x * 5), uint8(y * 8), uint8(rng.IntN(256)), 255})
		}
	}
	return img
}

func encodePNG(rng *rand.Rand) []byte {
	var b bytes.Buffer
	png.Encode(&b, testImage(rng))
	return b.Bytes()
}

func encodeJPEG(rng *rand.Rand) []byte {
	var b bytes.Buffer
	jpeg.Encode(&b, testImage(rng), &jpeg.Options{Quality: 80})
	return b.Bytes()
}

func encodeGIF(rng *rand.Rand) []byte {
	g := &gif.GIF{}
	for f := range 3 {
		img := image.NewPaletted(image.Rect(0, 0, 16, 16), palette.Plan9)
		for i := range img.Pix {
			img.Pix[i] = uint8(rng.IntN(256))
		}
		_ = f
		g.Image = append(g.Image, img)
		g.Delay = append(g.Delay, 10)
	}
	var b bytes.Buffer
	gif.EncodeAll(&b, g)
	return b.Bytes()
}

// wasmModule is a minimal valid module exporting add(i32, i32) i32.
func wasmModule() []byte {
	return []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // magic, version
		0x01, 0x07, 0x01, 0x60, 0x02, 0x7f, 0x7f, 0x01, 0x7f, // type: (i32,i32)->i32
		0x03, 0x02, 0x01, 0x00, // function section
		0x07, 0x07, 0x01, 0x03, 0x61, 0x64, 0x64, 0x00, 0x00, // export "add"
		0x0a, 0x09, 0x01, 0x07, 0x00, 0x20, 0x00, 0x20, 0x01, 0x6a, 0x0b, // code: local.get 0, local.get 1, i32.add
	}
}

func randomBytes(rng *rand.Rand, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(rng.IntN(256))
	}
	return b
}

func asciiOfLen(rng *rand.Rand, n int) []byte {
	const alphabet = "abcdefghijklmnopqrstuvwxyz ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.,;:<>/\"'{}()[]\n\t"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[rng.IntN(len(alphabet))]
	}
	return b
}

// highBytes uses only bytes 154..208, which lag-1 mode cannot print.
func highBytes(rng *rand.Rand, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(154 + rng.IntN(55))
	}
	return b
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "mkcorpus:", err)
	os.Exit(1)
}
