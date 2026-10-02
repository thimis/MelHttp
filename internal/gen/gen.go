package gen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"runtime"
	"sync"

	"github.com/thimis/MelHttp/internal/malbolge"
)

// Options tunes compilation. The zero value gives deterministic, compact output.
type Options struct {
	// LocalDepth bounds the per-byte search from the current state before
	// falling back to "nops, then '*', then a table path" (default 4).
	LocalDepth int
	// MaxCells caps the cells of one program (default and maximum 59049,
	// minimum 4096).
	MaxCells int
	// SegmentSize is how many input bytes are compiled independently, and in
	// parallel; each segment yields one or more programs (default 16384).
	SegmentSize int
	// Workers bounds parallelism (default GOMAXPROCS).
	Workers int
	// Seed, when non-zero, randomizes the generated code (extra nops, search
	// order) so the same input yields different programs. Output stays exact.
	Seed uint64
	// LineWidth wraps the source every LineWidth characters (default 128,
	// negative disables). Whitespace is ignored by Malbolge.
	LineWidth int

	noSweep bool // tests: use tape chunks instead of sweep chunks
}

// MinCells is the smallest allowed Options.MaxCells.
const MinCells = 4096

func (o Options) withDefaults() (Options, error) {
	if o.LocalDepth == 0 {
		o.LocalDepth = 4
	}
	if o.LocalDepth < 0 || o.LocalDepth > 12 {
		return o, fmt.Errorf("gen: LocalDepth %d out of range 0..12", o.LocalDepth)
	}
	if o.MaxCells == 0 || o.MaxCells > malbolge.MemSize {
		o.MaxCells = malbolge.MemSize
	}
	if o.MaxCells < MinCells {
		return o, fmt.Errorf("gen: MaxCells %d is below the minimum %d", o.MaxCells, MinCells)
	}
	if o.SegmentSize <= 0 {
		o.SegmentSize = 16384
	}
	if o.Workers <= 0 {
		o.Workers = runtime.GOMAXPROCS(0)
	}
	if o.LineWidth == 0 {
		o.LineWidth = 128
	}
	return o, nil
}

// ErrTooLarge is returned by Program when the input does not fit in a single
// Malbolge program; use Compile to get chunks.
var ErrTooLarge = errors.New("gen: input does not fit in one Malbolge program")

// Program compiles data into a single Malbolge program.
func Program(data []byte, opt Options) ([]byte, error) {
	opt.SegmentSize = len(data) + 1
	chunks, err := Compile(data, opt)
	if err != nil {
		return nil, err
	}
	if len(chunks) != 1 {
		return nil, ErrTooLarge
	}
	return chunks[0], nil
}

// Compile compiles data into one or more Malbolge programs whose outputs,
// concatenated in order, equal data. Empty input yields one program that
// prints nothing. The result does not depend on Workers.
func Compile(data []byte, opt Options) ([][]byte, error) {
	opt, err := opt.withDefaults()
	if err != nil {
		return nil, err
	}
	tl, err := getTape()
	if err != nil {
		return nil, err
	}
	getLag1()
	var segs [][]byte
	for off := 0; off < len(data); off += opt.SegmentSize {
		segs = append(segs, data[off:min(off+opt.SegmentSize, len(data))])
	}
	if len(segs) == 0 {
		segs = [][]byte{nil}
	}
	results := make([][][]byte, len(segs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, opt.Workers)
	for i, seg := range segs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			e := &encoder{opt: opt, tape: tl}
			if opt.Seed != 0 {
				e.rng = rand.New(rand.NewPCG(opt.Seed, uint64(i)))
			}
			results[i] = e.compileSegment(seg)
		}()
	}
	wg.Wait()
	var chunks [][]byte
	for _, r := range results {
		chunks = append(chunks, r...)
	}
	return chunks, nil
}

// Verify runs every chunk in the VM and checks that the concatenated output
// equals want. melc verifies all generated code this way before writing it.
func Verify(ctx context.Context, chunks [][]byte, want []byte) error {
	var got bytes.Buffer
	for i, c := range chunks {
		p, err := malbolge.Load(c)
		if err != nil {
			return fmt.Errorf("gen: chunk %d does not load: %w", i, err)
		}
		res, err := p.Run(ctx, nil, &got, malbolge.Limits{MaxSteps: malbolge.MemSize})
		if err != nil {
			return fmt.Errorf("gen: chunk %d: %w", i, err)
		}
		if !res.Halted || res.ReadInput {
			return fmt.Errorf("gen: chunk %d: halted=%v read_input=%v", i, res.Halted, res.ReadInput)
		}
	}
	if !bytes.Equal(got.Bytes(), want) {
		i := 0
		for i < got.Len() && i < len(want) && got.Bytes()[i] == want[i] {
			i++
		}
		return fmt.Errorf("gen: output differs from input at byte %d (got %d bytes, want %d)", i, got.Len(), len(want))
	}
	return nil
}

type encoder struct {
	opt   Options
	tape  *tapeLayout
	rng   *rand.Rand
	buf   [16]uint8 // scratch for depth-first searches
	sweep sweepSearch
}

// lag1MinRun is the shortest run of lag-1-printable bytes worth its own
// (cheaper) lag-1 chunk when it interrupts tape-chunk data.
const lag1MinRun = 256

// minTape is the shortest tape a chunk uses.
const minTape = 64

// lag1Run counts the leading lag-1-printable bytes of data.
func lag1Run(t *lag1Tables, data []byte) int {
	for i, b := range data {
		if !t.printable[b] {
			return i
		}
	}
	return len(data)
}

// compileSegment splits data into lag-1 chunks (runs of printable bytes) and
// tape chunks (everything else).
func (e *encoder) compileSegment(data []byte) [][]byte {
	l1 := getLag1()
	var chunks [][]byte
	for {
		run := lag1Run(l1, data)
		if run == len(data) || run >= lag1MinRun {
			n, src := e.lag1Chunk(data[:run])
			chunks = append(chunks, src)
			if data = data[n:]; len(data) == 0 {
				return chunks
			}
			continue
		}
		var n int
		var src []byte
		if !e.opt.noSweep {
			n, src = e.sweepChunk(data, e.longRunAhead(l1, data))
		}
		if n == 0 {
			n, src = e.tapeChunkFit(l1, data) // proven fallback
		}
		chunks = append(chunks, src)
		if data = data[n:]; len(data) == 0 {
			return chunks
		}
	}
}

// tapeChunkFit builds one tape chunk from the front of data, shrinking the
// tape to the smallest size that still holds what the chunk consumed.
func (e *encoder) tapeChunkFit(l1 *lag1Tables, data []byte) (int, []byte) {
	tl := e.tape
	stop := e.longRunAhead(l1, data)
	tmax := min(tapeMax, (e.opt.MaxCells-tapeOverhead)/2)
	// Start from an estimate (about 20 content cells per byte) and double.
	t := min(tmax, max(minTape, 24*len(data)))
	n, used, src := e.tapeChunk(tl, data, t, stop)
	for n < len(data) && !stop(n) && t < tmax {
		t = min(tmax, 2*t)
		n, used, src = e.tapeChunk(tl, data, t, stop)
	}
	if n == 0 {
		panic("gen: tape chunk could not hold a single byte") // excluded by TestTapeCompleteness
	}
	if n == len(data) || stop(n) {
		// Shrink the tape to what the content needed.
		need := max(minTape, used+tl.sStar-prefixLen+1)
		step := max(94, (t-need)/16)
		for small := need; small < t; small += step {
			if n2, _, src2 := e.tapeChunk(tl, data[:n], small, stop); n2 == n {
				return n, src2
			}
		}
	}
	return n, src
}

// randomNops returns 0–3 nops a quarter of the time when seeded.
func (e *encoder) randomNops() []uint8 {
	if e.rng == nil || e.rng.IntN(4) != 0 {
		return nil
	}
	return make([]uint8, 1+e.rng.IntN(3)) // mvNop == 0
}

// moveOrder is the order the depth-first searches try moves in.
func (e *encoder) moveOrder() [3]uint8 {
	order := [3]uint8{mvCrz, mvRotr, mvNop}
	if e.rng != nil {
		e.rng.Shuffle(3, func(x, y int) { order[x], order[y] = order[y], order[x] })
	}
	return order
}

// assemble lays out the prefix followed by ops as Malbolge source.
func (e *encoder) assemble(ops []malbolge.Op) []byte {
	w := e.opt.LineWidth
	n := prefixLen + len(ops)
	src := make([]byte, 0, n+n/max(w, 1)+1)
	pos := 0
	emit := func(op malbolge.Op) {
		if w > 0 && pos > 0 && pos%w == 0 {
			src = append(src, '\n')
		}
		src = append(src, malbolge.Encode(op, pos))
		pos++
	}
	for i := range prefixLen {
		emit(malbolge.Op(prefixOps[i]))
	}
	for _, op := range ops {
		emit(op)
	}
	return append(src, '\n')
}

// longRunAhead returns a stop function for data: true at i > 0 when a run of
// lag1MinRun printable bytes starts there, which a lag-1 chunk prints cheaper.
func (e *encoder) longRunAhead(l1 *lag1Tables, data []byte) func(i int) bool {
	return func(i int) bool {
		if i == 0 {
			return false
		}
		rest := data[i:]
		return lag1Run(l1, rest[:min(len(rest), lag1MinRun)]) >= lag1MinRun
	}
}
