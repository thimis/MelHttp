# Changelog

All notable changes to MelHttp. Versions follow [Semantic Versioning](https://semver.org/);
while the version is 0.x, MelHttp is **alpha** and any release may change flags,
the site layout or the MelCGI contract.

## [Unreleased]

### Added
- `melhttpd -obfuscate-inject`: the Malbolge transport for any site, unchanged.
  The server adds the transport script to every HTML page.
- `scripts/serve -Obfuscate` (`--obfuscate`) turns it on for any demo or folder,
  e.g. `.\scripts\serve.ps1 angular -Obfuscate -Open`.

## [0.1.0] - 2026-10-02

First alpha release.

### Malbolge
- VM matching Ben Olmstead's 1998 reference interpreter, differential-tested
  against the original C code; fault-injection checks prove the tests catch
  real bugs.
- Generator that compiles any bytes, binary included, into Malbolge
  (about 7.5 cells per byte for text, 8–11 for binary and UTF-8). Every
  generated program is run and verified before it is written.

### Server (`melhttpd`)
- MelCGI/1.0: a strict, in-process CGI-like contract for Malbolge programs.
- Caching with ETags and gzip, warm-up, hot reload, SPA fallback, limits,
  `/healthz` and Prometheus metrics.
- HTTPS: automatic certificates (ACME), certificate files with reload,
  self-signed certificates for testing, HTTP→HTTPS redirect, HSTS, HTTP/2.
- Malbolge transport: pages travel to the browser as Malbolge programs and a
  service worker decodes them (obfuscation, not encryption).
- In-browser Malbolge playground.
- WASI handlers: WebAssembly modules as sandboxed handlers.
- Native Windows service; systemd, launchd, FreeBSD rc.d and Kubernetes
  examples.

### Toolchain (`melc`)
- `run`, `check`, `gen`, `build` and `watch`.
- Framework presets: Angular, Vite (React, Vue, Svelte), Create React App,
  Next, Nuxt, Astro, Gatsby, Hugo, Jekyll.

### Distribution
- Binaries for Linux, Windows, macOS and FreeBSD on x86-64 and ARM64.
- Docker image and a compose file with eight demo sites.
- `scripts/test` and `scripts/serve` (PowerShell and bash).

[0.1.0]: https://github.com/thimis/MelHttp/releases/tag/v0.1.0
