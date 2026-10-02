# Performance

Malbolge itself makes serving slower, not faster. Every output byte costs
7–40 VM instructions, and programs are 7–38× the size of what they print.
MelHttp makes that cost a one-time startup expense:

- **Warm-up.** At startup every program runs once. The response of every
  deterministic program (one that never reads stdin) is kept in memory with
  a SHA-256 ETag and a gzip variant.
- **Steady state.** Requests are answered from memory: conditional GET
  answers 304, `Range` and `HEAD` are supported, and hashed asset names are
  marked `immutable` so browsers never ask again.
- **Concurrent misses** for the same URL share one VM run (singleflight).

## Measurements

Intel Core Ultra 9 285K (24 cores), Windows 11, Go 1.27, loopback, 32
concurrent clients (`go run ./tools/crawl -load`).

| What | Result |
|---|---|
| VM speed | ~480 million Malbolge instructions/s per core |
| Program load (parse + 59049-cell memory fill) | ~22 µs |
| Compile ASCII text | ~12 MB/s, ~7.1–7.6 cells/byte |
| Compile binary data | ~5.5 MB/s, ~37 cells/byte |
| `melc build` of the Angular showcase (1.34 MB, 20 files) | 0.3 s → 727 programs, 16 MB of Malbolge (11.9×) |
| Warm-up of the Angular showcase | 15.9 M instructions, **74 ms** |
| `GET /` (8.8 KB index.html), cached | **40 800 req/s**, p50 0.55 ms, p99 2.2 ms |
| `GET /main-*.js` (270 KB) with gzip | 29 100 req/s, p99 2.9 ms |
| 64 KB page vs Go's `http.FileServer` (goal G7) | **1.6× faster** than `FileServer` |

MelHttp beats `http.FileServer` because, after warm-up, it never touches the
disk. The file server reads, stats and type-checks on every request.

## Tuning

| Flag | Effect |
|---|---|
| `-cache-mb` (512) | Response cache size. A whole site normally fits; when it doesn't, LRU eviction re-runs programs on demand. |
| `-concurrency` (2 × CPUs) | Simultaneous VM runs. Only matters for dynamic programs and cache misses. |
| `-warm=false` | Faster start, slower first requests. |
| `-no-cache` | Runs the VM on every request (purist mode). Expect roughly the VM speed above divided by the cells per byte. |

## Reproduce

```bash
go test -bench . ./internal/malbolge ./internal/gen
go test -tags acceptance -run G7 -v ./acceptance
melc build --preset angular -o /tmp/ang testsites/angular-showcase
melhttpd -root /tmp/ang -addr 127.0.0.1:8097 &
go run ./tools/crawl -load http://127.0.0.1:8097/ -n 20000 -c 32
```
