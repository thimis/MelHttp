package obfs

import (
	"bytes"
	"context"
	"math/rand/v2"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	random := make([]byte, 30_000)
	for i := range random {
		random[i] = byte(rng.IntN(256))
	}
	for name, body := range map[string][]byte{
		"empty": {}, "html": []byte("<h1>obfuscated ✓</h1>"), "random": random,
		"large-ascii": bytes.Repeat([]byte("console.log('malbolge transport');\n"), 3000),
	} {
		for _, seed := range []uint64{0, 7} {
			enc, err := Encode(body, seed)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.ContainsAny(enc, "\x00\r") || !isASCII(enc) {
				t.Fatalf("%s: container is not plain ASCII text", name)
			}
			dec, err := Decode(context.Background(), enc, 0)
			if err != nil {
				t.Fatalf("%s seed %d: %v", name, seed, err)
			}
			if !bytes.Equal(dec, body) {
				t.Fatalf("%s seed %d: decoded %d bytes, want %d", name, seed, len(dec), len(body))
			}
		}
	}
}

func isASCII(b []byte) bool {
	for _, c := range b {
		if c != '\n' && (c < 33 || c > 126) {
			return false
		}
	}
	return true
}

func TestSeedsDiffer(t *testing.T) {
	a, _ := Encode([]byte("same page"), 1)
	b, _ := Encode([]byte("same page"), 2)
	if bytes.Equal(a, b) {
		t.Fatal("different seeds gave identical programs")
	}
}

func TestMultiProgramAndCRLF(t *testing.T) {
	body := bytes.Repeat([]byte{0xA9, 'x'}, 5000) // several tape chunks
	enc, _ := Encode(body, 3)
	if n := len(split(enc)); n < 2 {
		t.Fatalf("expected several programs, got %d", n)
	}
	crlf := bytes.ReplaceAll(enc, []byte("\n"), []byte("\r\n"))
	dec, err := Decode(context.Background(), crlf, 0)
	if err != nil || !bytes.Equal(dec, body) {
		t.Fatalf("CRLF container: %v", err)
	}
}

func TestDecodeRejects(t *testing.T) {
	cat := "(=BA#9\"=<;:3y7x54-21q/p-,+*)\"!h%B0/.\n~P<\n<:(8&\n66#\"!~}|{zyxwvu\ngJ%\n" // never halts
	for name, c := range map[string]string{
		"empty":        "",
		"not malbolge": "hello world",
		"loops":        cat,
	} {
		if _, err := Decode(context.Background(), []byte(c), 10_000); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := Decode(context.Background(), []byte(cat), 10_000); err != ErrTooExpensive {
		t.Errorf("budget: %v", err)
	}
	if !strings.Contains(MediaType, "malbolge") {
		t.Error("media type")
	}
}
