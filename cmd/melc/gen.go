package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/thimis/MelHttp/internal/gen"
	"github.com/thimis/MelHttp/internal/mbfile"
	"github.com/thimis/MelHttp/internal/melcgi"
	"github.com/thimis/MelHttp/internal/mimetype"
)

func cmdGen(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("melc gen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "", "output `path` (default <file>.mb; \"-\" writes a single program to stdout)")
	raw := fs.Bool("raw", false, "print the file only, without a MelCGI header block")
	ctype := fs.String("type", "", "Content-Type for the header block (default: from the file extension)")
	seed := fs.Uint64("seed", 0, "randomize the generated code (0 = deterministic)")
	stats := fs.Bool("stats", false, "print size statistics to stderr")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: melc gen [-o out.mb] [-raw] [-type mime] [-seed N] [-stats] <file>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitUsage
	}
	in := fs.Arg(0)
	data, err := os.ReadFile(in)
	if err != nil {
		fmt.Fprintln(stderr, "melc:", err)
		return exitError
	}
	if !*raw {
		ct := *ctype
		if ct == "" {
			ct = mimetype.ByName(filepath.Base(in))
		}
		data = append(melcgi.HeaderBlock(ct), data...)
	}
	chunks, err := compileVerified(ctx, data, gen.Options{Seed: *seed})
	if err != nil {
		fmt.Fprintln(stderr, "melc:", err)
		return exitError
	}
	dst := *out
	if dst == "" {
		dst = in + ".mb"
	}
	if dst == "-" {
		if len(chunks) != 1 {
			fmt.Fprintf(stderr, "melc: %s needs %d programs; use -o <dir> instead of stdout\n", in, len(chunks))
			return exitError
		}
		if _, err := stdout.Write(chunks[0]); err != nil {
			fmt.Fprintln(stderr, "melc:", err)
			return exitError
		}
	} else if err := mbfile.Write(dst, chunks); err != nil {
		fmt.Fprintln(stderr, "melc:", err)
		return exitError
	}
	if *stats {
		printStats(stderr, in, len(data), chunks)
	}
	return exitOK
}

// compileVerified compiles data and checks, by running every program, that
// the output is exactly data. Nothing unverified is ever written.
func compileVerified(ctx context.Context, data []byte, opt gen.Options) ([][]byte, error) {
	chunks, err := gen.Compile(data, opt)
	if err != nil {
		return nil, err
	}
	if err := gen.Verify(ctx, chunks, data); err != nil {
		return nil, fmt.Errorf("self-check failed (please report this bug): %w", err)
	}
	return chunks, nil
}

func printStats(w io.Writer, name string, n int, chunks [][]byte) {
	size := 0
	for _, c := range chunks {
		size += len(c)
	}
	ratio := 0.0
	if n > 0 {
		ratio = float64(size) / float64(n)
	}
	fmt.Fprintf(w, "%s: %d bytes -> %d program(s), %d bytes of Malbolge (%.1fx)\n", name, n, len(chunks), size, ratio)
}
