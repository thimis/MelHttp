# Testing

```bash
go test ./...                                         # unit tests (all OSes)
go test -tags acceptance -v ./acceptance              # the goal ladder (G8 needs MELHTTP_DOCKER=1)
go test -tags reference ./internal/malbolge ./internal/gen   # against the 1998 reference interpreter (Docker)
docker run --rm -v "$PWD":/src -w /src golang:1.27 go test -race ./...   # race detector (needs cgo on Windows)
```

## The goal ladder

The `acceptance/` tests drive the real `melc` and `melhttpd` binaries the way
a user would. They were written before the code and measure progress toward
the complete product.

| Goal | What it proves |
|---|---|
| G1 | The VM runs real Malbolge exactly like the reference interpreter (golden programs, plus a 1000-program differential test). |
| G2 | The generator turns **any** bytes into Malbolge that prints them back: completeness proof, all 256 byte values, 1 MB random, a real-world corpus. |
| G3 | MelCGI: programs see the request; malformed output becomes 502; fuzzers find nothing. |
| G4 | The server is correct and safe: types, 404/405, traversal, ADS and case tricks, HEAD/304/Range/gzip. |
| G5 | `melc build` + `melhttpd`: every file of the hand-made sites is served byte-identical. |
| G6 | The Angular Material showcase runs from Malbolge: byte-identical crawl, deep links, Playwright browser tests. |
| G7 | Cached serving is at least as fast as Go's `http.FileServer`. |
| G8 | Docker: the image builds, containers are healthy, crawls pass against them. |
| G9 | Every OS/arch builds; the VM and generator compile to WebAssembly. |
| G10 | The React and Vue sites convert with `--preset auto` and pass crawls, deep links and Playwright. |
| G11 | The repository is safe to publish: no secrets or local tool files are tracked, and README links resolve. |

## Layers of assurance for the converter

1. **Completeness proofs.** `TestLag1Completeness` and `TestTapeCompleteness`
   show every byte is printable from every state.
2. **Model ↔ VM checks.** `TestTapeOnRealVM` checks the generator's model of
   the machine against real execution.
3. **Self-verification.** Every program `melc` writes has already been run
   and compared with its input.
4. **An independent oracle.** Generated programs also run correctly on Ben
   Olmstead's original C interpreter.
5. **Fuzzing.** `FuzzRoundTrip` covers random bytes, seeds and segment sizes.
6. **Real files.** `testdata/corpus` holds HTML, JS, CSS, SVG, PNG, JPEG, GIF,
   wasm, UTF-16, BOM, CRLF, empty and boundary-sized files.
7. **End to end.** Byte-identical crawls of whole sites (G5, G6, G8, G10).

## Fixtures

- `testdata/programs/` holds third-party Malbolge programs with expected
  outputs; [SOURCES.md](../testdata/programs/SOURCES.md) credits their
  authors.
- `testdata/corpus/` is regenerated with `go run ./tools/mkcorpus`.
- `testdata/reference/` contains the reference interpreter and its harness
  patch.
- `testsites/` holds the demo and test websites
  ([testsites/README.md](../testsites/README.md)).
