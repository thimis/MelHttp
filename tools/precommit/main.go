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
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/thimis/MelHttp/internal/hygiene"
)

const hook = "#!/bin/sh\nexec go run ./tools/precommit\n"

func main() {
	all := flag.Bool("all", false, "check all tracked files instead of staged files")
	install := flag.Bool("install", false, "install as .git/hooks/pre-commit")
	flag.Parse()

	if *install {
		dir, err := gitOutput("rev-parse", "--git-path", "hooks")
		if err != nil {
			fail(err)
		}
		p := filepath.Join(strings.TrimSpace(dir), "pre-commit")
		if err := os.WriteFile(p, []byte(hook), 0o755); err != nil {
			fail(err)
		}
		fmt.Println("installed", p)
		return
	}

	args := []string{"diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z"}
	if *all {
		args = []string{"ls-files", "-z"}
	}
	out, err := gitOutput(args...)
	if err != nil {
		fail(err)
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
			data, err = os.ReadFile(f)
		} else {
			var s string
			s, err = gitOutput("show", ":"+f)
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
		fmt.Fprintln(os.Stderr, "precommit: refusing to commit:")
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "  "+p)
		}
		os.Exit(1)
	}
}

func gitOutput(args ...string) (string, error) {
	var out, errb bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, errb.String())
	}
	return out.String(), nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "precommit:", err)
	os.Exit(2)
}
