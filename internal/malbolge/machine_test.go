package malbolge

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

// TestMachineMatchesRun steps the golden programs and checks the machine
// produces exactly what the fast Run loop produces.
func TestMachineMatchesRun(t *testing.T) {
	for _, name := range []string{"hello.mb", "cat-terminating.mb", "adder.mb"} {
		p := loadFile(t, name)
		input := []byte("12 30")
		want, res, err := p.RunBytes(context.Background(), input, Limits{MaxSteps: 5_000_000})
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		m := p.NewMachine(bytes.NewReader(input), &out)
		for !m.Halted {
			if err := m.Step(); err != nil {
				t.Fatalf("%s: step %d: %v", name, m.Steps, err)
			}
		}
		if !bytes.Equal(out.Bytes(), want) || m.Steps != res.Steps {
			t.Fatalf("%s: machine printed %q in %d steps; Run printed %q in %d", name, out.Bytes(), m.Steps, want, res.Steps)
		}
		if err := m.Step(); err == nil {
			t.Error("Step after halt should fail")
		}
	}
}

func TestMachineInvalid(t *testing.T) {
	m := mustLoad(t, prog("oo")).NewMachine(nil, nil)
	m.Step()
	m.Step()
	if m.Op() != 0 {
		t.Fatalf("Op at invalid cell = %v", m.Op())
	}
	if err := m.Step(); !errors.Is(err, ErrInvalidInstruction) {
		t.Fatalf("err = %v", err)
	}
}
