# MelCGI/1.0

MelCGI is the contract between MelHttp and the Malbolge program that produces
a page. It is modelled on CGI/1.1 (RFC 3875), adapted to a program that has
only a byte stream for input and a byte stream for output: there are no
environment variables, so the meta-variables are sent on stdin ahead of the
request body.

The reference implementation is `internal/melcgi`.

## Input: what the program reads

stdin carries, in order:

1. a **meta-variable block**: one `NAME=value` line per variable, each ending
   in a single LF (`\n`);
2. a **blank line** (`\n`);
3. the **request body**, byte-exact (`CONTENT_LENGTH` bytes).

The body is streamed lazily: nothing is read from the client until the
program has consumed the whole meta block. A program that never reads past the
blank line never causes the body to be read.

Example (a `GET /echo.txt?x=1&y=two` with two headers):

```
GATEWAY_INTERFACE=MelCGI/1.0
SERVER_PROTOCOL=HTTP/1.1
REQUEST_METHOD=GET
SCRIPT_NAME=/echo.txt
PATH_INFO=
QUERY_STRING=x=1&y=two
SERVER_NAME=example.com
SERVER_PORT=8080
REMOTE_ADDR=127.0.0.1
HTTP_ACCEPT=*/*
HTTP_X_TEST=yes

```

A `POST` with an 11-byte `text/plain` body additionally has
`CONTENT_TYPE=text/plain` and `CONTENT_LENGTH=11` after `REMOTE_ADDR`, and the
11 body bytes follow the blank line.

### Variables

The standard variables always appear in this order; the optional ones are
omitted, not sent empty.

| Variable            | Present                  | Value |
|---------------------|--------------------------|-------|
| `GATEWAY_INTERFACE` | always                   | `MelCGI/1.0` |
| `SERVER_PROTOCOL`   | always                   | request protocol, e.g. `HTTP/1.1`, `HTTP/2.0` |
| `REQUEST_METHOD`    | always                   | e.g. `GET`, `POST` |
| `SCRIPT_NAME`       | always                   | URL path that selected the program, e.g. `/echo.txt` |
| `PATH_INFO`         | always (may be empty)    | the part of the URL path after `SCRIPT_NAME` |
| `QUERY_STRING`      | always (may be empty)    | raw query string without `?`, still percent-encoded |
| `SERVER_NAME`       | always                   | host part of the `Host` header (IPv6 without brackets) |
| `SERVER_PORT`       | always                   | port of the `Host` header; `80` if absent (`443` over TLS) |
| `REMOTE_ADDR`       | always                   | client IP address, without port (`::1`, not `[::1]:1234`) |
| `HTTPS`             | request came over TLS    | `on` |
| `REQUEST_SCHEME`    | request came over TLS    | `https` (absent means plain `http`) |
| `CONTENT_TYPE`      | request has Content-Type | the request `Content-Type` |
| `CONTENT_LENGTH`    | body length > 0          | body length in bytes, decimal |
| `HTTP_*`            | see below                | request headers, sorted by variable name |

### Request headers (`HTTP_*`)

A request header `Name` becomes `HTTP_` + `Name` upper-cased with `-` replaced
by `_` (`Accept-Language` → `HTTP_ACCEPT_LANGUAGE`). Multiple values, and
headers whose names differ only in case, are joined with `", "`. The variables
are sorted by name and follow the standard variables.

Headers are dropped when:

- the name contains anything other than ASCII letters, digits and `-`. In
  particular a name containing `_` is skipped (as nginx does), so that
  `X_Forwarded_For` cannot masquerade as `X-Forwarded-For`;
- the header is on the **never-pass** list:

  | Header | Reason |
  |--------|--------|
  | `Proxy` | httpoxy: `HTTP_PROXY` is read as a proxy setting by many HTTP clients |
  | `Content-Type`, `Content-Length` | have their own variables |
  | `Connection`, `Upgrade`, `Te`, `Trailer`, `Transfer-Encoding`, `Keep-Alive`, `Proxy-Connection` | hop-by-hop, meaningless to the program |

- the header is **sensitive** (`Authorization`, `Cookie`,
  `Proxy-Authorization`) and the server was not configured to pass sensitive
  headers (`Options.AllowSensitiveHeaders`, off by default).

### Escaping

Every value is made to fit on exactly one line: each **control byte** — any
byte below `0x20` except tab (`0x09`), and `0x7F` — is replaced by `%` and two
upper-case hex digits (`\r\n` → `%0D%0A`, NUL → `%00`).

**Only control bytes are escaped.** A literal `%` is passed unchanged (query
strings already contain `%XX` sequences, and double-encoding them would be
worse than useless), as are tabs and bytes `0x80`–`0xFF`. A program that sees
`%0A` therefore cannot tell whether the client sent a newline or the three
characters `%0A`; neither can it be tricked into seeing an extra variable.
For example a header `X-Test: a\r\nEVIL=1` arrives as

```
HTTP_X_TEST=a%0D%0AEVIL=1
```

Every line of the meta block matches `^[A-Z][A-Z0-9_]*=[^\n]*$`, contains no
control byte other than tab, and the block contains exactly one blank line:
its terminator.

## Output: what the program prints

The program prints a CGI response: header lines, a blank line, the body.

```
Status: 404 Not Found
Content-Type: text/html; charset=utf-8

<h1>Not here</h1>
```

Generated programs normally begin with the minimal block
`Content-Type: <type>\n\n` (`melcgi.HeaderBlock`).

### Rules

- Lines end in LF or CRLF; the two may be mixed.
- The header block ends at the first empty line. It must end within the first
  **8192 bytes** (`MaxHeaderBlock`, counting the blank line itself).
- Each header line is `Name: value`. The name must be an RFC 7230 token
  (no spaces, not even before the colon). Spaces and tabs after the colon and
  at the end of the line are trimmed.
- Lines beginning with a space or tab (obsolete line folding) are rejected.
- A value may not contain a lone CR or any control byte other than tab.
- Header names are case-insensitive; they are stored canonicalized
  (`content-type` → `Content-Type`).
- `Status`, `Content-Type` and `Location` may each appear at most once;
  `Content-Type` and `Location` may not be empty. Other allowed headers may
  repeat (`Set-Cookie`, `Link`, ...).
- `Status: NNN` or `Status: NNN Reason`. NNN must be three digits in
  **200–599** (a single space separates the reason). The reason phrase is
  ignored. `Status` is consumed by the server and never sent as a header.
- Without `Status`, the status is **200**, or **302** if `Location` is present.
- `Content-Type` is required unless `Location` is present.
- The body is everything after the blank line, byte-exact: it may contain
  further blank lines, CR bytes, NUL, anything.

### Allowed headers

Case-insensitive:

- `Content-Type`, `Status`, `Location`
- `Cache-Control`, `Content-Language`, `Content-Disposition`, `Last-Modified`,
  `Expires`, `Link`, `Set-Cookie`, `Vary`, `Access-Control-Allow-Origin`
- any `X-*` header, **except** `X-Powered-By` and `X-Malbolge` / `X-Malbolge-*`,
  which the server owns.

Everything else is forbidden. In particular `Content-Length`,
`Transfer-Encoding`, `Connection`, `Content-Encoding`, `Date`, `Server`,
`ETag`, `Set-Cookie2` and `Strict-Transport-Security` are rejected: framing,
encoding and transport security are the server's business.

### Errors and HTTP mapping

`melcgi.ParseResponse` returns an error wrapping one of:

| Error | Cause |
|-------|-------|
| `ErrNoHeaderBlock` | the output (≤ 8192 bytes) contains no blank line |
| `ErrHeaderTooLarge` | the output is longer than 8192 bytes and no blank line ends within the first 8192 |
| `ErrBadHeader` | malformed line, folding, bad name, control byte in value, duplicate or empty `Status`/`Content-Type`/`Location` |
| `ErrForbiddenHeader` | header not on the allowlist |
| `ErrBadStatus` | `Status` not three digits in 200–599 |
| `ErrNoContentType` | no `Content-Type` and no `Location` (includes an empty header block) |

**Any parse error is answered with `502 Bad Gateway`**: the program, acting
as the upstream, produced an invalid response. Nothing the program printed
is forwarded to the client.

### Raw programs

A program named `*.raw.mb` skips response parsing entirely: its whole output
is the body, sent with status 200 and the Content-Type derived from the file
name (`melcgi.RawResponse`). Raw programs exist for classic Malbolge programs
such as "Hello, world." that know nothing of MelCGI. They still receive the
meta block on stdin.

## Examples

Redirect (status 302, no Content-Type needed):

```
Location: /new-place

```

Permanent redirect:

```
Status: 301 Moved Permanently
Location: https://example.com/

```

Two cookies, CRLF line endings, binary body:

```
Content-Type: application/octet-stream\r\n
Set-Cookie: a=1\r\n
Set-Cookie: b=2\r\n
\r\n
<bytes>
```

Rejected (→ 502): `Content-Length: 5` (forbidden), `X-Powered-By: Malbolge`
(server-owned), `Status: 199` (out of range), output with no blank line.
