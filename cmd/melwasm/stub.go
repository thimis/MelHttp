//go:build !(js && wasm)

// Command melwasm is only useful compiled to WebAssembly:
//
//	go generate ./internal/webvm
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "melwasm runs in browsers; build it with: go generate ./internal/webvm")
	os.Exit(2)
}
