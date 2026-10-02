package gen

import (
	"runtime"
	"sync"

	"github.com/thimis/MelHttp/internal/malbolge"
)

// Stream mode: d walks forward through a fixed sequence of known cell values
// (the tape), so the cell executed at content step k sees [d] = stream[k].
//
//	o  nop                  a unchanged
//	*  a = rotr(stream[k])   independent of a: a "reset"
//	p  a = crz(a, stream[k])
//	<  print a % 256

// sstate is the machine state in stream mode: the next cell reads stream[k].
type sstate struct {
	a uint16
	k int
}

func (s sstate) step(stream []uint16, mv uint8) sstate {
	switch mv {
	case mvRotr:
		s.a = malbolge.Rotr(stream[s.k])
	case mvCrz:
		s.a = malbolge.Crz(s.a, stream[s.k])
	}
	s.k++
	return s
}

// resetDepth bounds the search after a reset.
const resetDepth = 5

// reachTable records, for every stream index k, which bytes can be printed
// within exactly d moves after a '*' that reads stream[k].
type reachTable struct {
	stream []uint16
	exact  [][resetDepth + 1][4]uint64 // exact[k][d] is a 256-bit set
	any    [][4]uint64                 // union over d
}

func buildReach(stream []uint16) *reachTable {
	n := max(0, len(stream)-resetDepth-1)
	r := &reachTable{
		stream: stream,
		exact:  make([][resetDepth + 1][4]uint64, n),
		any:    make([][4]uint64, n),
	}
	workers := runtime.GOMAXPROCS(0)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := w; k < n; k += workers {
				r.walk(k, sstate{a: malbolge.Rotr(stream[k]), k: k + 1}, 0)
				for d := range resetDepth + 1 {
					for i := range 4 {
						r.any[k][i] |= r.exact[k][d][i]
					}
				}
			}
		}()
	}
	wg.Wait()
	return r
}

func (r *reachTable) walk(k int, s sstate, d int) {
	b := byte(s.a)
	r.exact[k][d][b/64] |= 1 << (b % 64)
	if d == resetDepth {
		return
	}
	for _, mv := range [...]uint8{mvCrz, mvRotr, mvNop} {
		r.walk(k, s.step(r.stream, mv), d+1)
	}
}

func has(set *[4]uint64, b byte) bool { return set[b/64]&(1<<(b%64)) != 0 }

// next returns the smallest k' ≥ k such that b is printable after a reset at
// k', or -1 if there is none in the table.
func (r *reachTable) next(k int, b byte) int {
	for ; k < len(r.any); k++ {
		if has(&r.any[k], b) {
			return k
		}
	}
	return -1
}

// depth returns the fewest moves after a reset at k that leave a%256 == b.
func (r *reachTable) depth(k int, b byte) int {
	for d := range resetDepth + 1 {
		if has(&r.exact[k][d], b) {
			return d
		}
	}
	return -1
}

// streamPath returns moves, ending with mvOut, that print b from state s
// without reading beyond stream index limit, or nil if that is impossible.
func (e *encoder) streamPath(r *reachTable, s sstate, b byte, limit int) []uint8 {
	pre := e.randomNops()
	for range pre {
		s = s.step(r.stream, mvNop)
	}
	fb := 1 << 30
	k := r.next(s.k, b)
	if k >= 0 {
		fb = (k - s.k) + 1 + r.depth(k, b) + 1
	}
	for depth := 0; depth <= e.opt.LocalDepth && depth+1 < fb && s.k+depth < limit; depth++ {
		if e.dfsStream(r.stream, s, b, 0, depth) {
			return append(append(pre, e.buf[:depth]...), mvOut)
		}
	}
	if k < 0 || s.k+fb > limit {
		return nil
	}
	p := pre
	for ; s.k < k; s = s.step(r.stream, mvNop) {
		p = append(p, mvNop)
	}
	p = append(p, mvRotr)
	s = s.step(r.stream, mvRotr)
	d := r.depth(k, b)
	if !e.dfsStream(r.stream, s, b, 0, d) {
		panic("gen: reach table inconsistent") // guarded by the completeness tests
	}
	return append(append(p, e.buf[:d]...), mvOut)
}

func (e *encoder) dfsStream(stream []uint16, s sstate, b byte, i, depth int) bool {
	if i == depth {
		return byte(s.a) == b
	}
	for _, mv := range e.moveOrder() {
		e.buf[i] = mv
		if e.dfsStream(stream, s.step(stream, mv), b, i+1, depth) {
			return true
		}
	}
	return false
}
