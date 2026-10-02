package gen

import (
	"context"
	"strings"
	"testing"

	"github.com/thimis/MelHttp/internal/malbolge"
)

// TestLag1Completeness: lag-1 chunks can print exactly the 201 lag-1-printable
// bytes (all of ASCII among them) from every state, and the tables agree with
// the real VM.
func TestLag1Completeness(t *testing.T) {
	tb := getLag1()
	n := 0
	for b := range 256 {
		if tb.printable[b] {
			n++
		}
	}
	if n != 201 {
		t.Fatalf("%d printable bytes, want 201", n)
	}
	for b := range 128 {
		if !tb.printable[b] {
			t.Fatalf("ASCII byte %#x not lag-1 printable", b)
		}
	}
	worst := 0
	for r := range nResets {
		for b := range 256 {
			if tb.printable[b] {
				if tb.fbLen[r][b] == unreachable {
					t.Fatalf("reset %d cannot reach printable byte %#x", r, b)
				}
				worst = max(worst, int(tb.fbLen[r][b]))
			}
		}
	}
	t.Logf("lag-1: worst-case %d cells for one byte from any state", worst)
	// Model vs VM: print every printable byte after a run of setup bytes.
	e := &encoder{opt: Options{}}
	e.opt, _ = e.opt.withDefaults()
	var data []byte
	for b := range 256 {
		if tb.printable[b] {
			data = append(data, byte(b), 'x', byte(b))
		}
	}
	n, src := e.lag1Chunk(data)
	if n != len(data) {
		t.Fatalf("lag-1 chunk took %d of %d bytes", n, len(data))
	}
	p, err := malbolge.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := p.RunBytes(context.Background(), nil, malbolge.Limits{})
	if err != nil || string(out) != string(data) {
		t.Fatalf("lag-1 program printed %q, err %v", out, err)
	}
}

// TestTapeOnRealVM checks the tape layout against the real machine for
// several tape lengths: after the 'j', d == sStar, a matches the model, and
// the cells content will read hold exactly the stream values.
func TestTapeOnRealVM(t *testing.T) {
	tl, err := getTape()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("tape start cell %d, worst padding %d, stream %d values", tl.sStar, maxPad(tl), len(tl.stream))
	e := &encoder{tape: tl}
	e.opt, _ = Options{}.withDefaults()
	never := func(int) bool { return false }
	for _, T := range []int{64, 65, 200, 1000, 5001, tapeMax} {
		_, _, src := e.tapeChunk(tl, nil, T, never)
		p, err := malbolge.Load(src)
		if err != nil {
			t.Fatal(err)
		}
		m := p.NewMachine(nil, nil)
		p0 := prefixLen + T + tl.pad[(prefixLen+T)%94] + 2
		for m.C != p0 {
			if err := m.Step(); err != nil || m.Halted {
				t.Fatalf("T=%d: stopped at c=%d before content: %v", T, m.C, err)
			}
		}
		if m.D != tl.sStar {
			t.Fatalf("T=%d: d=%d after the jump, want %d", T, m.D, tl.sStar)
		}
		limit := prefixLen + T - 1 - tl.sStar
		for i := range limit {
			if m.Mem[tl.sStar+i] != tl.stream[i] {
				t.Fatalf("T=%d: cell %d = %d, stream says %d", T, tl.sStar+i, m.Mem[tl.sStar+i], tl.stream[i])
			}
		}
	}
}

func maxPad(tl *tapeLayout) int {
	w := 0
	for _, p := range tl.pad {
		w = max(w, p)
	}
	return w
}

// TestTapeCompleteness is the converter's completeness proof for arbitrary
// bytes. From any state the encoder can emit nops up to a reset ('*') whose
// short paths print the wanted byte. For every stream index content can read
// and every byte value, this finds that reset and bounds the cost, so every
// byte is printable from every state and every input is compilable.
func TestTapeCompleteness(t *testing.T) {
	tl, err := getTape()
	if err != nil {
		t.Fatal(err)
	}
	r := tl.reach
	const margin = 1024 // the last indices may run out of stream; chunks end there
	worst, worstK, worstB, total, count := 0, 0, 0, 0, 0
	for k := 0; k < len(r.any)-margin; k++ {
		for b := range 256 {
			kk := r.next(k, byte(b))
			if kk < 0 {
				t.Fatalf("byte %#x unreachable after stream index %d", b, k)
			}
			cost := (kk - k) + 1 + r.depth(kk, byte(b)) + 1
			total += cost
			count++
			if cost > worst {
				worst, worstK, worstB = cost, k, b
			}
		}
	}
	t.Logf("tape: worst %d cells for one byte (index %d, byte %#x); average fallback %.2f cells",
		worst, worstK, worstB, float64(total)/float64(count))
	if worst > 600 {
		t.Errorf("worst case %d cells is too high", worst)
	}
}

func TestASCIIUsesLag1Chunks(t *testing.T) {
	text := []byte(strings.Repeat("function malbolge(x){return x*3+1;} // ascii only\n", 400))
	chunks := roundTrip(t, text, Options{})
	ratio := float64(cells(chunks)) / float64(len(text))
	t.Logf("ASCII: %.2f cells/byte, %d chunks", ratio, len(chunks))
	if ratio > 10 {
		t.Errorf("ASCII costs %.2f cells/byte; lag-1 chunks should give < 10", ratio)
	}
}
