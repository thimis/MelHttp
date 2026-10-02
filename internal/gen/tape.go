package gen

import (
	"errors"
	"math/rand/v2"
	"sync"

	"github.com/thimis/MelHttp/internal/malbolge"
)

// Tape chunks print any byte. Layout:
//
//	prefix | tape: T universal lag-1 moves | k nops | prev | 'j' | content | 'v'
//
// In lag-1 mode every '*' or 'p' writes the new a into the previous cell, so
// the tape leaves a trail of large values (cell x holds the a computed at
// cell x+1). The 'j' then sets d = [t-1] = sStar-1 (chosen via k and prev),
// so content cells read the trail, one cell per instruction, starting at cell
// sStar. Because the tape moves are fixed, the values read (the "stream") are
// the same for every chunk, and the reach table and completeness proof cover
// every tape chunk.

// tapeMax is the longest tape; a tape chunk then holds about as many content
// cells, which fills the 59049-cell memory.
const tapeMax = 29400

// tapeSeed and the move mix were chosen by measuring the average and worst
// cost per byte over the whole stream (see TestTapeCompleteness).
const tapeSeed = 42

type tapeLayout struct {
	moves  []uint8 // the universal tape
	states []lag1  // states[T] is the lag-1 state after T tape moves
	sStar  int     // first cell read by content
	stream []uint16
	reach  *reachTable
	pad    [94]int   // nops needed after the tape, by (prefixLen+T)%94
	prev   [94]uint8 // op before the 'j', by (prefixLen+T)%94
}

var (
	tapeOnce sync.Once
	theTape  *tapeLayout
	tapeErr  error
)

func getTape() (*tapeLayout, error) {
	tapeOnce.Do(func() { theTape, tapeErr = buildTape() })
	return theTape, tapeErr
}

func tapeMoves() []uint8 {
	rng := rand.New(rand.NewPCG(tapeSeed, 1))
	mix := [...]uint8{mvRotr, mvCrz, mvCrz, mvNop}
	moves := make([]uint8, tapeMax)
	for i := range moves {
		moves[i] = mix[rng.IntN(len(mix))]
	}
	return moves
}

func buildTape() (*tapeLayout, error) {
	tl := &tapeLayout{moves: tapeMoves()}
	tl.states = make([]lag1, tapeMax+1)
	tl.states[0] = lag1Start()
	for i, mv := range tl.moves {
		tl.states[i+1] = tl.states[i].step(mv)
	}
	if !tl.chooseStart() {
		return nil, errors.New("gen: no tape start cell works for every position")
	}
	// Run the real VM over prefix + tape to read off the trail exactly.
	ops := []malbolge.Op(nil)
	for _, mv := range tl.moves {
		ops = append(ops, moveOp[mv])
	}
	e := &encoder{opt: Options{LineWidth: -1}}
	p, err := malbolge.Load(e.assemble(append(ops, malbolge.OpHalt)))
	if err != nil {
		return nil, err
	}
	m := p.NewMachine(nil, nil)
	for m.C != prefixLen+tapeMax {
		if err := m.Step(); err != nil || m.Halted {
			return nil, errors.New("gen: tape does not run")
		}
	}
	if m.A != tl.states[tapeMax].a {
		return nil, errors.New("gen: lag-1 model disagrees with the VM")
	}
	// Cells sStar .. prefixLen+tapeMax-2 are final (the last tape cell may
	// still be overwritten by the op after the tape).
	tl.stream = append([]uint16(nil), m.Mem[tl.sStar:prefixLen+tapeMax-1]...)
	tl.reach = buildReach(tl.stream)
	return tl, nil
}

// chooseStart picks the start cell sStar that needs the fewest padding nops
// in the worst case, and records the padding and 'prev' op per residue.
func (tl *tapeLayout) chooseStart() bool {
	bestWorst := 1 << 30
	for s := 34; s <= 127; s++ {
		var pad [94]int
		var prev [94]uint8
		worst := 0
		ok := true
		for r := range 94 {
			found := false
			for k := 0; k < 94 && !found; k++ {
				t := r + k + 1 // the 'j' cell, relative to the cell after the tape
				for _, pv := range [...]uint8{mvNop, mvRotr, mvCrz} {
					if int(lag1Mem[pv][t%94]) == s-1 {
						pad[r], prev[r], found = k, pv, true
						break
					}
				}
			}
			if !found {
				ok = false
				break
			}
			worst = max(worst, pad[r])
		}
		if ok && worst < bestWorst {
			bestWorst, tl.sStar, tl.pad, tl.prev = worst, s, pad, prev
		}
	}
	return bestWorst < 1<<30
}

// tapeOverhead is an upper bound on cells besides tape and content.
const tapeOverhead = prefixLen + 94 + 3

// tapeChunk compiles leading bytes of data into a tape chunk with a tape of T
// moves. It returns how many bytes it consumed, how many content cells it
// used, and the source. stop reports whether to end the chunk before data[i].
func (e *encoder) tapeChunk(tl *tapeLayout, data []byte, T int, stop func(i int) bool) (int, int, []byte) {
	r := (prefixLen + T) % 94
	ops := make([]malbolge.Op, 0, 2*T+tapeOverhead)
	for _, mv := range tl.moves[:T] {
		ops = append(ops, moveOp[mv])
	}
	st := tl.states[T]
	for range tl.pad[r] {
		ops = append(ops, malbolge.OpNop)
		st = st.step(mvNop)
	}
	ops = append(ops, moveOp[tl.prev[r]], malbolge.OpMovD)
	st = st.step(tl.prev[r])

	s := sstate{a: st.a, k: 0}
	limit := prefixLen + T - 1 - tl.sStar
	content := 0
	n := 0
	for i, b := range data {
		if stop(i) {
			break
		}
		path := e.streamPath(tl.reach, s, b, limit)
		if path == nil || prefixLen+len(ops)+len(path)+1 > e.opt.MaxCells {
			break
		}
		for _, mv := range path {
			s = s.step(tl.stream, mv)
			ops = append(ops, moveOp[mv])
		}
		content += len(path)
		n++
	}
	return n, content, e.assemble(append(ops, malbolge.OpHalt))
}
