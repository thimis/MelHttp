# Roadmap

## Done

- **VM** matching the 1998 reference interpreter: differential-tested, and
  fault-injection checks show the tests would catch real bugs.
- **Generator** that turns any bytes into Malbolge:
  - lag-1 chunks for text (~7.5 cells/byte);
  - **sweep chunks** for binary and UTF-8 (~8–11 cells/byte, about 4× smaller
    than before);
  - proven tape chunks as the fallback.
- **MelCGI/1.0**: a safe, in-process CGI contract (with `HTTPS`/`REQUEST_SCHEME`
  for TLS requests).
- **`melhttpd`**: caching, warm-up, gzip, conditional requests, SPA fallback,
  hot reload, limits.
- **`melc`**:
  - `build` with framework presets (Angular, Vite (React/Vue/Svelte), CRA,
    Next, Nuxt, Astro, Gatsby, Hugo, Jekyll);
  - **`watch`** with a live-reloading dev server;
  - incremental, **in-place** rebuilds that work while the site is served.
- **HTTPS** ([deployment.md](deployment.md#https)):
  - Let's Encrypt (ACME, with any RFC 8555 CA);
  - certificate files with hot reload, and a self-signed development mode;
  - redirects, HSTS, HTTP/2, TLS 1.2+.
- **Malbolge traffic obfuscation** ([transport.md](transport.md)), in both
  directions:
  - responses travel as randomly seeded Malbolge programs, with **randomized
    prefixes**, and request bodies are compiled in the browser;
  - a service worker decodes them with the Go VM compiled to WebAssembly.
- **WebAssembly**: `.wasm` served as `application/wasm`; the VM, decoder and
  generator in the browser (service worker and playground); WASI modules as
  sandboxed handlers ([wasi.md](wasi.md)).
- **Operations**:
  - Prometheus metrics on a private listener;
  - `-log-file`;
  - a **native Windows service** (`-service install`);
  - systemd, launchd, FreeBSD rc.d and Kubernetes examples.
- **Distribution**:
  - a Docker image and a compose demo of eight sites;
  - a release workflow (tag → GitHub Release with archives for 8 platforms,
    and a multi-arch image on ghcr.io);
  - CI on Linux, Windows and macOS.
- **Docs** for installing on every OS, hosting, and generation
  ([docs/README.md](README.md)).

## Ideas

- **Signed binaries.** Code-sign the Windows executables and notarize the
  macOS ones, so SmartScreen and Gatekeeper stop warning.
- **Package managers.** Homebrew tap, Scoop/winget manifests, a Debian/RPM
  package (for example with nfpm).
- **Faster binary compilation.** Sweep chunks compile at about 1 MB/s; caching
  reachability per region sweep could raise that.
- **Smaller tiny files.** A sweep chunk needs a 320-cell region, so a one-byte
  file costs about 420 cells; a dedicated small-file mode could help.
- **HTTP/3** via a QUIC library (this would be MelHttp's first network
  dependency).
