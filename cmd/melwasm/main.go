//go:build js && wasm

// Command melwasm is the MelHttp Malbolge VM, transport decoder and
// generator compiled to WebAssembly for browsers (service worker and
// playground). Build it with: go generate ./internal/webvm
//
// It defines these JavaScript globals:
//
//	melhttpDecode(Uint8Array)              → {data: Uint8Array} | {error}
//	melhttpRun(source, Uint8Array, steps)  → {output: Uint8Array, steps, halted, readInput} (+ error)
//	melhttpCompile(Uint8Array, seed)       → {programs: string, count, cells} | {error}
//	melhttpVersion                         → string
//
// The logic lives in api.go; this file only converts JavaScript values.
package main

import (
	"syscall/js"
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
	out, err := decodeContainer(bytesArg(args[0]))
	if err != nil {
		return fail(err)
	}
	return map[string]any{"data": uint8Array(out)}
}

func run(_ js.Value, args []js.Value) any {
	var input []byte
	if len(args) > 1 && !args[1].IsUndefined() && !args[1].IsNull() {
		input = bytesArg(args[1])
	}
	steps := int64(100_000_000)
	if len(args) > 2 && args[2].Type() == js.TypeNumber {
		steps = int64(args[2].Float())
	}
	r, err := runSource(args[0].String(), input, steps)
	if err != nil {
		return fail(err)
	}
	res := map[string]any{
		"output": uint8Array(r.Output), "steps": float64(r.Steps), "halted": r.Halted, "readInput": r.ReadInput,
	}
	if r.Err != "" {
		res["error"] = r.Err
	}
	return res
}

func compile(_ js.Value, args []js.Value) any {
	seed := uint64(0)
	if len(args) > 1 && args[1].Type() == js.TypeNumber {
		seed = uint64(args[1].Float())
	}
	programs, count, cells, err := compileText(bytesArg(args[0]), seed)
	if err != nil {
		return fail(err)
	}
	return map[string]any{"programs": programs, "count": count, "cells": cells}
}
