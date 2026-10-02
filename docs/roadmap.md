# Roadmap

## Done

- Malbolge VM matching the 1998 reference interpreter (differential-tested)
- Generator that turns any bytes into Malbolge, with completeness proofs
- MelCGI/1.0: a safe, in-process CGI contract
- `melhttpd`: caching, warm-up, gzip, conditional requests, SPA fallback, hot reload, limits
- `melc build` with framework presets (Angular, Vite/React/Vue/Svelte, CRA, Next, Nuxt, Astro, Gatsby, Hugo, Jekyll)
- Docker image and compose demo, release archives for 8 platforms, CI on Linux/Windows/macOS
- HTTPS: automatic Let's Encrypt certificates (ACME), certificate files with hot reload,
  self-signed development mode, HTTP→HTTPS redirect, HSTS, HTTP/2, TLS 1.2+ (goal G12
  issues a real certificate from Pebble, the ACME test CA)

## Next: Malbolge traffic obfuscation

The idea is `Content-Encoding: malbolge`: the server sends response bodies
*as Malbolge programs*, and the browser executes them to recover the content.

- A service worker would run the Go VM compiled to WebAssembly (the VM and
  generator already compile for `js/wasm` and `wasip1`; goal G9 checks it).
- Request bodies could travel the same way.
- `gen.Options.Seed` already produces different programs for identical
  content, so the same page looks different on every response.

**This is obfuscation, not encryption.** The format is public and anyone can
run the programs. It hides traffic from naive pattern matching only. Use it
on top of TLS, never instead of it.

## Later: WebAssembly

1. **Serving wasm** works today: `.wasm` is served as `application/wasm`
   (compiled to Malbolge like any other file), so `instantiateStreaming` works
   for Blazor, Rust, Go and other wasm frontends.
2. **Malbolge in the browser:** a playground and the obfuscation decoder
   above, using the existing pure-Go VM compiled to wasm.
3. **WASI programs as MelCGI handlers:** a second sandboxed executor behind
   the same interface (stdin = request, stdout = CGI response), using a
   pure-Go runtime such as `wazero` (no cgo, every OS).

## Ideas

- Smaller output for UTF-8-heavy text: today a file with frequent non-ASCII
  bytes uses tape chunks throughout (~35 cells/byte). Splitting at a finer
  grain, or a richer "second-generation" tape, could bring it closer to the
  ASCII cost (~7.5 cells/byte).
- A native Windows service (`golang.org/x/sys/windows/svc`, behind
  `//go:build windows`).
- `melc watch`: rebuild on change during development.
- Prometheus metrics: VM runs, steps, cache hit rate.
