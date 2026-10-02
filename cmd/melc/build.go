package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/thimis/MelHttp/internal/build"
)

func cmdBuild(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("melc build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "site", "output site `directory` (replaced on rebuild)")
	preset := fs.String("preset", "auto", "framework `name`: "+strings.Join(build.PresetNames(), ", "))
	runBuild := fs.Bool("run-build", false, "run the project's own build (npm run build, hugo, …) first")
	spa := fs.String("spa", "", "single-page app fallback: true or false (default: from the preset)")
	seed := fs.Uint64("seed", 0, "randomize the generated code (0 = deterministic)")
	verbose := fs.Bool("v", false, "list every file")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: melc build [-o out-dir] [--preset name] [--run-build] [--spa=true|false] <project-or-site-dir>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitUsage
	}
	project := fs.Arg(0)
	p, err := build.Resolve(*preset, project)
	if err != nil {
		fmt.Fprintln(stderr, "melc:", err)
		return exitUsage
	}
	if *runBuild {
		if err := runProjectBuild(ctx, p, project, stderr); err != nil {
			fmt.Fprintln(stderr, "melc:", err)
			return exitError
		}
	}
	src, err := p.OutputDir(project)
	if err != nil {
		fmt.Fprintln(stderr, "melc:", err)
		return exitError
	}
	opt := build.Options{Src: src, Out: *out, Seed: *seed}
	switch *spa {
	case "":
		if p.Name != "static" {
			opt.SPA = &p.SPA
		}
	case "true", "false":
		v := *spa == "true"
		opt.SPA = &v
	default:
		fmt.Fprintln(stderr, "melc: --spa must be true or false")
		return exitUsage
	}
	if *verbose {
		opt.Log = stderr
	}
	fmt.Fprintf(stderr, "melc: building %s (preset %s) from %s into %s\n", project, p.Name, src, *out)
	st, err := build.Site(ctx, opt)
	if err != nil {
		fmt.Fprintln(stderr, "melc:", err)
		return exitError
	}
	fmt.Fprintln(stdout, "melc:", st)
	return exitOK
}

// runProjectBuild runs the framework's build command in the project.
func runProjectBuild(ctx context.Context, p build.Preset, project string, stderr io.Writer) error {
	if len(p.Command) == 0 {
		return nil
	}
	run := func(argv ...string) error {
		fmt.Fprintf(stderr, "melc: running %s in %s\n", strings.Join(argv, " "), project)
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Dir = project
		cmd.Stdout, cmd.Stderr = stderr, stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", strings.Join(argv, " "), err)
		}
		return nil
	}
	if p.NodeDeps {
		if _, err := os.Stat(filepath.Join(project, "node_modules")); err != nil {
			install := []string{"npm", "install", "--no-audit", "--no-fund"}
			if _, err := os.Stat(filepath.Join(project, "package-lock.json")); err == nil {
				install = []string{"npm", "ci", "--no-audit", "--no-fund"}
			}
			if err := run(install...); err != nil {
				return err
			}
		}
	}
	return run(p.Command...)
}
