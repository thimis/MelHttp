package malbolge

import (
	"strings"
	"testing"
)

// tern parses a base-3 string (most significant trit first).
func tern(t *testing.T, s string) uint16 {
	t.Helper()
	var v int
	for _, c := range s {
		if c < '0' || c > '2' {
			t.Fatalf("bad trit %q in %q", c, s)
		}
		v = v*3 + int(c-'0')
	}
	return uint16(v)
}

func TestConstants(t *testing.T) {
	if MemSize != 59049 || MaxWord != 59048 {
		t.Fatalf("MemSize=%d MaxWord=%d", MemSize, MaxWord)
	}
}

// The per-trit table from the reference interpreter, indexed [d][a].
var crzTrit = [3][3]uint16{{1, 0, 0}, {1, 0, 2}, {2, 2, 1}}

func TestCrzSingleTrits(t *testing.T) {
	for a := uint16(0); a < 3; a++ {
		for d := uint16(0); d < 3; d++ {
			got := Crz(a, d) % 3 // lowest trit; higher trits are crz(0,0)=1
			if got != crzTrit[d][a] {
				t.Errorf("crz(a=%d, d=%d) low trit = %d, want %d", a, d, got, crzTrit[d][a])
			}
		}
	}
}

func TestCrzKnownVectors(t *testing.T) {
	tests := []struct{ a, d, want string }{
		// Wikipedia's worked example; asymmetric, so it pins the argument order.
		{"0001112220", "0120120120", "1120020211"},
		{"0000000000", "0000000000", "1111111111"}, // 29524
		{"2222222222", "2222222222", "1111111111"},
		{"0000000000", "2222222222", "2222222222"},
		{"2222222222", "0000000000", "0000000000"},
		{"1111111111", "1111111111", "0000000000"},
	}
	for _, tt := range tests {
		got := Crz(tern(t, tt.a), tern(t, tt.d))
		if want := tern(t, tt.want); got != want {
			t.Errorf("crz(%s, %s) = %d, want %s (%d)", tt.a, tt.d, got, tt.want, want)
		}
	}
	if Crz(0, 0) != 29524 {
		t.Errorf("crz(0,0) = %d, want 29524", Crz(0, 0))
	}
	if Crz(0, 1) == Crz(1, 0) {
		t.Error("crz must not be symmetric: crz(0,1) == crz(1,0)")
	}
}

// crzSlow is an obviously-correct trit-by-trit implementation.
func crzSlow(a, d uint16) uint16 {
	var r, p uint16 = 0, 1
	for range 10 {
		r += crzTrit[d%3][a%3] * p
		a, d, p = a/3, d/3, p*3
	}
	return r
}

func TestCrzMatchesTritwiseDefinition(t *testing.T) {
	// Exhaustive over a stride covering all residues; full 59049² would be 3.5e9.
	for a := 0; a < MemSize; a += 7 {
		for d := 0; d < MemSize; d += 241 {
			if got, want := Crz(uint16(a), uint16(d)), crzSlow(uint16(a), uint16(d)); got != want {
				t.Fatalf("crz(%d,%d) = %d, want %d", a, d, got, want)
			}
		}
	}
	for a := MaxWord - 300; a < MemSize; a++ {
		for d := MaxWord - 300; d < MemSize; d++ {
			if got, want := Crz(uint16(a), uint16(d)), crzSlow(uint16(a), uint16(d)); got != want {
				t.Fatalf("crz(%d,%d) = %d, want %d", a, d, got, want)
			}
		}
	}
}

func TestRotr(t *testing.T) {
	tests := []struct{ in, want uint16 }{
		{0, 0}, {1, 19683}, {3, 1}, {2, 39366}, {MaxWord, MaxWord}, {19683, 6561},
	}
	for _, tt := range tests {
		if got := Rotr(tt.in); got != tt.want {
			t.Errorf("rotr(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
	for x := 0; x < MemSize; x++ {
		v := uint16(x)
		for range 10 {
			v = Rotr(v)
		}
		if v != uint16(x) {
			t.Fatalf("10 rotations of %d gave %d", x, v)
		}
	}
}

func TestTablesVerbatim(t *testing.T) {
	// Copied from Ben Olmstead's reference interpreter (malbolge.c). The
	// reference differential test (build tag "reference") also parses the
	// tables out of the C source and compares them independently.
	const wantX1 = "+b(29e*j1VMEKLyC})8&m#~W>qxdRp0wkrUo[D7,XTcA\"lI.v%{gJh4G\\-=O@5`_3i<?Z';FNQuY]szf$!BS/|t:Pn6^Ha"
	const wantX2 = "5z]&gqtyfr$(we4{WP)H-Zn,[%\\3dL+Q;>U!pJS72FhOA1CB6v^=I_0/8|jsb9m<.TVac`uY*MK'X~xDl}REokN:#?G\"i@"
	if Xlat1 != wantX1 {
		t.Errorf("Xlat1 differs from reference")
	}
	if Xlat2 != wantX2 {
		t.Errorf("Xlat2 differs from reference")
	}
	for name, s := range map[string]string{"xlat1": Xlat1, "xlat2": Xlat2} {
		if len(s) != 94 {
			t.Errorf("%s has %d chars, want 94", name, len(s))
		}
		for c := byte(33); c <= 126; c++ {
			if strings.Count(s, string(c)) != 1 {
				t.Errorf("%s: char %q appears %d times", name, c, strings.Count(s, string(c)))
			}
		}
	}
}

func TestDecode(t *testing.T) {
	// The first instruction of the Wikipedia hello world, '(' at position 0.
	if op := Decode('(', 0); op != OpMovD {
		t.Errorf("Decode('(', 0) = %v, want OpMovD", op)
	}
	// Every op is reachable at every position from exactly one char.
	for pos := 0; pos < 94; pos++ {
		seen := map[Op]int{}
		for c := 33; c <= 126; c++ {
			seen[Decode(uint16(c), pos)]++
		}
		for _, op := range []Op{OpJmp, OpOut, OpIn, OpRotr, OpMovD, OpCrz, OpNop, OpHalt} {
			if seen[op] != 1 {
				t.Errorf("pos %d: op %v produced by %d chars, want 1", pos, op, seen[op])
			}
		}
	}
	for _, op := range []Op{OpJmp, OpOut, OpIn, OpRotr, OpMovD, OpCrz, OpNop, OpHalt} {
		for pos := 0; pos < 300; pos++ {
			c := Encode(op, pos)
			if c < 33 || c > 126 || Decode(uint16(c), pos) != op {
				t.Fatalf("Encode(%v, %d) = %q does not decode back", op, pos, c)
			}
		}
	}
}

func TestEncrypt(t *testing.T) {
	for c := uint16(33); c <= 126; c++ {
		if got := Encrypt(c); got != uint16(Xlat2[c-33]) {
			t.Errorf("Encrypt(%d) = %d", c, got)
		}
	}
}

func BenchmarkCrz(b *testing.B) {
	var s uint16
	for i := range b.N {
		s += Crz(uint16(i%MemSize), s%MemSize)
	}
	_ = s
}
