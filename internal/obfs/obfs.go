// Package obfs implements the "program" transport encoding: a response body
// sent as Malbolge programs that the client runs to recover the body. It is
// obfuscation, not encryption: anyone can run the programs.
//
// Format: one or more Malbolge programs separated by a blank line. Program
// text only contains printable ASCII and single line breaks, so a blank line
// cannot occur inside a program.
package obfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/thimis/MelHttp/internal/gen"
	"github.com/thimis/MelHttp/internal/malbolge"
)

// Header names used on the wire.
const (
	AcceptHeader      = "X-Malbolge-Accept"       // request: "program"
	EncodingHeader    = "X-Malbolge-Encoding"     // response: "program"
	ContentTypeHeader = "X-Malbolge-Content-Type" // response: the decoded body's type
	Program           = "program"
	// MediaType is the Content-Type of an encoded body.
	MediaType = "text/x-malbolge; charset=us-ascii"
)

// Encode compiles body into the program container. A non-zero seed makes
// the programs vary while printing the same bytes.
func Encode(body []byte, seed uint64) ([]byte, error) {
	chunks, err := gen.Compile(body, gen.Options{Seed: seed})
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	for i, c := range chunks {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.Write(bytes.TrimRight(c, "\r\n"))
		b.WriteByte('\n')
	}
	return b.Bytes(), nil
}

// ErrTooExpensive means decoding exceeded its step budget.
var ErrTooExpensive = errors.New("obfs: program exceeded its step budget")

// Decode runs every program in the container and returns the concatenated
// output. Programs may not read input; maxSteps bounds the total work
// (0 means 64 steps per container byte, ample for generated code).
func Decode(ctx context.Context, container []byte, maxSteps int64) ([]byte, error) {
	if maxSteps <= 0 {
		maxSteps = 64*int64(len(container)) + malbolge.MemSize
	}
	var out bytes.Buffer
	n := 0
	for _, prog := range split(container) {
		p, err := malbolge.Load(prog)
		if err != nil {
			return nil, fmt.Errorf("obfs: program %d: %w", n, err)
		}
		res, err := p.Run(ctx, nil, &out, malbolge.Limits{MaxSteps: maxSteps})
		if errors.Is(err, malbolge.ErrStepLimit) {
			return nil, ErrTooExpensive
		}
		if err != nil {
			return nil, fmt.Errorf("obfs: program %d: %w", n, err)
		}
		if !res.Halted {
			return nil, fmt.Errorf("obfs: program %d did not halt", n)
		}
		maxSteps -= res.Steps
		n++
	}
	if n == 0 {
		return nil, errors.New("obfs: no programs")
	}
	return out.Bytes(), nil
}

// split cuts the container at blank lines (LF or CRLF).
func split(c []byte) [][]byte {
	c = bytes.ReplaceAll(c, []byte("\r\n"), []byte("\n"))
	var progs [][]byte
	for _, p := range bytes.Split(c, []byte("\n\n")) {
		if len(bytes.TrimSpace(p)) > 0 {
			progs = append(progs, p)
		}
	}
	return progs
}
