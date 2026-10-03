package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/thimis/MelHttp/internal/malbolge"
	"github.com/thimis/MelHttp/internal/mbfile"
)

func cmdRun(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("melc run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	steps := fs.Int64("steps", 0, "stop after `N` instructions (0 = unlimited)")
	stats := fs.Bool("stats", false, "print step count and status to stderr")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: melc run [-steps N] [-stats] <program.mb>")
		return exitUsage
	}
	set, err := mbfile.Load(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, "melc:", err)
		return exitError
	}
	out := bufio.NewWriterSize(stdout, 64<<10)
	res, err := set.Run(ctx, stdin, out, malbolge.Limits{MaxSteps: *steps})
	if ferr := out.Flush(); ferr != nil && err == nil {
		err = ferr
	}
	if *stats {
		fmt.Fprintf(stderr, "steps=%d output=%d read_input=%v halted=%v\n", res.Steps, res.Output, res.ReadInput, res.Halted)
	}
	switch {
	case err == nil:
		return exitOK
	case errors.Is(err, malbolge.ErrStepLimit):
		fmt.Fprintln(stderr, "melc:", err)
		return exitSteps
	case errors.Is(err, malbolge.ErrInvalidInstruction):
		fmt.Fprintln(stderr, "melc:", err)
		return exitInvalid
	}
	fmt.Fprintln(stderr, "melc:", err)
	return exitError
}

func cmdCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("melc check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "usage: melc check <program.mb>...")
		return exitUsage
	}
	code := exitOK
	for _, p := range fs.Args() {
		set, err := mbfile.Load(p)
		if err != nil {
			fmt.Fprintln(stderr, "melc:", err)
			code = exitError
			continue
		}
		cells := 0
		for _, prog := range set.Programs {
			cells += prog.Len()
		}
		fmt.Fprintf(stdout, "%s: ok (%d program(s), %d cells)\n", p, len(set.Programs), cells)
	}
	return code
}
