# Roadmap

## Done

- **VM** matching the 1998 reference interpreter: differential-tested, and
  fault-injection checks show the tests would catch real bugs.
- **Generator** that turns any bytes into Malbolge, with completeness proofs
  for both chunk modes.
- **MelCGI/1.0**: a safe, in-process CGI contract (with `HTTPS`/`REQUEST_SCHEME`
  for TLS requests).
- **`melhttpd`**: caching, warm-up, gzip, conditional requests, SPA fallback,
  hot reload, limits.
- **`melc build`** with framework presets: Angular, Vite (React/Vue/Svelte),
  CRA, Next, Nuxt, Astro, Gatsby, Hugo, Jekyll.
- **Packaging**: a Docker image with a compose demo of eight sites, release
  archives for 8 platforms, and CI on Linux/Windows/macOS.
- **HTTPS** ([deployment.md](deployment.md#https)):
  - automatic Let's Encrypt certificates (ACME, with any RFC 8555 CA);
  - certificate files with hot reload, and a self-signed development mode;
  - HTTP→HTTPS redirects, HSTS, HTTP/2, TLS 1.2+;
  - goal G12 gets a real certificate from Pebble.
- **Malbolge traffic obfuscation** ([transport.md](transport.md)): responses
  travel as randomly seeded Malbolge programs, and a service worker decodes
  them with the Go VM compiled to WebAssembly. This is obfuscation, not
  encryption. Goal G13 runs it in real Chromium.
- **WebAssembly**:
  1. `.wasm` files are served as `application/wasm`, so `instantiateStreaming`
     works (the classic test site uses it, and goal G5 checks it).
  2. The Malbolge VM, decoder and generator run in browsers: the service
     worker and the [playground](transport.md#the-playground).
  3. WASI modules run as sandboxed MelCGI handlers ([wasi.md](wasi.md)); goal
     G14 checks this.

## Ideas

- **Smaller UTF-8 output.** A file with frequent non-ASCII bytes uses tape
  chunks throughout (~35 cells/byte). Finer-grained splitting, or a richer
  "second-generation" tape that the content writes and then re-reads, could
  bring it closer to the ASCII cost (~7.5 cells/byte).
- **Less recognisable transport.** Every program starts with the same 42-cell
  prefix. Several equivalent prefixes, chosen at random, would make encoded
  responses harder to fingerprint.
- **A native Windows service** using `golang.org/x/sys/windows/svc` behind
  `//go:build windows`.
- **`melc watch`**: rebuild on change during development.
- **Prometheus metrics**: VM runs, instructions executed, cache hit rate, WASI
  runs.
- **Request bodies through the transport.** Today only responses travel as
  Malbolge.
