package malbolge

import "fmt"

// Program is a loaded, validated Malbolge memory image. It is immutable and
// safe to share; each run works on its own copy.
type Program struct {
	mem [MemSize]uint16
	n   int
}

// Len returns the number of cells occupied by the source program.
func (p *Program) Len() int { return p.n }

// LoadError describes why a source file is not a valid Malbolge program.
type LoadError struct {
	Offset    int  // byte offset in the source
	Index     int  // memory cell the character would occupy
	Line, Col int  // 1-based position in the source
	Char      byte // offending byte (0 for length errors)
	Reason    string
}

func (e *LoadError) Error() string {
	if e.Char == 0 && e.Reason != "invalid character" {
		return "malbolge: " + e.Reason
	}
	return fmt.Sprintf("malbolge: %s %q at line %d col %d (cell %d)", e.Reason, e.Char, e.Line, e.Col, e.Index)
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\v' || c == '\f' || c == '\r'
}

// Load parses Malbolge source the way the reference interpreter does:
// whitespace is skipped, every other character must be printable ASCII that
// decodes to one of the eight instructions at its position, and memory after
// the program is filled with mem[i] = crz(mem[i-1], mem[i-2]).
//
// Deviations from the reference, which has undefined behavior in these cases:
// bytes outside 33..126 are rejected, and programs shorter than 2 cells are
// rejected.
func Load(src []byte) (*Program, error) {
	p := new(Program)
	line, col := 1, 0
	for off, c := range src {
		col++
		if c == '\n' {
			line, col = line+1, 0
		}
		if isSpace(c) {
			continue
		}
		if p.n == MemSize {
			return nil, &LoadError{Offset: off, Index: p.n, Line: line, Col: col,
				Reason: fmt.Sprintf("program too long (more than %d cells)", MemSize)}
		}
		if c < 33 || c > 126 {
			return nil, &LoadError{Offset: off, Index: p.n, Line: line, Col: col, Char: c, Reason: "invalid character"}
		}
		if !Decode(uint16(c), p.n).Valid() {
			return nil, &LoadError{Offset: off, Index: p.n, Line: line, Col: col, Char: c, Reason: "invalid instruction"}
		}
		p.mem[p.n] = uint16(c)
		p.n++
	}
	if p.n < 2 {
		return nil, &LoadError{Reason: "program too short (need at least 2 instructions)"}
	}
	fillMemory(&p.mem, p.n)
	return p, nil
}

// fillMemory sets mem[i] = crz(mem[i-1], mem[i-2]) for i >= n. Each value
// depends only on the previous two, so once a pair (mem[i-1], mem[i-2])
// repeats with period q, the rest of memory repeats with period q too. The
// fill enters such a short cycle almost immediately, so after detecting it the
// remaining cells are copied instead of computed.
func fillMemory(mem *[MemSize]uint16, n int) {
	const maxPeriod = 16
	i := n
	for ; i < MemSize; i++ {
		if faultMode == "fill-swap" {
			mem[i] = Crz(mem[i-2], mem[i-1])
		} else {
			mem[i] = Crz(mem[i-1], mem[i-2])
		}
		for q := 1; q <= maxPeriod && i-1-q >= n; q++ {
			if mem[i] == mem[i-q] && mem[i-1] == mem[i-1-q] {
				for j := i + 1; j < MemSize; j++ {
					mem[j] = mem[j-q]
				}
				return
			}
		}
	}
}
