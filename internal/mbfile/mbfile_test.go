package mbfile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thimis/MelHttp/internal/malbolge"
)

// nopProgram returns a valid program that prints nothing and halts.
func haltProgram() string {
	return string([]byte{malbolge.Encode(malbolge.OpNop, 0), malbolge.Encode(malbolge.OpHalt, 1)})
}

func readHello(t *testing.T) []byte {
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "programs", "hello.mb"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLoadSingleFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hello.mb")
	os.WriteFile(p, readHello(t), 0o644)
	set, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Programs) != 1 || set.Chunked {
		t.Fatalf("got %d programs chunked=%v", len(set.Programs), set.Chunked)
	}
	out, _, err := set.RunBytes(context.Background(), nil, malbolge.Limits{})
	if err != nil || string(out) != "Hello, world." {
		t.Fatalf("%q %v", out, err)
	}
}

func TestLoadChunkDirInOrder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "page.html.mb")
	os.MkdirAll(dir, 0o755)
	hello := readHello(t)
	// Written out of order; must run sorted by name.
	os.WriteFile(filepath.Join(dir, "002.mb"), hello, 0o644)
	os.WriteFile(filepath.Join(dir, "000.mb"), hello, 0o644)
	os.WriteFile(filepath.Join(dir, "001.mb"), []byte(haltProgram()), 0o644)
	os.WriteFile(filepath.Join(dir, "README.txt"), []byte("ignored"), 0o644)
	set, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !set.Chunked || len(set.Programs) != 3 {
		t.Fatalf("chunked=%v n=%d", set.Chunked, len(set.Programs))
	}
	out, res, err := set.RunBytes(context.Background(), nil, malbolge.Limits{})
	if err != nil || string(out) != "Hello, world.Hello, world." {
		t.Fatalf("%q %v", out, err)
	}
	if res.ReadInput || !res.Halted || res.Steps == 0 {
		t.Fatalf("res = %+v", res)
	}
	if len(set.Files) != 3 || !strings.HasSuffix(set.Files[0], "000.mb") {
		t.Fatalf("Files = %v", set.Files)
	}
}

func TestLimitsSpanAllChunks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "x.mb")
	os.MkdirAll(dir, 0o755)
	for _, n := range []string{"000.mb", "001.mb"} {
		os.WriteFile(filepath.Join(dir, n), readHello(t), 0o644)
	}
	set, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = set.RunBytes(context.Background(), nil, malbolge.Limits{MaxOutput: 20})
	if err != malbolge.ErrOutputLimit {
		t.Fatalf("err = %v, want output limit across chunks", err)
	}
	// An exactly-spent output budget followed by a silent chunk is fine.
	os.WriteFile(filepath.Join(dir, "002.mb"), []byte(haltProgram()), 0o644)
	set, _ = Load(dir)
	out, _, err := set.RunBytes(context.Background(), nil, malbolge.Limits{MaxOutput: 26})
	if err != nil || len(out) != 26 {
		t.Fatalf("exact budget: %q %v", out, err)
	}
	_, one, _ := set.RunBytes(context.Background(), nil, malbolge.Limits{})
	_, _, err = set.RunBytes(context.Background(), nil, malbolge.Limits{MaxSteps: one.Steps - 1})
	if err != malbolge.ErrStepLimit {
		t.Fatalf("err = %v, want step limit across chunks", err)
	}
}

func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(filepath.Join(dir, "missing.mb")); err == nil {
		t.Error("missing file accepted")
	}
	empty := filepath.Join(dir, "empty.mb")
	os.MkdirAll(empty, 0o755)
	if _, err := Load(empty); err == nil {
		t.Error("empty chunk dir accepted")
	}
	bad := filepath.Join(dir, "bad.mb")
	os.WriteFile(bad, []byte("not malbolge"), 0o644)
	_, err := Load(bad)
	if err == nil || !strings.Contains(err.Error(), "bad.mb") {
		t.Errorf("error should name the file: %v", err)
	}
	if _, err := Parse([]byte(haltProgram())); err != nil {
		t.Error(err)
	}
}
