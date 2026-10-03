# WebAssembly (WASI) handlers

Besides Malbolge programs, melhttpd can run **WebAssembly modules** (WASI
preview 1) as MelCGI handlers. The contract is the same as for Malbolge (see
[melcgi.md](melcgi.md)): the request arrives on stdin, and the response leaves
on stdout. Any language that targets `wasip1` works: Go, Rust, C/C++, Zig,
AssemblyScript and others.

```bash
GOOS=wasip1 GOARCH=wasm go build -o site/hello.html.wasi ./my-handler   # → /hello.html
melhttpd -root site -wasi
```

| On disk | URL | Output |
|---|---|---|
| `hello.html.wasi` | `/hello.html` | MelCGI response (header block, blank line, body) |
| `data.json.raw.wasi` | `/data.json` | raw: the whole output is the body, typed by `.json` |

`melc build` copies `.wasi` files unchanged after checking they are
WebAssembly. The module bytes themselves are never served.

A minimal handler in Go:

```go
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	vars := map[string]string{}
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() && in.Text() != "" { // meta-variables end at a blank line; the body follows
		k, v, _ := strings.Cut(in.Text(), "=")
		vars[k] = v
	}
	fmt.Printf("Content-Type: text/plain\n\nHello %s from WebAssembly!\n", vars["REMOTE_ADDR"])
}
```

## Sandbox

Handlers run in [wazero](https://wazero.io), a pure-Go WebAssembly runtime.
It has no cgo and runs on every OS melhttpd supports.

| | |
|---|---|
| Files | none: no directories are mounted |
| Environment | empty: the request comes on stdin, as MelCGI |
| Network | none: WASI preview 1 has no sockets, and none are granted |
| Memory | `-wasi-memory-mb` per run (default 64 MiB) |
| Time | `-timeout` (default 30 s): the module is stopped, and the client gets 503 |
| Output | `-max-output`: the module is stopped as soon as it exceeds it, and the client gets 500 |
| Clock, random | real wall clock and a crypto-grade random source |

A handler that exits non-zero, traps or runs out of memory produces a 500,
logged with its stderr. Invalid MelCGI output produces a 502.

Unlike Malbolge programs, WASI handlers are **never cached**. A module can read
the clock and random numbers, so its output may change on every request.
Modules are compiled during warm-up, so compilation never counts against a
request's time limit. They are recompiled when the file changes, and a warm run
takes a few milliseconds.

WASI support is **off by default**. Enable it with `-wasi` (`MELHTTP_WASI=true`).

## Tests

- `internal/wasi`: a real Go handler built for `wasip1` covers success,
  timeout, exit status, out-of-memory, output flood, no filesystem access,
  bad modules and recompilation.
- `internal/server`: routing, raw handlers, per-request execution, status
  mapping, and checks that module bytes are never served.
- **Goal G14**: builds `testsites/wasi`, serves it with `melhttpd -wasi`, and
  checks the page and that each response is fresh.
