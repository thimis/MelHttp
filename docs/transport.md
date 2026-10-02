# Malbolge transport and the browser VM

melhttpd can send responses **as Malbolge programs**. Visitors' browsers then
execute the programs, using the MelHttp VM compiled to WebAssembly, to
recover the page. The pages still work normally; only their transfer is in
Malbolge.

> **This is obfuscation, not encryption.** The format is public and the decoder
> ships with every page, so anyone can run the programs. The transport stops
> naive keyword matching on the wire and is a fun demonstration, nothing more.
> Use HTTPS for confidentiality; the two combine fine.

## Turning it on

```bash
melhttpd -root site -obfuscate          # plus -playground for the in-browser playground
```

Add one line to the pages that should opt in:

```html
<script src="/_melhttp/obfuscate.js"></script>
```

1. The first visit is ordinary HTTP. `obfuscate.js` installs a service worker
   (`/_melhttp/sw.js`) for the whole site.
2. From the next navigation on, the worker re-sends every same-origin GET with
   `X-Malbolge-Accept: program`.
3. melhttpd answers with the body compiled to Malbolge (`text/x-malbolge`), and
   sends the original Content-Type in `X-Malbolge-Content-Type`.
4. The worker loads `/_melhttp/melhttp.wasm` (the Go VM, about 0.8 MB gzipped),
   runs the programs, and hands the page the original bytes. It adds
   `X-Malbolge-Decoded: service-worker` so you can see what happened.

Service workers need a secure context: HTTPS, or `localhost`/`127.0.0.1` for
testing.

## On the wire

```http
GET /about.html
X-Malbolge-Accept: program

HTTP/1.1 200 OK
Content-Type: text/x-malbolge; charset=us-ascii
X-Malbolge-Encoding: program
X-Malbolge-Content-Type: text/html; charset=utf-8
Cache-Control: no-store
Vary: X-Malbolge-Accept

D'`A@?>=<;:9876543210/.-,+*)('&%$#"!~}|{)yxwpun4lTj0Qmled*ba`&dcbaZYX|?[=<XWPtTMLpP2NGFEi,HGF(D=<;_">=6|4X8xw/…
```

- The body is one or more Malbolge programs separated by blank lines
  (`internal/obfs`).
- Each page is encoded with a **random seed**, so the same page looks
  different from one response to the next.
- melhttpd keeps `-obfuscate-variants` encodings per page (default 2) and picks
  one at random for each request, so it doesn't recompile on every hit.
- Dynamic responses get a fresh encoding every time.
- Error responses (404 and so on) are sent as usual.
- Every program begins with the same fixed 42-cell prefix (the `D'`A@?>=<;:98…` that
  sets up the machine), so encoded responses are easy to recognise as MelHttp traffic.
  Again: obfuscation, not secrecy.

Encoded bodies are 7–40× larger than the original. gzip, applied when the
browser accepts it, reduces that substantially. Decoding takes milliseconds:
the Angular showcase's 270 KB main bundle is about 2.2 M instructions.

## The playground

With `-playground`, `/_melhttp/playground.html` runs the same WebAssembly
build. You can paste any Malbolge program and run it, or compile text into
Malbolge and run the result. All of it happens in the browser.

## Building the browser VM

`melhttp.wasm` and its loader `wasm_exec.js` are generated, not committed:

```bash
go generate ./internal/webvm   # GOOS=js GOARCH=wasm build of cmd/melwasm
```

- `tools/dist`, the Dockerfile and CI run this automatically.
- A melhttpd built without it refuses `-obfuscate` and `-playground` with a clear
  message.

## Tests

- `internal/obfs`: encode/decode round trips (random bytes, multi-program,
  CRLF), and errors for non-halting or invalid programs.
- `internal/webvm`: asset serving and gzip, plus **the real wasm under
  Node.js**: it runs Hello World, decodes a Go-encoded container and compiles
  text.
- `internal/server`: encoded responses decode to the page, never contain the
  page text, and vary between requests; the feature is off by default.
- **Goal G13**: in real Chromium, Playwright checks:
  - the service worker installs, and the wire carries only Malbolge text;
  - pages, navigations, UTF-8 and a PNG decode correctly;
  - the playground runs and compiles programs.
