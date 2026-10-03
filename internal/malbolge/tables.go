package malbolge

import (
	"fmt"
	"strings"
)

// Xlat1 decodes an instruction: op = Xlat1[([c]-33+c) % 94].
const Xlat1 = "+b(29e*j1VMEKLyC})8&m#~W>qxdRp0wkrUo[D7,XTcA\"lI.v%{gJh4G\\-=O@5`_3i<?Z';FNQuY]szf$!BS/|t:Pn6^Ha"

// Xlat2 encrypts the executed cell: [c] = Xlat2[[c]-33].
const Xlat2 = "5z]&gqtyfr$(we4{WP)H-Zn,[%\\3dL+Q;>U!pJS72FhOA1CB6v^=I_0/8|jsb9m<.TVac`uY*MK'X~xDl}REokN:#?G\"i@"

// Op is a decoded instruction, named by its character in Xlat1 (the
// "normalized" Malbolge form).
type Op byte

// The eight instructions. Any other decoded value is a no-op at run time and
// rejected by the loader.
const (
	OpJmp  Op = 'i' // c = [d]
	OpOut  Op = '<' // write a % 256
	OpIn   Op = '/' // a = read byte, or 59048 on EOF
	OpRotr Op = '*' // a = [d] = rotr([d])
	OpMovD Op = 'j' // d = [d]
	OpCrz  Op = 'p' // a = [d] = crz(a, [d])
	OpNop  Op = 'o'
	OpHalt Op = 'v'
)

// Ops lists the eight valid instructions.
var Ops = [...]Op{OpJmp, OpOut, OpIn, OpRotr, OpMovD, OpCrz, OpNop, OpHalt}

// Valid reports whether op is one of the eight instructions.
func (op Op) Valid() bool { return strings.IndexByte("i</*jpov", byte(op)) >= 0 }

func (op Op) String() string {
	switch op {
	case OpJmp:
		return "jmp"
	case OpOut:
		return "out"
	case OpIn:
		return "in"
	case OpRotr:
		return "rotr"
	case OpMovD:
		return "movd"
	case OpCrz:
		return "crz"
	case OpNop:
		return "nop"
	case OpHalt:
		return "halt"
	}
	return fmt.Sprintf("Op(%q)", byte(op))
}

// opIndex[op] is the position of op in Xlat1.
var opIndex [128]int

func init() {
	for i := range len(Xlat1) {
		opIndex[Xlat1[i]] = i
	}
}

// Decode returns the instruction for cell value v (33..126) at address pos.
func Decode(v uint16, pos int) Op {
	return Op(Xlat1[(int(v)-33+pos)%94])
}

// Encode returns the source character that decodes to op at address pos.
func Encode(op Op, pos int) byte {
	return byte((opIndex[op]-pos%94+94)%94 + 33)
}

// Encrypt returns the value a cell holding v (33..126) takes after it executes.
func Encrypt(v uint16) uint16 {
	return uint16(Xlat2[v-33])
}
