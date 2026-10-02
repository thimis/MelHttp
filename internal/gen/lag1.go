package gen

import (
	"runtime"
	"sync"

	"github.com/thimis/MelHttp/internal/malbolge"
)

// Lag-1 chunks: prefix, then content moves in lag-1 mode, then 'v'.
//
// After any '*' the lag-1 state is one of 4×94 "reset states" (a no longer
// depends on its old value). lag1Tables holds, for every reset state and
// byte, a shortest move sequence printing the byte, plus the cheapest
// "k nops, '*', table path" fallback from every (prev, pos%94).

const nResets = nMoves * 94

func resetIndex(prev uint8, pos int) int { return int(prev)*94 + pos%94 }

func lag1Reset(prev uint8, pos94 int) lag1 {
	return lag1{a: malbolge.Rotr(lag1Mem[prev][pos94]), prev: mvRotr, pos: pos94 + 1}
}

const unreachable = 0xffff

type lag1Tables struct {
	path      [nResets][256][]uint8 // nil when unreachable from that reset
	fbLen     [nResets][256]uint16  // unreachable when no reset reaches the byte
	fbNops    [nResets][256]uint8
	printable [256]bool // bytes lag-1 mode can print at all
}

var (
	lag1Once sync.Once
	theLag1  *lag1Tables
)

func getLag1() *lag1Tables {
	lag1Once.Do(func() { theLag1 = buildLag1() })
	return theLag1
}

func buildLag1() *lag1Tables {
	t := new(lag1Tables)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			seen := newBitset(malbolge.MemSize * nResets)
			for r := range jobs {
				t.fill(r, seen)
			}
		}()
	}
	for r := range nResets {
		jobs <- r
	}
	close(jobs)
	wg.Wait()
	for r := range nResets {
		for b := range 256 {
			if t.path[r][b] != nil {
				t.printable[b] = true
			}
		}
	}
	for r := range nResets {
		prev, pos94 := uint8(r/94), r%94
		for b := range 256 {
			best, bestK := unreachable, 0
			p := prev
			for k := range 94 {
				if path := t.path[resetIndex(p, pos94+k)][b]; path != nil {
					if cost := k + 1 + len(path); cost < best {
						best, bestK = cost, k
					}
				}
				p = mvNop
			}
			t.fbLen[r][b], t.fbNops[r][b] = uint16(best), uint8(bestK)
		}
	}
	return t
}

// fill runs a breadth-first search over (a, prev, pos%94) from reset state r
// until the reachable set is exhausted, recording the first path to each byte.
func (t *lag1Tables) fill(r int, seen *bitset) {
	seen.clear()
	type node struct {
		s      lag1
		parent int32
		mv     uint8
	}
	root := lag1Reset(uint8(r/94), r%94)
	root.pos %= 94
	seen.add(lag1Key(root))
	nodes := []node{{s: root, parent: -1}}
	found := 0
	for i := 0; i < len(nodes) && found < 256; i++ {
		n := nodes[i]
		if b := byte(n.s.a); t.path[r][b] == nil {
			var rev []uint8
			for j := int32(i); nodes[j].parent >= 0; j = nodes[j].parent {
				rev = append(rev, nodes[j].mv)
			}
			p := make([]uint8, 0, len(rev)+1)
			for j := len(rev) - 1; j >= 0; j-- {
				p = append(p, rev[j])
			}
			t.path[r][b] = append(p, mvOut)
			found++
		}
		for _, mv := range [...]uint8{mvNop, mvRotr, mvCrz} {
			next := n.s.step(mv)
			next.pos %= 94
			if seen.add(lag1Key(next)) {
				nodes = append(nodes, node{next, int32(i), mv})
			}
		}
	}
}

// lag1Chunk compiles the longest prefix of data that fits one lag-1 program.
// Every byte of data must be lag-1 printable.
func (e *encoder) lag1Chunk(data []byte) (int, []byte) {
	t := getLag1()
	ops := []malbolge.Op(nil)
	s := lag1Start()
	n := 0
	for _, b := range data {
		path := e.lag1Path(t, s, b)
		if s.pos+len(path)+1 > e.opt.MaxCells {
			break
		}
		for _, mv := range path {
			s = s.step(mv)
			ops = append(ops, moveOp[mv])
		}
		n++
	}
	return n, e.assemble(append(ops, malbolge.OpHalt))
}

// lag1Path returns moves, ending with mvOut, that print b from state s.
func (e *encoder) lag1Path(t *lag1Tables, s lag1, b byte) []uint8 {
	pre := e.randomNops()
	for range pre {
		s = s.step(mvNop)
	}
	r := resetIndex(s.prev, s.pos)
	best := int(t.fbLen[r][b])
	for depth := 0; depth <= e.opt.LocalDepth && depth+1 < best; depth++ {
		if e.dfsLag1(s, b, 0, depth) {
			return append(append(pre, e.buf[:depth]...), mvOut)
		}
	}
	p := pre
	for range t.fbNops[r][b] {
		p = append(p, mvNop)
		s = s.step(mvNop)
	}
	p = append(p, mvRotr)
	return append(p, t.path[resetIndex(s.prev, s.pos)][b]...)
}

func (e *encoder) dfsLag1(s lag1, b byte, i, depth int) bool {
	if i == depth {
		return byte(s.a) == b
	}
	for _, mv := range e.moveOrder() {
		e.buf[i] = mv
		if e.dfsLag1(s.step(mv), b, i+1, depth) {
			return true
		}
	}
	return false
}

// lag1Key packs a lag-1 state (with pos already reduced mod 94) densely.
func lag1Key(s lag1) int { return int(s.a)*nResets + resetIndex(s.prev, s.pos) }

// bitset is a set of small integers that can be cleared cheaply.
type bitset struct {
	words   []uint64
	touched []int
}

func newBitset(n int) *bitset { return &bitset{words: make([]uint64, (n+63)/64)} }

// add inserts k and reports whether it was absent.
func (b *bitset) add(k int) bool {
	w, bit := k/64, uint64(1)<<(k%64)
	if b.words[w]&bit != 0 {
		return false
	}
	if b.words[w] == 0 {
		b.touched = append(b.touched, w)
	}
	b.words[w] |= bit
	return true
}

func (b *bitset) clear() {
	for _, w := range b.touched {
		b.words[w] = 0
	}
	b.touched = b.touched[:0]
}
