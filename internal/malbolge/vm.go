package malbolge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
)

var (
	// ErrStepLimit means the program executed Limits.MaxSteps instructions without halting.
	ErrStepLimit = errors.New("malbolge: step limit exceeded")
	// ErrOutputLimit means the program tried to write more than Limits.MaxOutput bytes.
	ErrOutputLimit = errors.New("malbolge: output limit exceeded")
	// ErrInvalidInstruction means execution reached a cell outside 33..126.
	// The reference interpreter spins forever there; we stop instead.
	ErrInvalidInstruction = errors.New("malbolge: execution reached a cell outside 33..126")
)

// Limits bounds a single run. Zero values mean unlimited.
type Limits struct {
	MaxSteps  int64 // maximum instructions executed
	MaxOutput int64 // maximum bytes written
}

// Result describes a finished (or stopped) run.
type Result struct {
	Steps  int64 // instructions executed, including the final halt
	Output int64 // bytes written
	// ReadInput reports whether the program executed an input instruction.
	// A program that never reads input is deterministic: its output depends
	// only on the program, so it can be cached.
	ReadInput bool
	Halted    bool // the program executed its halt instruction
}

// decode2 is Xlat1 twice, so the decode index ([c]-33) + (c mod 94) needs no modulo.
var decode2 [2 * 94]Op

func init() {
	for i := range decode2 {
		decode2[i] = Op(Xlat1[i%94])
	}
}

var memPool = sync.Pool{New: func() any { return new([MemSize]uint16) }}

const (
	outBufSize = 8192
	inBufSize  = 4096
	ctxEvery   = 1 << 16 // check for cancellation every 64K instructions
)

// Run executes the program on a private copy of its memory. Input is read
// lazily from in (nil means immediate EOF) and output is written to out.
// Output produced before an error is still written. Run is safe to call
// concurrently on the same Program.
func (p *Program) Run(ctx context.Context, in io.Reader, out io.Writer, lim Limits) (Result, error) {
	mem := memPool.Get().(*[MemSize]uint16)
	defer memPool.Put(mem)
	*mem = p.mem

	var (
		res        Result
		err        error
		a          uint16
		c, d, cmod int
		obuf       [outBufSize]byte
		on         int
		ibuf       [inBufSize]byte
		ipos, ilen int
		ieof       = in == nil
		done       = ctx.Done()
	)

loop:
	for {
		if lim.MaxSteps > 0 && res.Steps >= lim.MaxSteps {
			err = ErrStepLimit
			break
		}
		if done != nil && res.Steps%ctxEvery == 0 {
			select {
			case <-done:
				err = ctx.Err()
				break loop
			default:
			}
		}
		v := mem[c]
		if v < 33 || v > 126 {
			err = ErrInvalidInstruction
			break
		}
		res.Steps++
		switch decode2[int(v)-33+cmod] {
		case OpMovD:
			d = int(mem[d])
		case OpJmp:
			c = int(mem[d])
			cmod = c % 94
		case OpRotr:
			x := Rotr(mem[d])
			mem[d], a = x, x
		case OpCrz:
			x := Crz(a, mem[d])
			mem[d], a = x, x
		case OpOut:
			if lim.MaxOutput > 0 && res.Output >= lim.MaxOutput {
				err = ErrOutputLimit
				break loop
			}
			obuf[on] = byte(a)
			on++
			res.Output++
			if on == outBufSize {
				if _, werr := out.Write(obuf[:on]); werr != nil {
					return res, fmt.Errorf("malbolge: writing output: %w", werr)
				}
				on = 0
			}
		case OpIn:
			res.ReadInput = true
			for ipos == ilen && !ieof {
				n, rerr := in.Read(ibuf[:])
				ipos, ilen = 0, n
				if rerr == io.EOF {
					ieof = true
				} else if rerr != nil {
					return res, fmt.Errorf("malbolge: reading input: %w", rerr)
				}
			}
			if ipos < ilen {
				a = uint16(ibuf[ipos])
				ipos++
			} else if faultMode == "eof-zero" {
				a = 0
			} else {
				a = MaxWord
			}
		case OpHalt:
			res.Halted = true
			break loop
		}
		// Encrypt the cell at c (the jump target, after a jump). The reference
		// indexes out of bounds when that cell is outside 33..126; we leave it.
		if v := mem[c]; v >= 33 && v <= 126 && faultMode != "no-encrypt" {
			mem[c] = uint16(Xlat2[v-33])
		}
		if c == MaxWord {
			c, cmod = 0, 0
		} else {
			c++
			if cmod++; cmod == 94 {
				cmod = 0
			}
		}
		if d == MaxWord {
			d = 0
		} else {
			d++
		}
	}
	if on > 0 {
		if _, werr := out.Write(obuf[:on]); werr != nil {
			return res, fmt.Errorf("malbolge: writing output: %w", werr)
		}
	}
	return res, err
}

// RunBytes runs the program with the given input (nil means immediate EOF)
// and returns everything it printed.
func (p *Program) RunBytes(ctx context.Context, input []byte, lim Limits) ([]byte, Result, error) {
	var in io.Reader
	if input != nil {
		in = bytes.NewReader(input)
	}
	var out bytes.Buffer
	res, err := p.Run(ctx, in, &out, lim)
	return out.Bytes(), res, err
}
