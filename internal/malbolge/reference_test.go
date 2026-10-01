//go:build reference

package malbolge

import (
	"bytes"
	"context"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/thimis/MelHttp/internal/reftest"
)

// TestReferenceTables parses xlat1/xlat2 out of the original C source, an
// independent check on the strings typed into tables.go.
func TestReferenceTables(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(reftest.Dir(), "malbolge.c"))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"xlat1": Xlat1, "xlat2": Xlat2} {
		re := regexp.MustCompile(`(?s)const char ` + name + `\[\] =\s*"((?:[^"\\]|\\.)*)"\s*"((?:[^"\\]|\\.)*)";`)
		m := re.FindSubmatch(src)
		if m == nil {
			t.Fatalf("%s not found in malbolge.c", name)
		}
		got := strings.NewReplacer(`\\`, `\`, `\"`, `"`).Replace(string(m[1]) + string(m[2]))
		if got != want {
			t.Errorf("%s from C source:\n %q\nGo:\n %q", name, got, want)
		}
	}
}

func status(res Result, err error) string {
	switch {
	case err == nil && res.Halted:
		return "HALT"
	case errors.Is(err, ErrInvalidInstruction):
		return "INVALID"
	case errors.Is(err, ErrStepLimit):
		return "LIMIT"
	}
	return "ERROR: " + err.Error()
}

// randomProgram returns a valid program of n cells with ops drawn from weights.
func randomProgram(r *rand.Rand, n int) []byte {
	// Bias toward the interesting instructions; halt rarely so programs run on.
	weighted := []Op{OpNop, OpNop, OpOut, OpOut, OpOut, OpIn, OpRotr, OpRotr, OpCrz, OpCrz, OpCrz,
		OpMovD, OpMovD, OpJmp, OpJmp}
	b := make([]byte, n)
	for i := range b {
		op := weighted[r.IntN(len(weighted))]
		if r.IntN(150) == 0 {
			op = OpHalt
		}
		b[i] = Encode(op, i)
	}
	return b
}

// TestReferenceDifferential compares the VM to the reference interpreter on
// golden programs and 1000 random valid programs with random inputs.
func TestReferenceDifferential(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	var cases []reftest.Case
	golden, _ := filepath.Glob(filepath.Join(reftest.Dir(), "..", "programs", "*.mb"))
	for _, g := range golden {
		src, err := os.ReadFile(g)
		if err != nil {
			t.Fatal(err)
		}
		in, err := os.ReadFile(strings.TrimSuffix(g, ".mb") + ".in")
		if err != nil {
			in = []byte("Malbolge!\n")
		}
		cases = append(cases, reftest.Case{Src: src, Input: in, MaxSteps: 200_000_000})
	}
	for i := range 1000 {
		n := 2 + r.IntN(400)
		if i%10 == 0 {
			n = 2 + r.IntN(20000)
		}
		var in []byte
		if r.IntN(3) > 0 {
			in = make([]byte, r.IntN(64))
			for j := range in {
				in[j] = byte(r.IntN(256))
			}
		}
		cases = append(cases, reftest.Case{Src: randomProgram(r, n), Input: in, MaxSteps: int64(1000 + r.IntN(200000))})
	}
	outs := reftest.Run(t, cases)
	counts := map[string]int{}
	for i, c := range cases {
		p, err := Load(c.Src)
		if err != nil {
			t.Fatalf("case %d: load: %v", i, err)
		}
		lim := Limits{MaxSteps: c.MaxSteps}
		if lim.MaxSteps == 0 {
			lim.MaxSteps = 1_000_000
		}
		out, res, err := p.RunBytes(context.Background(), c.Input, lim)
		got := status(res, err)
		ref := outs[i]
		counts[ref.Status]++
		if got != ref.Status || res.Steps != ref.Steps || !bytes.Equal(out, ref.Output) {
			t.Errorf("case %d (%d cells): Go %s after %d steps, %d bytes; reference %s after %d steps, %d bytes %s",
				i, p.Len(), got, res.Steps, len(out), ref.Status, ref.Steps, len(ref.Output), ref.Detail)
			if t.Failed() && i > 20 {
				t.FailNow()
			}
		}
	}
	t.Logf("reference outcomes: %v", counts)
	if counts["HALT"] == 0 || counts["INVALID"] == 0 || counts["LIMIT"] == 0 {
		t.Errorf("random programs should exercise every outcome, got %v", counts)
	}
}
