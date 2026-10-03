// Command handler is a MelCGI handler compiled to WASI, used by the tests:
//
//	GOOS=wasip1 GOARCH=wasm go build -o handler.wasi ./testdata/wasi/handler
//
// The query selects a behavior: (none) greet, loop, oom, exit, fs, flood.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	vars := map[string]string{}
	in := bufio.NewReader(os.Stdin)
	for {
		line, err := in.ReadString('\n')
		line = strings.TrimSuffix(line, "\n")
		if line == "" || err != nil {
			break
		}
		k, v, _ := strings.Cut(line, "=")
		vars[k] = v
	}
	body, _ := io.ReadAll(in)
	switch vars["QUERY_STRING"] {
	case "loop":
		for {
		}
	case "oom":
		var keep [][]byte
		for {
			keep = append(keep, make([]byte, 1<<20))
		}
	case "exit":
		fmt.Fprintln(os.Stderr, "failing on purpose")
		os.Exit(3)
	case "fs":
		_, err := os.ReadFile("/etc/passwd")
		fmt.Printf("Content-Type: text/plain\n\nfs: %v\n", err)
		return
	case "flood":
		fmt.Print("Content-Type: text/plain\n\n")
		for {
			fmt.Print(strings.Repeat("x", 4096))
		}
	}
	fmt.Printf("Content-Type: text/plain; charset=utf-8\n\nHello from WASI! method=%s script=%s https=%s body=%q env=%d\n",
		vars["REQUEST_METHOD"], vars["SCRIPT_NAME"], vars["HTTPS"], body, len(os.Environ()))
}
