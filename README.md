# MelHttp

A web server where **every byte you serve is printed by a [Malbolge](https://en.wikipedia.org/wiki/Malbolge) program**, the esoteric language designed in 1998 to be the hardest to program in.

It serves real sites: the repository includes an Angular Material app, React and Vue apps, and a classic multi-page site. Their HTML, JavaScript, CSS, fonts and images are all compiled to Malbolge and run in a sandboxed VM. In browser tests they work exactly like the originals, and cached pages are served faster than Go's own static file server.

## How it works

1. **`melc build`** compiles every file of your site, binary files included, into Malbolge programs that print it. Each program first prints a small CGI-style header. Large files become several programs ("chunks"), and every program is run once to verify it before it is written. → [generator](docs/generator.md)
2. **`melhttpd`** maps URLs to those programs and runs them in an in-process Malbolge VM that matches the original 1998 interpreter. The VM is a perfect sandbox: a program can only read stdin and write stdout. → [VM](docs/malbolge-vm.md)
3. Programs talk to the server through **MelCGI**, a strict CGI-like contract: the request goes in on stdin, and the response comes back on stdout. → [MelCGI](docs/melcgi.md)
4. Programs that don't read their input run once at startup and are then served from memory with ETags and gzip. → [architecture](docs/architecture.md), [performance](docs/performance.md)

Also included:

- **HTTPS:** automatic Let's Encrypt certificates.
- **Malbolge transport:** pages travel to the browser *as Malbolge programs*, and the browser runs them.
- **Playground:** an in-browser Malbolge playground.
- **WASI handlers:** WebAssembly modules as sandboxed handlers.
- **Operations:** `melc watch` for development, Prometheus metrics, and a native Windows service.

## Setup

Single executables with no dependencies, for **Linux, Windows, macOS and FreeBSD** on **x86-64 and ARM64**.

| Method | |
|---|---|
| **Prebuilt binaries** | Download the archive for your platform from the [releases](https://github.com/thimis/MelHttp/releases), unpack it, and put `melc` and `melhttpd` on your `PATH`. |
| **Docker** | `docker compose up --build -d --wait` starts eight demo sites (ports 8080–8087). After a release: `docker run -p 8080:8080 ghcr.io/thimis/melhttp`. |
| **From source** (Go 1.26+) | `git clone https://github.com/thimis/MelHttp && cd MelHttp && go generate ./internal/webvm && go build -o bin/ ./cmd/melc ./cmd/melhttpd` |

**Step-by-step guides for every OS:** [docs/install.md](docs/install.md). **Putting a site online** (Linux server, Docker, Windows service, macOS, FreeBSD, reverse proxies, Kubernetes): [docs/hosting.md](docs/hosting.md).

## Usage

```bash
melc build -o site ./my-site                  # compile a folder of static files to Malbolge
melhttpd -root site                           # serve it on http://localhost:8080
melc watch -serve :8080 ./my-site             # or: rebuild and serve on every change
```

For HTTPS, add your domain; ports 80 and 443 must reach the server:

```bash
melhttpd -root site -addr :80 -tls-addr :443 -acme-domains example.com -acme-email you@example.com
```

Every response from a program carries `X-Powered-By: Malbolge` and `X-Malbolge-Steps: <instructions executed>`.

| Flag | Default | Meaning |
|---|---|---|
| `-root` / `-addr` | `site` / `:8080` | site directory, listen address |
| `-acme-domains`, `-tls-cert`/`-tls-key`, `-tls-self-signed` | off | HTTPS on `-tls-addr` (`:8443`); plain HTTP then redirects; `-hsts 8760h` adds HSTS |
| `-spa` | off | serve `index.html` for unknown paths (usually set by `melc build`) |
| `-obfuscate` / `-playground` | off | Malbolge transport for browsers / in-browser playground → [transport](docs/transport.md) |
| `-wasi` | off | run `*.wasi` WebAssembly handlers → [WASI](docs/wasi.md) |
| `-metrics-addr` | off | Prometheus metrics on a private address |
| `-service install` | | Windows: install as a service → [Windows](deploy/windows.md) |

Every flag also has a `MELHTTP_*` environment variable. The full list is in [docs/deployment.md](docs/deployment.md).

## Compiling `.mb` files

```bash
melc build --preset auto --run-build -o site ./my-app   # Angular, React, Vue, Svelte, Next, Nuxt, Astro, Gatsby, Hugo, Jekyll…
melc gen page.html                                      # one file → page.html.mb (a chunk directory if large)
melc gen -raw -o hello.mb hello.txt                     # no CGI header: the program prints only the file
melc run hello.mb                                       # run any Malbolge program (stdin → stdout)
melc check site/index.html.mb                           # validate without running
```

On disk, `about.html.mb`, or a chunk directory `about.html.mb/000.mb, 001.mb, …`, serves `/about.html`. Hand-written programs that print only a body are named `*.raw.mb`.

Output costs about 7.5 cells per byte for text and 8–11 for binary or UTF-8. Rebuilds are incremental and happen in place, even while the site is served.

**Full guide:** [docs/generating.md](docs/generating.md) (frameworks: [docs/frameworks.md](docs/frameworks.md)).

## How it's tested

The tests were written first, as a ladder of 15 goals that build up to the full product, and every goal passes. They include:

- a differential test of the VM against the original C interpreter;
- completeness proofs for the converter;
- byte-for-byte crawls of every test site;
- Playwright tests of the Angular, React and Vue apps and of the Malbolge transport in Chromium;
- a real Let's Encrypt-style certificate from an ACME test CA;
- WASI handlers, a Windows service, Docker;
- fuzzing, the race detector, fault injection, and cross-builds for 8 platforms and WebAssembly.

See [docs/testing.md](docs/testing.md) and [docs/security.md](docs/security.md). All documentation: [docs/](docs/README.md).

## Credits

- Malbolge was created by Ben Olmstead (1998); his reference interpreter is public domain.
- The golden test programs are by Matthias Lutter and others ([credits](testdata/programs/SOURCES.md)).
- The generator's lag-1 technique follows ideas from zb3's malbolge-tools.
