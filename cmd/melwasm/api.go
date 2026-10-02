package main

import (
	"bytes"
	"context"

	"github.com/thimis/MelHttp/internal/gen"
	"github.com/thimis/MelHttp/internal/malbolge"
	"github.com/thimis/MelHttp/internal/obfs"
)

// The browser API in plain Go. main.go only converts these to and from
// JavaScript values, so everything here is tested natively (api_test.go).

type runResult struct {
	Output    []byte
	Steps     int64
	Halted    bool
	ReadInput bool
	Err       string // set when the run stopped early (limit, invalid cell, …)
}

// runSource runs Malbolge source with input (nil = EOF) and a step limit.
func runSource(src string, input []byte, maxSteps int64) (runResult, error) {
	p, err := malbolge.Load([]byte(src))
	if err != nil {
		return runResult{}, err
	}
	var out bytes.Buffer
	var res malbolge.Result
	if input != nil {
		res, err = p.Run(context.Background(), bytes.NewReader(input), &out, malbolge.Limits{MaxSteps: maxSteps})
	} else {
		res, err = p.Run(context.Background(), nil, &out, malbolge.Limits{MaxSteps: maxSteps})
	}
	r := runResult{Output: out.Bytes(), Steps: res.Steps, Halted: res.Halted, ReadInput: res.ReadInput}
	if err != nil {
		r.Err = err.Error()
	}
	return r, nil
}

// decodeContainer runs a transport container and returns what it prints.
func decodeContainer(container []byte) ([]byte, error) {
	return obfs.Decode(context.Background(), container, 0)
}

// compileText compiles data into verified Malbolge programs separated by
// blank lines, and reports how many programs and cells it produced.
func compileText(data []byte, seed uint64) (programs string, count, cells int, err error) {
	chunks, err := gen.Compile(data, gen.Options{Seed: seed, Workers: 1})
	if err != nil {
		return "", 0, 0, err
	}
	if err := gen.Verify(context.Background(), chunks, data); err != nil {
		return "", 0, 0, err
	}
	var b bytes.Buffer
	for i, c := range chunks {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.Write(bytes.TrimRight(c, "\n"))
		b.WriteByte('\n')
		cells += len(bytes.Join(bytes.Fields(c), nil))
	}
	return b.String(), len(chunks), cells, nil
}
