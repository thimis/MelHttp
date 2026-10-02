package malbolge

import (
	"errors"
	"io"
)

// Machine is a single-stepping view of a run, for tools and tests that need
// to inspect registers and memory (debuggers, generator verification). Use
// Program.Run for normal execution; it is much faster.
type Machine struct {
	Mem     [MemSize]uint16
	A       uint16
	C, D    int
	Steps   int64
	Halted  bool
	in      io.ByteReader
	out     io.ByteWriter
	scratch [1]byte
}

// NewMachine returns a machine at the start of the program. in may be nil
// (immediate EOF) and out may be nil (output discarded).
func (p *Program) NewMachine(in io.ByteReader, out io.ByteWriter) *Machine {
	m := &Machine{in: in, out: out}
	m.Mem = p.mem
	return m
}

// Op returns the instruction at C, or an invalid Op if the cell is out of range.
func (m *Machine) Op() Op {
	v := m.Mem[m.C]
	if v < 33 || v > 126 {
		return 0
	}
	return Decode(v, m.C)
}

// Step executes one instruction, with exactly the semantics of Program.Run.
func (m *Machine) Step() error {
	if m.Halted {
		return errors.New("malbolge: machine halted")
	}
	v := m.Mem[m.C]
	if v < 33 || v > 126 {
		return ErrInvalidInstruction
	}
	m.Steps++
	switch Decode(v, m.C) {
	case OpMovD:
		m.D = int(m.Mem[m.D])
	case OpJmp:
		m.C = int(m.Mem[m.D])
	case OpRotr:
		x := Rotr(m.Mem[m.D])
		m.Mem[m.D], m.A = x, x
	case OpCrz:
		x := Crz(m.A, m.Mem[m.D])
		m.Mem[m.D], m.A = x, x
	case OpOut:
		if m.out != nil {
			if err := m.out.WriteByte(byte(m.A)); err != nil {
				return err
			}
		}
	case OpIn:
		m.A = MaxWord
		if m.in != nil {
			if b, err := m.in.ReadByte(); err == nil {
				m.A = uint16(b)
			} else if err != io.EOF {
				return err
			}
		}
	case OpHalt:
		m.Halted = true
		return nil
	}
	if v := m.Mem[m.C]; v >= 33 && v <= 126 {
		m.Mem[m.C] = uint16(Xlat2[v-33])
	}
	m.C = (m.C + 1) % MemSize
	m.D = (m.D + 1) % MemSize
	return nil
}
