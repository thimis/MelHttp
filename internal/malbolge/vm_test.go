package malbolge

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustLoad(t testing.TB, src string) *Program {
	t.Helper()
	p, err := Load([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func loadFile(t testing.TB, name string) *Program {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", "programs", name))
	if err != nil {
		t.Fatal(err)
	}
	return mustLoad(t, string(src))
}

// prog assembles normalized ops (one char per op, e.g. "o<v") into source.
func prog(ops string) string {
	b := make([]byte, len(ops))
	for i := range len(ops) {
		b[i] = Encode(Op(ops[i]), i)
	}
	return string(b)
}

func TestRunHelloWorld(t *testing.T) {
	out, res, err := mustLoad(t, helloSrc).RunBytes(context.Background(), nil, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "Hello, world." {
		t.Fatalf("output %q", out)
	}
	if res.ReadInput {
		t.Error("hello world reported reading input")
	}
	if res.Steps == 0 {
		t.Error("Steps not counted")
	}
}

// goldenPrograms lists every program in testdata/programs with what it needs:
// its expected output (.out) and, if it reads input, the input (.in). The
// test fails if any listed file is missing or an unlisted program appears.
var goldenPrograms = map[string]struct{ out, in bool }{
	"hello.mb":           {out: true},
	"cat-terminating.mb": {out: true, in: true},
	"adder.mb":           {out: true, in: true},
	"digital_root.mb":    {out: true, in: true},
	"quine.mb":           {out: true},
	"cat.mb":             {}, // never halts; TestRunCatEchoesThenEOFValue covers it
}

func TestRunGoldenFiles(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "programs")
	files, _ := filepath.Glob(filepath.Join(dir, "*.mb"))
	if len(files) != len(goldenPrograms) {
		t.Errorf("testdata/programs has %d programs, the test expects %d", len(files), len(goldenPrograms))
	}
	for _, f := range files {
		if _, ok := goldenPrograms[filepath.Base(f)]; !ok {
			t.Errorf("%s is not listed in goldenPrograms", filepath.Base(f))
		}
	}
	for name, need := range goldenPrograms {
		base := filepath.Join(dir, strings.TrimSuffix(name, ".mb"))
		if _, err := os.Stat(base + ".mb"); err != nil {
			t.Errorf("missing %s", name)
			continue
		}
		if !need.out {
			continue
		}
		want, err := os.ReadFile(base + ".out")
		if err != nil {
			t.Errorf("missing expected output for %s: %v", name, err)
			continue
		}
		var input []byte
		if need.in {
			if input, err = os.ReadFile(base + ".in"); err != nil {
				t.Errorf("missing input for %s: %v", name, err)
				continue
			}
		}
		t.Run(name, func(t *testing.T) {
			out, _, err := loadFile(t, name).RunBytes(context.Background(), input, Limits{MaxSteps: 1_000_000_000})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out, want) {
				t.Fatalf("output differs:\n got %q\nwant %q", out, want)
			}
		})
	}
}

func TestRunCatEchoesThenEOFValue(t *testing.T) {
	out, res, err := loadFile(t, "cat.mb").RunBytes(context.Background(), []byte("Hi!\n"), Limits{MaxSteps: 100_000})
	if !errors.Is(err, ErrStepLimit) {
		t.Fatalf("err = %v, want ErrStepLimit (this cat never halts)", err)
	}
	if !bytes.HasPrefix(out, []byte("Hi!\n\xa8\xa8\xa8")) {
		t.Fatalf("output starts %q", out[:min(len(out), 12)])
	}
	if !res.ReadInput {
		t.Error("ReadInput = false for cat")
	}
	if res.Steps != 100_000 {
		t.Errorf("Steps = %d, want exactly the limit", res.Steps)
	}
}

func TestRunNilInputIsEOF(t *testing.T) {
	out, _, err := loadFile(t, "cat.mb").RunBytes(context.Background(), nil, Limits{MaxSteps: 5000})
	if !errors.Is(err, ErrStepLimit) {
		t.Fatal(err)
	}
	if len(out) == 0 || out[0] != 0xa8 {
		t.Fatalf("first byte on EOF = %q, want 0xA8 (59048 %% 256)", out[:min(len(out), 4)])
	}
}

func TestRunOutputLimit(t *testing.T) {
	_, _, err := mustLoad(t, helloSrc).RunBytes(context.Background(), nil, Limits{MaxOutput: 5})
	if !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("err = %v, want ErrOutputLimit", err)
	}
	out, _, err := mustLoad(t, helloSrc).RunBytes(context.Background(), nil, Limits{MaxOutput: 13})
	if err != nil || string(out) != "Hello, world." {
		t.Fatalf("exact output limit: %q %v", out, err)
	}
}

func TestRunContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err := loadFile(t, "cat.mb").RunBytes(ctx, nil, Limits{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("cancellation took too long")
	}
}

func TestRunHalt(t *testing.T) {
	out, res, err := mustLoad(t, prog("ov")).RunBytes(context.Background(), nil, Limits{})
	if err != nil || len(out) != 0 || res.Steps != 2 || !res.Halted {
		t.Fatalf("out=%q res=%+v err=%v", out, res, err)
	}
}

func TestRunInvalidInstructionHalts(t *testing.T) {
	// prog("oo") is "DC": after two nops c = 2, and the fill rule gives
	// mem[2] = crz('C'=67, 'D'=68) = 1111111002₃ = 29513, outside 33..126.
	// The reference interpreter would spin forever; we stop.
	p := mustLoad(t, prog("oo"))
	if p.mem[2] != 29513 {
		t.Fatalf("mem[2] = %d, want 29513", p.mem[2])
	}
	_, res, err := p.RunBytes(context.Background(), nil, Limits{MaxSteps: 1_000_000})
	if !errors.Is(err, ErrInvalidInstruction) || res.Steps != 2 {
		t.Fatalf("err = %v steps = %d, want ErrInvalidInstruction after 2 steps", err, res.Steps)
	}
}

func TestRunDoesNotMutateProgram(t *testing.T) {
	p := mustLoad(t, helloSrc)
	before := p.mem
	for range 3 {
		out, _, err := p.RunBytes(context.Background(), nil, Limits{})
		if err != nil || string(out) != "Hello, world." {
			t.Fatalf("run: %q %v", out, err)
		}
	}
	if p.mem != before {
		t.Fatal("Run modified the shared program image")
	}
}

func TestRunConcurrent(t *testing.T) {
	p := mustLoad(t, helloSrc)
	errs := make(chan error, 32)
	for range 32 {
		go func() {
			out, _, err := p.RunBytes(context.Background(), nil, Limits{})
			if err == nil && string(out) != "Hello, world." {
				err = errors.New("bad output " + string(out))
			}
			errs <- err
		}()
	}
	for range 32 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunWriterError(t *testing.T) {
	_, err := mustLoad(t, helloSrc).Run(context.Background(), nil, failWriter{}, Limits{})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want writer error", err)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }

func BenchmarkRunCat(b *testing.B) {
	p := loadFile(b, "cat.mb")
	const steps = 1_000_000
	b.SetBytes(steps) // reported MB/s == million instructions per second
	for b.Loop() {
		p.RunBytes(context.Background(), nil, Limits{MaxSteps: steps})
	}
}

func BenchmarkRunHello(b *testing.B) {
	p := mustLoad(b, helloSrc)
	for b.Loop() {
		p.RunBytes(context.Background(), nil, Limits{})
	}
}
