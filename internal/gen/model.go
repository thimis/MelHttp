// Package gen compiles arbitrary bytes into Malbolge programs that print them.
//
// # Layout of a generated program (one "chunk")
//
//	cells 0..41    prefix   "oji" + 37×"o" + "vo" (normalized ops)
//	cells 42..p0-1 setup    moves d into the fill region (see setup.go)
//	cells p0..     content  moves that print the data
//	               'v'      halt
//	               'o'…     padding up to N cells, N a multiple of 94
//
// The prefix's 'j' sets d = 39 and its 'i' jumps to cell 41, after which
// d == c-1 ("lag-1 mode"). In lag-1 mode [d] is always a re-encrypted
// printable character, and TestLag1CannotPrintAllBytes shows that this only
// ever reaches 355 values of a — 201 of the 256 byte values. So the setup
// writes a large value V into an early cell and executes two 'j's that bounce
// d through that cell to V+1, beyond the end of the program.
//
// Memory beyond the program is filled by the loader with
// mem[i] = crz(mem[i-1], mem[i-2]): a deterministic stream of full 10-trit
// values. Because every program ends with the same two cells at the same
// positions mod 94, that stream F is the same for every program ("fill mode").
// While the content executes, d walks through F one cell per instruction, so
// each cell's behavior depends only on its op and on F[index].
//
// In fill mode the useful moves are:
//
//	o  nop              a unchanged
//	*  a = rotr(F[k])   independent of a: a "reset"
//	p  a = crz(a, F[k])
//	<  print a % 256
//
// TestGeneratorCompleteness proves that from every reset in the usable range
// every byte is printable within a few cells, so any input can be compiled.
package gen

import "github.com/thimis/MelHttp/internal/malbolge"

// Move indices, used to name the op written at a cell.
const (
	mvNop  = 0
	mvRotr = 1
	mvCrz  = 2
	mvOut  = 3
	nMoves = 4
)

var moveOp = [nMoves]malbolge.Op{malbolge.OpNop, malbolge.OpRotr, malbolge.OpCrz, malbolge.OpOut}

// prefixOps is the normalized prefix; execution of the following cells starts
// at cell prefixLen in lag-1 mode with a = 0 and an encrypted 'o' before it.
const prefixOps = "oji" + "ooooooooooooooooooooooooooooooooooooo" + "vo"

const prefixLen = 42

// lag1Mem[prev][pos%94] is the value of [d] in lag-1 mode when the cell at
// pos executes, given that the op written at pos-1 was prev.
var lag1Mem [nMoves][94]uint16

func init() {
	if len(prefixOps) != prefixLen {
		panic("gen: bad prefix length")
	}
	for mv := range nMoves {
		for p := range 94 {
			prevPos := (p + 93) % 94 // (p-1) mod 94
			lag1Mem[mv][p] = malbolge.Encrypt(uint16(malbolge.Encode(moveOp[mv], prevPos)))
		}
	}
}

// lag1 is the machine state in lag-1 mode, just before the cell at pos executes.
type lag1 struct {
	a    uint16
	prev uint8 // move written at pos-1
	pos  int
}

func lag1Start() lag1 { return lag1{a: 0, prev: mvNop, pos: prefixLen} }

// step applies move mv at the current position.
func (s lag1) step(mv uint8) lag1 {
	m := lag1Mem[s.prev][s.pos%94]
	switch mv {
	case mvRotr:
		s.a = malbolge.Rotr(m)
	case mvCrz:
		s.a = malbolge.Crz(s.a, m)
	}
	s.prev = mv
	s.pos++
	return s
}
