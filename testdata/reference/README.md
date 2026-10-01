# Reference interpreter harness

`malbolge.c` is Ben Olmstead's original 1998 Malbolge interpreter, verbatim
(public domain, extracted from https://www.lscheffer.com/malbolge_interp.html).

`harness.c` is the same file with minimal, behavior-preserving patches so it
can be used as a test oracle for our Go VM:

| Reference behavior | Harness behavior (= Go VM) |
|---|---|
| executing a cell outside 33..126 spins forever | stop, print `INVALID <steps>`, exit 4 |
| encrypting a cell outside 33..126 reads out of bounds (UB) | leave the cell unchanged |
| no step limit | stop after `MAL_STEPS` instructions, print `LIMIT <steps>`, exit 3 |
| `v` halts silently | print `HALT <steps>` to stderr |
| `#include <malloc.h>` | removed (not available on musl) |

The Go tests behind the `reference` build tag build this image and compare
outputs, final status and step counts:

    go test -tags reference ./internal/malbolge ./internal/gen
