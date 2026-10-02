package gen

import (
	"github.com/thimis/MelHttp/internal/malbolge"
)

// Sweep chunks print any byte at roughly ASCII cost. Layout:
//
//	prefix | head: lag-1 moves | k nops | prev | 'j' | sweep moves (+ forced 'j's) | 'v'
//
// The head prints the leading lag-1-printable bytes (padded with filler moves
// to at least minRegion cells), leaving its usual trail of values. The 'j'
// moves d back to cell sStar, exactly like a tape chunk; but the cell before
// the 'j' — the anchor — still holds sStar-1. So whenever d reaches the
// anchor, another 'j' rewinds d to sStar, and the region [sStar, anchor) is
// read again and again. Every '*' or 'p' overwrites the region cell it just
// read with its result, so the region's values get richer with each sweep,
// and short searches find almost any byte (about 8–10 cells per byte, against
// ~35 for tape chunks).
//
// The region's starting contents are taken from the real VM (the head is
// executed with a Machine), so the model cannot drift from Malbolge. Sweep
// chunks have no static completeness proof; if one cannot make progress, the
// encoder falls back to a tape chunk, which has one.

const (
	minRegion   = 320 // cells of history the sweep region needs
	headMax     = 640 // cells the lag-1 head may use for content
	sweepDepth  = 8   // per-byte search depth in sweep mode
	sweepGiveUp = 400 // nops to skip without finding a byte before ending the chunk
)

// region is the self-renewing sweep region.
type region struct {
	mem    []uint16 // cell values, indexed by cell number (only [s, anchor] used)
	s      int      // first cell
	anchor int      // holds s-1; a 'j' here rewinds d to s
}

// swState is the machine state in sweep mode.
type swState struct {
	a uint16
	d int
}

// next applies move mv. If d is at the anchor, a forced 'j' (one cell) comes
// first. When write is set the region is updated; it returns the new state
// and the cells used.
func (g *region) next(st swState, mv uint8, write bool) (swState, int) {
	cells := 1
	if st.d == g.anchor {
		st.d = g.s
		cells++
	}
	m := g.mem[st.d]
	switch mv {
	case mvRotr:
		st.a = malbolge.Rotr(m)
		if write {
			g.mem[st.d] = st.a
		}
	case mvCrz:
		st.a = malbolge.Crz(st.a, m)
		if write {
			g.mem[st.d] = st.a
		}
	}
	st.d++
	return st, cells
}

// sweepChunk builds a sweep chunk from the front of data. It returns the
// number of bytes consumed (0 if the chunk could not print its first sweep
// byte) and the program source. stop reports whether to end before data[i].
func (e *encoder) sweepChunk(data []byte, stop func(i int) bool) (int, []byte) {
	tl := e.tape
	l1 := getLag1()

	// Head: leading printable bytes in lag-1 mode, then filler.
	var ops []malbolge.Op
	s := lag1Start()
	n := 0
	for n < len(data) && l1.printable[data[n]] && !(n > 0 && stop(n)) {
		path := e.lag1Path(l1, s, data[n])
		if s.pos+len(path) > prefixLen+headMax {
			break
		}
		for _, mv := range path {
			s = s.step(mv)
			ops = append(ops, moveOp[mv])
		}
		n++
	}
	for i := 0; s.pos < prefixLen+minRegion; i++ {
		mv := tl.moves[i%len(tl.moves)]
		s = s.step(mv)
		ops = append(ops, moveOp[mv])
	}
	r := s.pos % 94
	for range tl.pad[r] {
		ops = append(ops, malbolge.OpNop)
	}
	ops = append(ops, moveOp[tl.prev[r]], malbolge.OpMovD)
	p0 := prefixLen + len(ops)

	// Run the head on the real VM to get the region exactly.
	p, err := malbolge.Load(e.assemble(append(ops[:len(ops):len(ops)], malbolge.OpHalt)))
	if err != nil {
		panic("gen: sweep head does not load: " + err.Error())
	}
	m := p.NewMachine(nil, nil)
	for m.C != p0 {
		if err := m.Step(); err != nil || m.Halted {
			panic("gen: sweep head does not run")
		}
	}
	if m.D != tl.sStar || m.Mem[p0-2] != uint16(tl.sStar-1) {
		panic("gen: sweep head left d or the anchor in the wrong place")
	}
	g := &region{mem: append([]uint16(nil), m.Mem[:p0-1]...), s: tl.sStar, anchor: p0 - 2}
	st := swState{a: m.A, d: tl.sStar}

	limit := e.opt.MaxCells - 1 // room for 'v'
	headBytes := n
	for i := n; i < len(data); i++ {
		if i > headBytes && stop(i) {
			break
		}
		path, ok := e.sweepPath(g, st, data[i])
		if !ok {
			break
		}
		// Apply the path, emitting forced 'j's at the anchor.
		var add []malbolge.Op
		cur := st
		for _, mv := range path {
			if cur.d == g.anchor {
				add = append(add, malbolge.OpMovD)
			}
			cur, _ = g.next(cur, mv, false)
			add = append(add, moveOp[mv])
		}
		if prefixLen+len(ops)+len(add) > limit {
			break
		}
		for _, mv := range path {
			st, _ = g.next(st, mv, true)
		}
		ops = append(ops, add...)
		n = i + 1
	}
	if n == headBytes && headBytes < len(data) && !stop(n) {
		return 0, nil // no sweep progress: let the caller use a tape chunk
	}
	return n, e.assemble(append(ops, malbolge.OpHalt))
}

// sweepPath finds moves (ending with mvOut) that print b, skipping ahead
// with nops when the bytes ahead cannot reach it quickly.
func (e *encoder) sweepPath(g *region, st swState, b byte) ([]uint8, bool) {
	pre := e.randomNops()
	for range pre {
		st, _ = g.next(st, mvNop, false)
	}
	for skip := 0; skip <= sweepGiveUp; skip++ {
		if moves, ok := e.bfsSweep(g, st, b); ok {
			return append(append(pre, moves...), mvOut), true
		}
		pre = append(pre, mvNop)
		st, _ = g.next(st, mvNop, false)
	}
	return nil, false
}

// sweepSearch is reusable scratch space for bfsSweep.
type sweepSearch struct {
	nodes []swNode
	keys  []uint64 // open-addressing set of seen states, stamped by gen
	gens  []uint32
	gen   uint32
}

type swNode struct {
	st     swState
	parent int32
	mv     uint8
}

const seenBits = 14

// bfsSweep finds the fewest moves (at most sweepDepth) after which a%256 == b.
// Many move sequences reach the same (a, d) — every '*' discards a, nops keep
// it — so visiting each state once keeps the search small.
func (e *encoder) bfsSweep(g *region, root swState, b byte) ([]uint8, bool) {
	if byte(root.a) == b {
		return nil, true
	}
	sr := &e.sweep
	if sr.keys == nil {
		sr.keys = make([]uint64, 1<<seenBits)
		sr.gens = make([]uint32, 1<<seenBits)
	}
	sr.gen++
	sr.nodes = append(sr.nodes[:0], swNode{st: root, parent: -1})
	sr.seen(root)
	levelEnd := 1
	for depth, i := 1, 0; depth <= sweepDepth; depth++ {
		for ; i < levelEnd; i++ {
			n := sr.nodes[i]
			for _, mv := range e.moveOrder() {
				next, _ := g.next(n.st, mv, false)
				if !sr.seen(next) {
					continue
				}
				sr.nodes = append(sr.nodes, swNode{st: next, parent: int32(i), mv: mv})
				if byte(next.a) == b {
					return sr.path(len(sr.nodes) - 1), true
				}
			}
		}
		levelEnd = len(sr.nodes)
		if levelEnd > 1<<(seenBits-1) {
			break // the set is half full; give up on this position
		}
	}
	return nil, false
}

// seen marks st and reports whether it was new.
func (sr *sweepSearch) seen(st swState) bool {
	k := uint64(st.a)<<32 | uint64(st.d)
	mask := uint64(len(sr.keys) - 1)
	for h := (k * 0x9E3779B97F4A7C15) >> (64 - seenBits); ; h = (h + 1) & mask {
		if sr.gens[h] != sr.gen {
			sr.gens[h], sr.keys[h] = sr.gen, k
			return true
		}
		if sr.keys[h] == k {
			return false
		}
	}
}

func (sr *sweepSearch) path(i int) []uint8 {
	var rev []uint8
	for ; sr.nodes[i].parent >= 0; i = int(sr.nodes[i].parent) {
		rev = append(rev, sr.nodes[i].mv)
	}
	p := make([]uint8, len(rev))
	for j := range rev {
		p[j] = rev[len(rev)-1-j]
	}
	return p
}
