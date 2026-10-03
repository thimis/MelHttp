// Command precommit refuses to let secrets, keys or local tooling files be
// committed. Run it directly, or install it as a git pre-commit hook:
//
//	go run ./tools/precommit            # check staged files
//	go run ./tools/precommit -all       # check every tracked file
//	go run ./tools/precommit -install   # install .git/hooks/pre-commit
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/thimis/MelHttp/internal/hygiene"
)

const hook = "#!/bin/sh\nexec go run ./tools/precommit\n"

func main() {
	os.Exit(run(os.Args[1:], ".", os.Stdout, os.Stderr))
}

// run checks the git repository in dir. Exit codes: 0 clean, 1 refused, 2 error.
func run(args []string, dir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("precommit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	all := fs.Bool("all", false, "check all tracked files instead of staged files")
	install := fs.Bool("install", false, "install as .git/hooks/pre-commit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	git := func(args ...string) (string, error) { return gitOutput(dir, args...) }

	if *install {
		hooks, err := git("rev-parse", "--git-path", "hooks")
		if err != nil {
			fmt.Fprintln(stderr, "precommit:", err)
			return 2
		}
		p := strings.TrimSpace(hooks)
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		os.MkdirAll(p, 0o755)
		p = filepath.Join(p, "pre-commit")
		if err := os.WriteFile(p, []byte(hook), 0o755); err != nil {
			fmt.Fprintln(stderr, "precommit:", err)
			return 2
		}
		fmt.Fprintln(stdout, "installed", p)
		return 0
	}

	listArgs := []string{"diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z"}
	if *all {
		listArgs = []string{"ls-files", "-z"}
	}
	out, err := git(listArgs...)
	if err != nil {
		fmt.Fprintln(stderr, "precommit:", err)
		return 2
	}
	var problems []string
	for _, f := range strings.Split(out, "\x00") {
		if f == "" {
			continue
		}
		if r := hygiene.CheckPath(f); r != "" {
			problems = append(problems, f+": "+r)
			continue
		}
		var data []byte
		if *all {
			data, err = os.ReadFile(filepath.Join(dir, filepath.FromSlash(f)))
		} else {
			var s string
			s, err = git("show", ":"+f)
			data = []byte(s)
		}
		if err != nil {
			continue // deleted or unreadable: nothing to leak
		}
		if r := hygiene.CheckContent(data); r != "" {
			problems = append(problems, f+": "+r)
		}
	}
	if len(problems) > 0 {
		fmt.Fprintln(stderr, "precommit: refusing to commit:")
		for _, p := range problems {
			fmt.Fprintln(stderr, "  "+p)
		}
		return 1
	}
	return 0
}

func gitOutput(dir string, args ...string) (string, error) {
	var out, errb bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, errb.String())
	}
	return out.String(), nil
}
