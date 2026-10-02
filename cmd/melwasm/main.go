//go:build js && wasm

// Command melwasm is the MelHttp Malbolge VM, transport decoder and
// generator compiled to WebAssembly for browsers (service worker and
// playground). Build it with: go generate ./internal/webvm
//
// It defines these JavaScript globals:
//
//	melhttpDecode(Uint8Array)              → {data: Uint8Array} | {error}
//	melhttpRun(source, Uint8Array, steps)  → {output: Uint8Array, steps, halted, readInput} | {error}
//	melhttpCompile(Uint8Array, seed)       → {programs: string, count, cells} | {error}
//	melhttpVersion                         → string
package main

import (
	"bytes"
	"context"
	"syscall/js"

	"github.com/thimis/MelHttp/internal/gen"
	"github.com/thimis/MelHttp/internal/malbolge"
	"github.com/thimis/MelHttp/internal/obfs"
)

var version = "dev"

func main() {
	g := js.Global()
	g.Set("melhttpDecode", js.FuncOf(decode))
	g.Set("melhttpRun", js.FuncOf(run))
	g.Set("melhttpCompile", js.FuncOf(compile))
	g.Set("melhttpVersion", version)
	select {} // keep the functions alive
}

func bytesArg(v js.Value) []byte {
	b := make([]byte, v.Get("length").Int())
	js.CopyBytesToGo(b, v)
	return b
}

func uint8Array(b []byte) js.Value {
	a := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(a, b)
	return a
}

func fail(err error) any { return map[string]any{"error": err.Error()} }

func decode(_ js.Value, args []js.Value) any {
	out, err := obfs.Decode(context.Background(), bytesArg(args[0]), 0)
	if err != nil {
		return fail(err)
	}
	return map[string]any{"data": uint8Array(out)}
}

func run(_ js.Value, args []js.Value) any {
	p, err := malbolge.Load([]byte(args[0].String()))
	if err != nil {
		return fail(err)
	}
	var input []byte
	if len(args) > 1 && !args[1].IsUndefined() && !args[1].IsNull() {
		input = bytesArg(args[1])
	}
	steps := int64(100_000_000)
	if len(args) > 2 && args[2].Type() == js.TypeNumber {
		steps = int64(args[2].Float())
	}
	var out bytes.Buffer
	var in *bytes.Reader
	if input != nil {
		in = bytes.NewReader(input)
	}
	var res malbolge.Result
	if in != nil {
		res, err = p.Run(context.Background(), in, &out, malbolge.Limits{MaxSteps: steps})
	} else {
		res, err = p.Run(context.Background(), nil, &out, malbolge.Limits{MaxSteps: steps})
	}
	r := map[string]any{
		"output": uint8Array(out.Bytes()), "steps": float64(res.Steps),
		"halted": res.Halted, "readInput": res.ReadInput,
	}
	if err != nil {
		r["error"] = err.Error()
	}
	return r
}

func compile(_ js.Value, args []js.Value) any {
	seed := uint64(0)
	if len(args) > 1 && args[1].Type() == js.TypeNumber {
		seed = uint64(args[1].Float())
	}
	data := bytesArg(args[0])
	chunks, err := gen.Compile(data, gen.Options{Seed: seed, Workers: 1})
	if err != nil {
		return fail(err)
	}
	if err := gen.Verify(context.Background(), chunks, data); err != nil {
		return fail(err)
	}
	cells := 0
	var b bytes.Buffer
	for i, c := range chunks {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.Write(bytes.TrimRight(c, "\n"))
		b.WriteByte('\n')
		cells += len(bytes.Join(bytes.Fields(c), nil))
	}
	return map[string]any{"programs": b.String(), "count": len(chunks), "cells": cells}
}
