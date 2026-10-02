// Command testreport turns `go test -json` output into a readable report:
// package results as they finish, then totals, every skipped test with its
// reason, and every failed test with its output. It exits 1 if anything
// failed (or nothing ran), so scripts can rely on its exit code.
//
//	go test -json ./... | go run ./tools/testreport
//	go test -json -tags acceptance ./acceptance | go run ./tools/testreport -each
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// event is one line of `go test -json` output.
type event struct {
	Action     string
	Package    string
	ImportPath string // set instead of Package on build events (e.g. "pkg.test")
	Test       string
	Output     string
	Elapsed    float64
}

type result struct {
	name   string // "package TestName"
	detail []string
}

type report struct {
	each                  bool // print every top-level test as it finishes
	passed, failed, skips int
	packages              int
	failedPkgs            []string
	skipped, failures     []result
	outputs               map[string][]string // test key → output lines
}

func main() {
	each := flag.Bool("each", false, "print each top-level test result as it finishes")
	flag.Parse()
	os.Exit(run(os.Stdin, os.Stdout, *each))
}

func run(in io.Reader, out io.Writer, each bool) int {
	r := &report{each: each, outputs: map[string][]string{}}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimPrefix(sc.Bytes(), []byte("\xef\xbb\xbf")) // byte-order mark (Windows PowerShell pipes)
		var e event
		if err := json.Unmarshal(line, &e); err != nil || e.Action == "" {
			// Not JSON: build errors and the like. Show them as they are.
			fmt.Fprintln(out, string(line))
			continue
		}
		if e.Package == "" {
			e.Package, _, _ = strings.Cut(e.ImportPath, " ")
			e.Package = strings.TrimSuffix(e.Package, ".test")
		}
		r.add(out, e)
	}
	return r.summary(out)
}

func (r *report) add(out io.Writer, e event) {
	key := e.Package + " " + e.Test
	switch e.Action {
	case "output", "build-output":
		if e.Test == "" {
			// Package lines: "ok  pkg 0.2s", "FAIL pkg", "?  pkg [no test files]", build errors.
			s := strings.TrimRight(e.Output, "\n")
			if strings.HasPrefix(s, "ok ") || strings.HasPrefix(s, "FAIL") || strings.HasPrefix(s, "?") ||
				e.Action == "build-output" || strings.HasPrefix(s, "panic") {
				fmt.Fprintln(out, s)
			}
			return
		}
		r.outputs[key] = append(r.outputs[key], strings.TrimRight(e.Output, "\n"))
	case "pass", "fail", "skip":
		if e.Test == "" {
			r.packages++
			if e.Action == "fail" {
				r.failedPkgs = append(r.failedPkgs, e.Package)
			}
			return
		}
		top := !strings.Contains(e.Test, "/")
		switch e.Action {
		case "pass":
			r.passed++
		case "fail":
			r.failed++
			r.failures = append(r.failures, result{shortPkg(e.Package) + " " + e.Test, r.outputs[key]})
		case "skip":
			r.skips++
			r.skipped = append(r.skipped, result{shortPkg(e.Package) + " " + e.Test, []string{skipReason(r.outputs[key])}})
		}
		if r.each && top {
			fmt.Fprintf(out, "    %-4s %s (%.1fs)\n", strings.ToUpper(e.Action), e.Test, e.Elapsed)
		}
		delete(r.outputs, key)
	case "build-fail":
		r.failedPkgs = append(r.failedPkgs, e.Package+" (build failed)")
	}
}

// skipReason is the message passed to t.Skip: the last output line that is
// not one of go test's own status lines.
func skipReason(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		s := strings.TrimSpace(lines[i])
		if s == "" || strings.HasPrefix(s, "=== ") || strings.HasPrefix(s, "--- ") {
			continue
		}
		if _, msg, ok := strings.Cut(s, ": "); ok && strings.Contains(s, "_test.go:") {
			return msg
		}
		return s
	}
	return "(no reason given)"
}

func shortPkg(p string) string {
	return strings.TrimPrefix(p, "github.com/thimis/MelHttp/")
}

func (r *report) summary(out io.Writer) int {
	fmt.Fprintf(out, "\nTests: %d passed, %d failed, %d skipped in %d packages\n", r.passed, r.failed, r.skips, r.packages)
	if len(r.skipped) > 0 {
		sort.Slice(r.skipped, func(i, j int) bool { return r.skipped[i].name < r.skipped[j].name })
		fmt.Fprintln(out, "Skipped (not run, so not verified):")
		for _, s := range r.skipped {
			fmt.Fprintf(out, "  %s: %s\n", s.name, s.detail[0])
		}
	}
	if len(r.failures) > 0 {
		fmt.Fprintln(out, "Failed:")
		for _, f := range r.failures {
			fmt.Fprintf(out, "  %s\n", f.name)
			for _, l := range f.detail {
				if s := strings.TrimSpace(l); s != "" && !strings.HasPrefix(s, "=== ") {
					fmt.Fprintf(out, "      %s\n", s)
				}
			}
		}
	}
	for _, p := range r.failedPkgs {
		fmt.Fprintf(out, "Package failed: %s\n", p)
	}
	if r.failed > 0 || len(r.failedPkgs) > 0 || r.packages == 0 {
		if r.packages == 0 {
			fmt.Fprintln(out, "No test results: did `go test` run?")
		}
		return 1
	}
	return 0
}
