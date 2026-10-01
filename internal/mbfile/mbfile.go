// Package mbfile loads Malbolge programs from disk. A program is either a
// single ".mb" file or a chunk directory ("page.html.mb/000.mb, 001.mb, ...")
// whose programs run in name order with their outputs concatenated. Chunks
// exist because one Malbolge program can occupy at most 59049 cells.
package mbfile

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thimis/MelHttp/internal/malbolge"
)

// Set is one logical program: one or more chunks run in sequence.
type Set struct {
	Programs []*malbolge.Program
	Files    []string // source file of each program
	Chunked  bool     // loaded from a chunk directory
}

// Parse loads a single program from source bytes.
func Parse(src []byte) (*Set, error) {
	p, err := malbolge.Load(src)
	if err != nil {
		return nil, err
	}
	return &Set{Programs: []*malbolge.Program{p}, Files: []string{""}}, nil
}

// Load reads a ".mb" file or chunk directory.
func Load(path string) (*Set, error) {
	return LoadFS(os.DirFS(filepath.Dir(path)), filepath.Base(path))
}

// LoadFS reads a ".mb" file or chunk directory from fsys.
func LoadFS(fsys fs.FS, name string) (*Set, error) {
	fi, err := fs.Stat(fsys, name)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		p, err := loadOne(fsys, name)
		if err != nil {
			return nil, err
		}
		return &Set{Programs: []*malbolge.Program{p}, Files: []string{name}}, nil
	}
	chunks, err := Chunks(fsys, name)
	if err != nil {
		return nil, err
	}
	set := &Set{Chunked: true}
	for _, c := range chunks {
		p, err := loadOne(fsys, c)
		if err != nil {
			return nil, err
		}
		set.Programs = append(set.Programs, p)
		set.Files = append(set.Files, c)
	}
	return set, nil
}

// Chunks lists the chunk files (slash paths, sorted) of a chunk directory.
func Chunks(fsys fs.FS, dir string) ([]string, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var chunks []string
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".mb") {
			chunks = append(chunks, dir+"/"+e.Name())
		}
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("%s: chunk directory contains no .mb files", dir)
	}
	sort.Strings(chunks)
	return chunks, nil
}

func loadOne(fsys fs.FS, name string) (*malbolge.Program, error) {
	src, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	p, err := malbolge.Load(src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return p, nil
}

// Run executes every chunk in order, reading from in and writing to out.
// Limits apply to the whole set. The combined result halts only if every
// chunk halted.
func (s *Set) Run(ctx context.Context, in io.Reader, out io.Writer, lim malbolge.Limits) (malbolge.Result, error) {
	var total malbolge.Result
	for _, p := range s.Programs {
		l, w := lim, out
		if l.MaxSteps > 0 {
			if l.MaxSteps -= total.Steps; l.MaxSteps <= 0 {
				return total, malbolge.ErrStepLimit
			}
		}
		if l.MaxOutput > 0 {
			if l.MaxOutput -= total.Output; l.MaxOutput <= 0 {
				// The budget is spent: this chunk may still run, but must print nothing.
				l.MaxOutput, w = 0, fullWriter{}
			}
		}
		res, err := p.Run(ctx, in, w, l)
		total.Steps += res.Steps
		total.Output += res.Output
		total.ReadInput = total.ReadInput || res.ReadInput
		total.Halted = res.Halted
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// fullWriter rejects every write; used once the output budget is spent.
type fullWriter struct{}

func (fullWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return 0, malbolge.ErrOutputLimit
}

// RunBytes is Run with in-memory input and output.
func (s *Set) RunBytes(ctx context.Context, input []byte, lim malbolge.Limits) ([]byte, malbolge.Result, error) {
	var out bytes.Buffer
	var in io.Reader
	if input != nil {
		in = bytes.NewReader(input)
	}
	res, err := s.Run(ctx, in, &out, lim)
	return out.Bytes(), res, err
}
