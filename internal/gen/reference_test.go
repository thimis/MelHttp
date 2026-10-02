//go:build reference

package gen

import (
	"bytes"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/thimis/MelHttp/internal/reftest"
)

// TestReferenceRunsGeneratedPrograms runs generated chunks (both lag-1 and
// tape chunks) through Ben Olmstead's reference interpreter: our programs must
// not depend on anything specific to our VM.
func TestReferenceRunsGeneratedPrograms(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 5))
	random := make([]byte, 6000)
	for i := range random {
		random[i] = byte(rng.IntN(256))
	}
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	inputs := map[string][]byte{
		"ascii":  []byte(strings.Repeat("<p>Hello from Malbolge, reference edition.</p>\n", 200)),
		"utf8":   []byte(strings.Repeat("Ünïcødé — ✓ 🎉 ", 100)),
		"all256": all,
		"random": random,
		"empty":  nil,
	}
	type item struct {
		name   string
		chunks [][]byte
		want   []byte
	}
	var items []item
	var cases []reftest.Case
	for name, data := range inputs {
		for _, opt := range []Options{{}, {Seed: 99}} {
			chunks, err := Compile(data, opt)
			if err != nil {
				t.Fatal(err)
			}
			items = append(items, item{name, chunks, data})
			for _, c := range chunks {
				cases = append(cases, reftest.Case{Src: c, MaxSteps: 100_000})
			}
		}
	}
	outs := reftest.Run(t, cases)
	i := 0
	for _, it := range items {
		var got bytes.Buffer
		for range it.chunks {
			o := outs[i]
			i++
			if o.Status != "HALT" {
				t.Fatalf("%s: reference interpreter status %s %s", it.name, o.Status, o.Detail)
			}
			got.Write(o.Output)
		}
		if !bytes.Equal(got.Bytes(), it.want) {
			t.Errorf("%s: reference interpreter printed %d bytes, want %d", it.name, got.Len(), len(it.want))
		}
	}
	t.Logf("%d generated programs verified on the reference interpreter", len(cases))
}
