# MelHttp documentation

## Using MelHttp

| Guide | What's in it |
|---|---|
| [install.md](install.md) | Installing on Linux, Windows, macOS and FreeBSD (binaries, Docker, from source) |
| [generating.md](generating.md) | Compiling sites and files into Malbolge with `melc`: build, watch, gen, run, check |
| [frameworks.md](frameworks.md) | Angular, React, Vue, Svelte, Next, Nuxt, Astro, Gatsby, Hugo, Jekyll presets |
| [hosting.md](hosting.md) | Putting a site online: Linux/systemd, Docker, reverse proxies, Windows service, macOS, FreeBSD, Kubernetes, monitoring |
| [deployment.md](deployment.md) | Every `melhttpd` flag and environment variable, and HTTPS |
| [transport.md](transport.md) | The Malbolge transport (obfuscation, not encryption) and the in-browser playground |
| [wasi.md](wasi.md) | WebAssembly (WASI) modules as sandboxed handlers |

## How it works

| Document | What's in it |
|---|---|
| [architecture.md](architecture.md) | Packages, URL mapping, caching, limits |
| [malbolge-vm.md](malbolge-vm.md) | The VM and its fidelity to the 1998 reference interpreter |
| [generator.md](generator.md) | How bytes become Malbolge: lag-1, tape and sweep chunks, and the completeness proofs |
| [melcgi.md](melcgi.md) | The MelCGI/1.0 contract between server and programs |
| [security.md](security.md) | Threat model, sandboxing, hardening |
| [performance.md](performance.md) | Benchmarks and tuning |
| [testing.md](testing.md) | The goal ladder (G1–G15) and every test layer |
| [roadmap.md](roadmap.md) | What's done and what could come next |
