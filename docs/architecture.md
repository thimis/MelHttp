# Architecture

MelHttp has two halves: a **compiler** (`melc`) that turns a website into
Malbolge programs ahead of time, and a **server** (`melhttpd`) that runs those
programs in a sandboxed VM to answer requests.

```
            build time                                   request time
 ┌──────────────────────────────┐        ┌────────────────────────────────────────────────┐
 │ your site (HTML, JS, fonts…) │        │ net/http                                       │
 │            │                 │        │   → middleware (security headers, access log)  │
 │      melc build              │        │   → resolver   URL → index entry               │
 │   gen: bytes → Malbolge      │        │   → cache      hit? ──────────────┐            │
 │   verify: run every program  │        │   → executor   MelCGI + VM        │            │
 │            ▼                 │  ───▶  │   → response   status, headers, body           │
 │ site/ (*.mb programs)        │        │   → writer     ETag/304, Range, gzip, HEAD     │
 └──────────────────────────────┘        └────────────────────────────────────────────────┘
```

## Packages

| Package | Role |
|---|---|
| `internal/malbolge` | The VM, matching the 1998 reference interpreter: loader, runner, single-step `Machine`. Pure Go, no I/O, so it also compiles to WebAssembly. |
| `internal/gen` | The compiler from bytes to Malbolge (see [generator.md](generator.md)). Pure Go. |
| `internal/mbfile` | Loads and writes programs: a single `.mb` file or a chunk directory. |
| `internal/melcgi` | MelCGI/1.0: encodes requests and strictly parses responses (see [melcgi.md](melcgi.md)). |
| `internal/server` | Site index, executor, cache and HTTP handler. |
| `internal/config` | Flags and `MELHTTP_*` environment variables for `melhttpd`. |
| `internal/mimetype` | A fixed Content-Type table that behaves the same on every OS. |
| `internal/crawl` | Checks that every source file is served byte-identically (used by tests and `tools/crawl`). |

## How a site maps to URLs

The server indexes the site directory at startup. It rescans when a request
misses, at most once per second, so new files appear without a restart.

| On disk | URL | Served as |
|---|---|---|
| `about.html.mb` | `/about.html` | MelCGI program: prints headers, a blank line, then the body |
| `about.html.mb/000.mb`, `001.mb`, … | `/about.html` | chunked program: chunks run in order and their outputs concatenate |
| `echo.txt.raw.mb` | `/echo.txt` | raw program: the whole output is the body; the type comes from `.txt` |
| `index.html.mb` | `/` and `/index.html` | directory index (`/docs` redirects to `/docs/`) |
| `404.html.mb` | any unknown URL | custom 404 page, status 404 |
| `style.css` | `/style.css` | static file (allowed, but `melc build` compiles everything) |
| `melhttp.json` | — | site config: `spa`, `immutable` patterns, extra `headers` |
| dotfiles | — | never served, except `/.well-known/` |

Lookups are exact matches against names read from directory listings.
Path traversal, Windows alternate data streams (`::$DATA`), 8.3 short names,
trailing dots and case variations therefore cannot reach anything, and every
OS behaves the same. The `.mb` source is never served unless `-expose-source`
is on, and then only under `/_source/`.

If a program and a static file map to the same URL, the program wins and a
warning is logged.

## Requests and caching

1. A program that never executes the input instruction `/` is
   **deterministic**: its output depends only on the program. Its response is
   cached in memory (LRU, `-cache-mb`) together with:
   - a strong ETag (SHA-256),
   - a gzip variant for compressible types over 1 KB.
2. A program that reads its input is **dynamic**. It runs on every request
   with the request on stdin, and its response is sent with
   `Cache-Control: no-store`.
3. Concurrent misses for the same URL share one VM run (singleflight).
4. **Warm-up**: at startup every program runs once, so even the first visitor
   gets a cache hit.
5. **Hot reload**: a cached entry re-checks its files (size and modification
   time of the file or of every chunk) at most once per second and re-runs
   when they change.
6. Content-hashed names such as `main-B3YI7OQ3.js` get
   `Cache-Control: public, max-age=31536000, immutable`. Everything else gets
   `no-cache` (revalidate with the ETag).

## Limits (per request unless noted)

| Limit | Flag | Default | On breach |
|---|---|---|---|
| VM instructions | `-max-steps` | 2 000 000 000 | 500 |
| Output bytes | `-max-output` | 256 MiB | 500 |
| Wall time per run | `-timeout` | 30 s | 503 |
| Request body | `-max-body` | 1 MiB | 413 |
| Concurrent VM runs (server-wide) | `-concurrency` | 2 × CPUs | requests queue |
| Response cache (server-wide) | `-cache-mb` | 512 MiB | LRU eviction |
| Invalid MelCGI output | — | — | 502 |

Errors are logged with details. Clients only see the status text.

## Response headers

Every response carries `X-Content-Type-Options: nosniff`,
`Referrer-Policy: strict-origin-when-cross-origin`,
`X-Frame-Options: SAMEORIGIN` and
`Content-Security-Policy: frame-ancestors 'self'`.

Program responses also carry:
- `X-Powered-By: Malbolge`,
- `X-Malbolge-Steps` (instructions executed),
- `X-Malbolge-Cache: hit|miss`.
