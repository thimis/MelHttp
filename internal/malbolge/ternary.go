// Package malbolge implements the Malbolge virtual machine exactly as Ben
// Olmstead's reference interpreter (malbolge.c, 1998) behaves, plus the
// loader. It is pure Go with no OS or file I/O so it can also be compiled to
// WebAssembly.
package malbolge

const (
	// Trits is the number of ternary digits in a machine word.
	Trits = 10
	// MemSize is the number of memory cells (3^10).
	MemSize = 59049
	// MaxWord is the largest word value (3^10 - 1, "2222222222" in ternary).
	MaxWord = MemSize - 1

	half    = 243   // 3^5: a word splits into two independent 5-trit halves
	topTrit = 19683 // 3^9
)

// crzHalf[a*243+d] is the crazy operation on 5-trit values a and d. The crazy
// operation is tritwise, so a 10-trit result is crzHalf(lo) + 243*crzHalf(hi).
var crzHalf [half * half]uint8

func init() {
	trit := [3][3]int{{1, 0, 0}, {1, 0, 2}, {2, 2, 1}} // [d][a]
	for a := range half {
		for d := range half {
			r, p, x, y := 0, 1, a, d
			for range 5 {
				r += trit[y%3][x%3] * p
				x, y, p = x/3, y/3, p*3
			}
			crzHalf[a*half+d] = uint8(r)
		}
	}
}

// Crz is Malbolge's "crazy" operation, as executed by the crz instruction:
// a = [d] = Crz(a, [d]). It is not commutative; argument order matters.
func Crz(a, d uint16) uint16 {
	if faultMode == "crz-swap" {
		a, d = d, a
	}
	lo := crzHalf[int(a%half)*half+int(d%half)]
	hi := crzHalf[int(a/half)*half+int(d/half)]
	return uint16(hi)*half + uint16(lo)
}

// Rotr rotates a word one trit to the right.
func Rotr(x uint16) uint16 {
	return x/3 + x%3*topTrit
}
