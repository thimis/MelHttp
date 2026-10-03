package malbolge

import (
	"errors"
	"math/rand/v2"
	"strings"
	"testing"
)

const helloSrc = "(=<`#9]~6ZY327Uv4-QsqpMn&+Ij\"'E%e{Ab~w=_:]Kw%o44Uqp0/Q?xNvL:`H%c#DD2^WV>gY;dts76qKJImZkj"

func TestLoadHello(t *testing.T) {
	p, err := Load([]byte(helloSrc))
	if err != nil {
		t.Fatal(err)
	}
	if p.Len() != len(helloSrc) {
		t.Fatalf("Len = %d, want %d", p.Len(), len(helloSrc))
	}
	for i := range len(helloSrc) {
		if p.mem[i] != uint16(helloSrc[i]) {
			t.Fatalf("mem[%d] = %d, want %d", i, p.mem[i], helloSrc[i])
		}
	}
}

func TestLoadFillsRemainingMemoryWithCrz(t *testing.T) {
	p, err := Load([]byte(helloSrc))
	if err != nil {
		t.Fatal(err)
	}
	for i := p.Len(); i < MemSize; i++ {
		if want := crzSlow(p.mem[i-1], p.mem[i-2]); p.mem[i] != want {
			t.Fatalf("mem[%d] = %d, want crz(mem[i-1], mem[i-2]) = %d", i, p.mem[i], want)
		}
	}
}

func TestLoadSkipsWhitespace(t *testing.T) {
	spaced := " \t" + helloSrc[:10] + "\r\n\v\f" + helloSrc[10:40] + "\n\n" + helloSrc[40:] + "\r\n"
	a, err := Load([]byte(spaced))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Load([]byte(helloSrc))
	if a.mem != b.mem {
		t.Fatal("whitespace changed the loaded image")
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name, src, reason string
		offset            int
	}{
		{"empty", "", "too short", 0},
		{"whitespace only", " \n\t", "too short", 0},
		{"one char", "(", "too short", 0},
		// 'a' at index 0 decodes to Xlat1[('a'-33)%94] = Xlat1[64] = '\'' — not an instruction.
		{"invalid op", "aa", "invalid instruction", 0},
		{"invalid op later", helloSrc[:5] + "a" + helloSrc[6:], "invalid instruction", 5},
		{"control char", "(\x01", "invalid character", 1},
		{"del", "(\x7f", "invalid character", 1},
		{"high byte", "(\xc3\xa9", "invalid character", 1},
		{"nul", "(\x00", "invalid character", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load([]byte(tt.src))
			var le *LoadError
			if !errors.As(err, &le) {
				t.Fatalf("Load error = %v, want *LoadError", err)
			}
			if !strings.Contains(le.Error(), tt.reason) || le.Offset != tt.offset {
				t.Fatalf("error %q at offset %d, want %q at offset %d", le, le.Offset, tt.reason, tt.offset)
			}
		})
	}
}

func TestLoadTooLong(t *testing.T) {
	// Build a valid program of MemSize+1 nops.
	var b strings.Builder
	for i := 0; i <= MemSize; i++ {
		b.WriteByte(Encode(OpNop, i))
	}
	src := b.String()
	if _, err := Load([]byte(src[:MemSize])); err != nil {
		t.Fatalf("program of exactly MemSize cells rejected: %v", err)
	}
	_, err := Load([]byte(src))
	if err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("Load(MemSize+1 cells) error = %v, want too long", err)
	}
}

func TestLoadErrorReportsLineAndColumn(t *testing.T) {
	_, err := Load([]byte(helloSrc[:5] + "\n  a"))
	var le *LoadError
	if !errors.As(err, &le) {
		t.Fatal(err)
	}
	if le.Line != 2 || le.Col != 3 || le.Index != 5 {
		t.Fatalf("line %d col %d index %d, want 2 3 5", le.Line, le.Col, le.Index)
	}
}

func FuzzLoad(f *testing.F) {
	f.Add([]byte(helloSrc))
	f.Add([]byte("(=BA#9\"=<;:3y7x54-21q/p-,+*)\"!h%B0/."))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, src []byte) {
		p, err := Load(src)
		if err != nil {
			return
		}
		if p.Len() < 2 || p.Len() > MemSize {
			t.Fatalf("accepted program with %d cells", p.Len())
		}
		for i := range p.Len() {
			if v := p.mem[i]; v < 33 || v > 126 || !Decode(v, i).Valid() {
				t.Fatalf("accepted invalid cell %d = %d", i, v)
			}
		}
	})
}

func TestFillShortcutMatchesNaiveFill(t *testing.T) {
	r := rand.New(rand.NewPCG(9, 9))
	for range 300 {
		n := 2 + r.IntN(3000)
		src := make([]byte, n)
		for i := range src {
			src[i] = Encode(Ops[r.IntN(len(Ops))], i)
		}
		p, err := Load(src)
		if err != nil {
			t.Fatal(err)
		}
		var naive [MemSize]uint16
		copy(naive[:], p.mem[:n])
		for i := n; i < MemSize; i++ {
			naive[i] = Crz(naive[i-1], naive[i-2])
		}
		if naive != p.mem {
			t.Fatalf("fill shortcut differs from naive fill for a %d-cell program", n)
		}
	}
}

func BenchmarkLoad(b *testing.B) {
	for b.Loop() {
		Load([]byte(helloSrc))
	}
}
