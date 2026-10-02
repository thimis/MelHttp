# Security

## Threat model

`melhttpd` serves untrusted network clients. A site's Malbolge programs come
from whoever built the site; they are treated as untrusted too.

## The sandbox

Programs run inside an in-process Malbolge VM. A Malbolge program can only
read bytes from stdin and write bytes to stdout. It has no files, no network,
no clock, no system calls, and no way to start a process. MelHttp never
spawns processes or interprets a program's output as anything but a strictly
parsed MelCGI response.

Per-request resource limits keep a hostile or buggy program from exhausting
the server. A server-wide semaphore bounds how many VMs run at once.

| Limit | Default | Enforced by |
|---|---|---|
| Instructions | 2 × 10⁹ | VM step counter |
| Output | 256 MiB | VM output counter |
| Wall time | 30 s | context cancellation, checked every 65 536 instructions |
| Request body | 1 MiB | `Content-Length` check plus `http.MaxBytesReader` |
| Memory | 118 KB of VM memory per run | fixed by the language (59049 cells) |

## Requests → programs (MelCGI input)

- Every meta-variable is forced onto one line: control bytes are
  percent-encoded.
- Headers with `_` or other unusual characters in their names are dropped,
  so `X_Forwarded_For` cannot spoof `X-Forwarded-For`.
- These request headers never reach programs: `Proxy` (httpoxy),
  `Connection`, `Upgrade`, `TE`, `Trailer`, `Transfer-Encoding`,
  `Keep-Alive`, `Proxy-Connection`.
- `Cookie`, `Authorization` and `Proxy-Authorization` are withheld unless
  `-allow-sensitive-headers` is set.

## Programs → responses (MelCGI output)

- The header block is limited to 8 KB, and only allowlisted headers are
  accepted: `Content-Type`, `Status`, `Location`, `Cache-Control`,
  `Set-Cookie`, `X-*` and a few others.
- Programs cannot set framing or server-owned headers (`Content-Length`,
  `Transfer-Encoding`, `X-Powered-By`, `X-Malbolge-*`, …).
- Malformed output becomes **502 Bad Gateway**, logged and never echoed to
  the client.

## Files

- **Exact matching.** URLs are matched against an index built from
  directory listings of the site root, opened with `os.Root` so nothing can
  resolve outside it. Path traversal (`..`, `%2e%2e`, `%2f`), Windows
  alternate data streams (`::$DATA`), 8.3 short names (`INDEX~1.HTM`),
  trailing dots and spaces, and case variations all miss the index. They
  behave the same on every OS.
- **Hidden files.** Dotfiles are never served (`.env`, `.git/`, the build
  manifest), except `/.well-known/`.
- **Program source.** `.mb` source is never served unless `-expose-source`
  is set, and then only under `/_source/` for program URLs.
- **Links.** Symlinks are never followed.

## Response headers

Every response gets `X-Content-Type-Options: nosniff`,
`X-Frame-Options: SAMEORIGIN`,
`Content-Security-Policy: frame-ancestors 'self'` and
`Referrer-Policy: strict-origin-when-cross-origin`.

Sites can add more, such as a full CSP or HSTS, with `headers` in
`melhttp.json`.

## HTTPS

- **Versions:** TLS 1.2 minimum (`-tls-min 1.3` for 1.3 only), and HTTP/2.
- **Certificates:** automatic ACME certificates are restricted to the configured `-acme-domains`;
  unknown SNI names get no certificate. Certificate files reload on change, and a
  half-written renewal keeps the old certificate.
- **Redirects and HSTS:** plain HTTP redirects to HTTPS (`308`); optional HSTS. The ACME cache
  holds private keys: keep it on a private, persistent volume (directory mode 0700).

## HTTP server

- **Timeouts:** read-header 10 s, read 60 s, write = program timeout + 60 s,
  idle 120 s.
- **Header limit:** 64 KB maximum header size.
- **Methods:** GET/HEAD for static files; GET/HEAD/POST for programs;
  everything else is 405.
- **Shutdown:** graceful on SIGINT/SIGTERM.

## Supply chain and repository

- **Dependencies:** the Go standard library only, with no cgo.
  `govulncheck` and `staticcheck` run in CI.
- **Docker:** a distroless, non-root image with a read-only root filesystem,
  all capabilities dropped and `no-new-privileges` (see `compose.yaml`).
- **Secrets:** `tools/precommit` refuses to commit keys, `.env` files and
  local tool configuration (`go run ./tools/precommit -install` adds it as a
  git hook). CI runs it over every tracked file.

## Testing

- **Fuzzing:** `FuzzLoad`, `FuzzRoundTrip`, `FuzzParseResponse` and
  `FuzzEncodeRequest`.
- **Server tests:** traversal, ADS, case and dotfile probes; limits;
  timeouts; and the 413/405/502/503 paths, all in `internal/server` and
  goal G4.
- **Race detector:** `go test -race`, run in CI.

## Reporting

Please report security issues privately to the repository owner, not in
public issues.

## Not a security feature

Malbolge "obfuscation" of traffic (see [roadmap.md](roadmap.md)) is
obfuscation, not encryption. Anyone can decode it with the public spec. Use
HTTPS for confidentiality.
