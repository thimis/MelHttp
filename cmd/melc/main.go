// Command melc is the MelHttp Malbolge toolchain: it runs and checks Malbolge
// programs and compiles files and whole sites into Malbolge.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	melversion "github.com/thimis/MelHttp/internal/version"
)

// version is set by release builds with -ldflags "-X main.version=...".
var version = melversion.Dev

const usage = `melc — the MelHttp Malbolge toolchain

Usage:
  melc run   [-steps N] [-stats] <program.mb | chunk-dir.mb>   run a program (stdin → program → stdout)
  melc check <program.mb | chunk-dir.mb>...                     validate programs
  melc gen   [-o out.mb] [-raw] [-type mime] <file>              compile one file into Malbolge
  melc build [-o out-dir] [--preset name] [--run-build] <dir>    compile a whole site into Malbolge
  melc watch [-o out-dir] [-serve :8080] <dir>                   rebuild on every change (and serve)
  melc version

Run "melc <command> -h" for command flags.
`

// Exit codes.
const (
	exitOK      = 0
	exitError   = 1
	exitUsage   = 2
	exitSteps   = 3 // step limit reached
	exitInvalid = 4 // execution reached an invalid cell
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "run":
		return cmdRun(ctx, rest, stdin, stdout, stderr)
	case "check":
		return cmdCheck(rest, stdout, stderr)
	case "gen":
		return cmdGen(ctx, rest, stdout, stderr)
	case "build":
		return cmdBuild(ctx, rest, stdout, stderr)
	case "watch":
		return cmdWatch(ctx, rest, stdout, stderr)
	case "version", "-version", "--version":
		fmt.Fprintln(stdout, "melc", version)
		return exitOK
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return exitOK
	}
	fmt.Fprintf(stderr, "melc: unknown command %q\n\n%s", cmd, usage)
	return exitUsage
}
