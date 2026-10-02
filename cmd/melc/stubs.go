package main

import (
	"context"
	"fmt"
	"io"
)

// Temporary until P7 (build).
func cmdBuild(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "melc build: not implemented yet")
	return exitError
}
