package main

import (
	"context"
	"fmt"
	"io"
)

// Temporary until P4 (gen) and P7 (build).
func cmdGen(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "melc gen: not implemented yet")
	return exitError
}

func cmdBuild(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "melc build: not implemented yet")
	return exitError
}
