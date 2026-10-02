package gen

import (
	"bytes"
	"context"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thimis/MelHttp/internal/malbolge"
)

// TestLag1CannotPrintAllBytes documents why the generator uses fill mode: with
// d == c-1, [d] is always an encrypted printable char and only 355 values of a
// (201 byte values) are ever reachable, from any state.
func TestLag1CannotPrintAllBytes(t *testing.T) {
	seen := map[lag1]bool{}
	var stack []lag1
	for prev := range uint8(nMoves) {
		for p := range 94 {
			stack = append(stack, lag1{a: malbolge.Rotr(lag1Mem[prev][p]), prev: mvRotr, pos: p + 1})
		}
	}
	stack = append(stack, lag1Start())
	as := map[uint16]bool{}
	for len(stack) > 0 {
		s := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		s.pos %= 94
		if seen[s] {
			continue
		}
		seen[s] = true
		as[s.a] = true
		for mv := range uint8(nMoves) {
			stack = append(stack, s.step(mv))
		}
	}
	var bytesSeen [256]bool
	for a := range as {
		bytesSeen[a%256] = true
	}
	n := 0
	for _, ok := range bytesSeen {
		if ok {
			n++
		}
	}
	if len(as) != 355 || n != 201 || bytesSeen[0xA9] {
		t.Fatalf("lag-1 mode reaches %d values of a and %d byte values (0xA9: %v); want 355, 201, false",
			len(as), n, bytesSeen[0xA9])
	}
}

func roundTrip(t *testing.T, data []byte, opt Options) [][]byte {
	t.Helper()
	chunks, err := Compile(data, opt)
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(context.Background(), chunks, data); err != nil {
		t.Fatal(err)
	}
	return chunks
}

func cells(chunks [][]byte) int {
	n := 0
	for _, c := range chunks {
		n += len(bytes.Join(bytes.Fields(c), nil))
	}
	return n
}

func TestRoundTrip(t *testing.T) {
	all := make([]byte, 0, 768)
	for i := range 256 {
		all = append(all, byte(i))
	}
	for i := 255; i >= 0; i-- {
		all = append(all, byte(i), byte(i))
	}
	rng := rand.New(rand.NewPCG(7, 7))
	random := make([]byte, 50_000)
	for i := range random {
		random[i] = byte(rng.IntN(256))
	}
	html := []byte(strings.Repeat("<li class=\"item\">Ünïcødé ✓ — Malbolge says hi! 🎉</li>\r\n", 300))
	for name, data := range map[string][]byte{
		"empty": {}, "A": []byte("A"), "nul": {0}, "all256": all, "random50k": random, "utf8html": html,
		"0xA9": bytes.Repeat([]byte{0xA9}, 100),
	} {
		t.Run(name, func(t *testing.T) {
			chunks := roundTrip(t, data, Options{})
			if len(data) > 0 {
				t.Logf("%d bytes → %d chunks, %d cells (%.2f cells/byte)", len(data), len(chunks), cells(chunks),
					float64(cells(chunks))/float64(len(data)))
			}
		})
	}
}

func TestEmptyInputIsOneSmallProgram(t *testing.T) {
	chunks := roundTrip(t, nil, Options{})
	if len(chunks) != 1 || cells(chunks) != prefixLen+1 {
		t.Fatalf("empty input: %d chunks, %d cells; want 1 chunk of %d", len(chunks), cells(chunks), prefixLen+1)
	}
}

func TestChunking(t *testing.T) {
	data := append(bytes.Repeat([]byte("chunk me please, Malbolge! "), 2000), bytes.Repeat([]byte("é∑"), 3000)...)
	for _, opt := range []Options{{}, {MaxCells: MinCells}, {SegmentSize: 1000}, {SegmentSize: 1}} {
		chunks := roundTrip(t, data, opt)
		for i, c := range chunks {
			n := len(bytes.Join(bytes.Fields(c), nil))
			if n > malbolge.MemSize || (opt.MaxCells > 0 && n > opt.MaxCells) {
				t.Fatalf("opt %+v: chunk %d has %d cells", opt, i, n)
			}
		}
	}
}

func TestDeterministicAndSeeded(t *testing.T) {
	data := []byte("same input, same program — unless seeded")
	a := roundTrip(t, data, Options{})
	b := roundTrip(t, data, Options{Workers: 1})
	if !bytes.Equal(bytes.Join(a, nil), bytes.Join(b, nil)) {
		t.Fatal("output depends on worker count")
	}
	s1 := roundTrip(t, data, Options{Seed: 1})
	s2 := roundTrip(t, data, Options{Seed: 2})
	s1b := roundTrip(t, data, Options{Seed: 1})
	if bytes.Equal(s1[0], s2[0]) || bytes.Equal(s1[0], a[0]) {
		t.Fatal("seeds should change the program")
	}
	if !bytes.Equal(s1[0], s1b[0]) {
		t.Fatal("the same seed should give the same program")
	}
}

func TestProgramSingle(t *testing.T) {
	src, err := Program([]byte("Hello, Malbolge!"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(context.Background(), [][]byte{src}, []byte("Hello, Malbolge!")); err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewPCG(3, 3))
	big := make([]byte, 20_000)
	for i := range big {
		big[i] = byte(rng.IntN(256))
	}
	if _, err := Program(big, Options{}); err != ErrTooLarge {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestLineWidth(t *testing.T) {
	src, _ := Program([]byte("wrap"), Options{LineWidth: 50})
	for _, line := range strings.Split(strings.TrimSpace(string(src)), "\n") {
		if len(line) > 50 {
			t.Fatalf("line of %d chars", len(line))
		}
	}
	src, _ = Program([]byte("wrap"), Options{LineWidth: -1})
	if strings.Count(string(src), "\n") != 1 {
		t.Fatal("LineWidth -1 should not wrap")
	}
}

func TestVerifyDetectsCorruption(t *testing.T) {
	chunks := roundTrip(t, []byte("abc"), Options{})
	if err := Verify(context.Background(), chunks, []byte("abd")); err == nil {
		t.Fatal("Verify accepted wrong output")
	}
	if err := Verify(context.Background(), [][]byte{[]byte("not malbolge")}, nil); err == nil {
		t.Fatal("Verify accepted an invalid program")
	}
}

// corpusSize is how many files tools/mkcorpus generates into testdata/corpus.
const corpusSize = 19

func TestCorpus(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "corpus", "*"))
	if len(files) != corpusSize {
		t.Fatalf("testdata/corpus has %d files, want %d (regenerate: go run ./tools/mkcorpus)", len(files), corpusSize)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(filepath.Base(f), func(t *testing.T) {
			chunks := roundTrip(t, data, Options{})
			if len(data) > 0 {
				t.Logf("%d bytes → %d chunks (%.2f cells/byte)", len(data), len(chunks), float64(cells(chunks))/float64(len(data)))
			}
		})
	}
}

func FuzzRoundTrip(f *testing.F) {
	f.Add([]byte("Hello"), uint64(0), 0)
	f.Add([]byte{0, 0xff, 0xa9, 0x9a}, uint64(3), 1)
	f.Fuzz(func(t *testing.T, data []byte, seed uint64, seg int) {
		if len(data) > 4096 {
			data = data[:4096]
		}
		opt := Options{Seed: seed, SegmentSize: seg%64 + 1 + len(data)%3}
		roundTrip(t, data, opt)
	})
}

func BenchmarkCompileText(b *testing.B) {
	data := []byte(strings.Repeat("<p class=\"x\">The quick brown fox jumps over the lazy dog.</p>\n", 1600)) // ~100 KB
	benchCompile(b, data)
}

func BenchmarkCompileBinary(b *testing.B) {
	rng := rand.New(rand.NewPCG(1, 1))
	data := make([]byte, 100_000)
	for i := range data {
		data[i] = byte(rng.IntN(256))
	}
	benchCompile(b, data)
}

func benchCompile(b *testing.B, data []byte) {
	getTape()
	getLag1()
	b.SetBytes(int64(len(data)))
	var chunks [][]byte
	for b.Loop() {
		chunks, _ = Compile(data, Options{})
	}
	b.ReportMetric(float64(cells(chunks))/float64(len(data)), "cells/byte")
}
