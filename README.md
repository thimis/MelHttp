# MelHttp

A web server where **every byte you serve is printed by a [Malbolge](https://en.wikipedia.org/wiki/Malbolge) program**, the esoteric language designed in 1998 to be the hardest to program in.

It serves real sites: the repository includes an Angular Material app, React and Vue apps, and a classic multi-page site. Their HTML, JavaScript, CSS, fonts and images are all compiled to Malbolge and run in a sandboxed VM. In browser tests they work exactly like the originals, and the cached pages are served faster than Go's own static file server.

## How it works

1. **`melc build`** compiles every file of your site (including binary files) into Malbolge programs that print it. Each program first prints a small CGI-style header. Large files become several programs ("chunks"). Every program is run once to verify it before it is written. → [generator](docs/generator.md)
2. **`melhttpd`** maps URLs to those programs and runs them in an in-process Malbolge VM that matches the original 1998 interpreter. The VM is a perfect sandbox: a program can only read stdin and write stdout. → [VM](docs/malbolge-vm.md)
3. Programs talk to the server through **MelCGI**, a strict CGI-like contract: the request goes in on stdin, and the response comes back on stdout. → [MelCGI](docs/melcgi.md)
4. Programs that don't read their input are run once at startup and cached with ETags and gzip. Serving is then as fast as a normal static server. → [architecture](docs/architecture.md), [performance](docs/performance.md)

## Setup

### Option A: prebuilt binaries
Download the archive for your OS and CPU (Linux, Windows, macOS or FreeBSD; amd64 or arm64), unpack it, and put `melhttpd` and `melc` on your `PATH`.

### Option B: from source
Requires [Go 1.26+](https://go.dev/dl/) (`winget install GoLang.Go`, `brew install go`, or your package manager).

```bash
git clone https://github.com/thimis/MelHttp.git && cd MelHttp
go build -o bin/ ./cmd/melc ./cmd/melhttpd        # bin/melc, bin/melhttpd (.exe on Windows)
go run ./tools/dist                                # optional: release archives for all 8 platforms in dist/
```

### Option C: Docker
```bash
docker compose up --build -d --wait
```
This builds the image (about 53 MB, distroless, non-root) and starts six demo sites:

| Port | Site |
|---|---|
| 8080 | Angular |
| 8081 | classic |
| 8082 | MelCGI demos |
| 8083 | hello |
| 8084 | React |
| 8085 | Vue |

## Usage

```bash
melc build -o site ./my-site          # compile a folder of static files
melhttpd -root site -addr :8080       # serve it → http://localhost:8080
```

Every response from a program carries `X-Powered-By: Malbolge` and `X-Malbolge-Steps: <instructions executed>`.

Useful `melhttpd` flags (each can also be set with a `MELHTTP_*` environment variable):

| Flag | Default | Meaning |
|---|---|---|
| `-root` | `site` | site directory built by `melc build` |
| `-addr` | `:8080` | listen address |
| `-spa` | off | serve `index.html` for unknown paths (single-page apps; usually set by `melc build`) |
| `-expose-source` | off | let visitors read the Malbolge behind any page at `/_source/<path>` |
| `-no-cache` | off | run the VM on every request |
| `-max-steps`, `-timeout`, `-max-body` | 2e9, 30s, 1 MiB | per-request limits |
| `-healthcheck` | | exit 0 if a server on `-addr` is healthy (for Docker) |
| `-acme-domains`, `-tls-cert`/`-tls-key`, `-tls-self-signed` | off | enable HTTPS on `-tls-addr` (`:8443`) |

All flags, plus systemd, launchd and Windows setup: [docs/deployment.md](docs/deployment.md). **HTTPS:** `-acme-domains example.com` gets free, automatically renewed Let's Encrypt certificates (ports 80 and 443 must reach the server), `-tls-cert`/`-tls-key` serves your own, and `-tls-self-signed` is for local testing. Plain HTTP then redirects to HTTPS, and `-hsts 8760h` adds HSTS. Details: [docs/deployment.md#https](docs/deployment.md#https).

## Compiling `.mb` files

**A whole site.** `melc build` compiles a folder, or a framework project via presets:

```bash
melc build -o site ./public                                # any folder of static files
melc build --preset auto --run-build -o site ./my-app      # detect the framework, run its build, compile the output
melc build --preset angular -o site ./my-angular-app       # or name it: angular, vite, react, vue, svelte,
                                                           # cra, next, nuxt, astro, gatsby, hugo, jekyll, static
```

- **Presets** know where each framework writes its output and whether it needs SPA fallback. → [docs/frameworks.md](docs/frameworks.md)
- **Rebuilds** are incremental: only changed files are recompiled.
- **Safety:** `-o` is only ever replaced if melc created it.

**A single file.**

```bash
melc gen page.html                  # → page.html.mb (or a page.html.mb/ chunk directory if large)
melc gen -o site/about.html.mb about.html
melc gen -raw -o hello.mb hello.txt # no CGI header: the program prints only the file
melc gen -seed 42 page.html         # randomized code with the same output
```

**Running and checking programs.**

```bash
melc run hello.mb                   # run any Malbolge program (stdin → program → stdout)
echo "Hi" | melc run -stats cat.mb  # -stats prints instructions executed; -steps N sets a limit
melc check site/index.html.mb       # validate a program without running it
```

**Layout on disk.** The server derives each URL from the file name:

| On disk | URL |
|---|---|
| `about.html.mb` | `/about.html` |
| `about.html.mb/000.mb`, `001.mb`, … | `/about.html` |
| `echo.txt.raw.mb` | `/echo.txt` |

- **Chunk directories:** the programs run in order and their outputs are concatenated.
- **`.raw.mb`:** a raw program; its whole output is the body. Use this for hand-written Malbolge that reads the request.

## How it's tested

The tests were written first, as a ladder of goals that build up to the full product, and every goal passes. The goals cover:

- **VM:** a differential test against the original C interpreter.
- **Converter:** completeness proofs that every byte value can be printed.
- **End to end:** byte-for-byte crawls of every test site.
- **Browsers:** Playwright tests of the Angular, React and Vue apps served from Malbolge.
- **Hardening and builds:** fuzzing, the race detector, Docker, and cross-builds for 8 platforms and WebAssembly.

See [docs/testing.md](docs/testing.md) and [docs/security.md](docs/security.md).

## Credits

- Malbolge was created by Ben Olmstead (1998); his reference interpreter is public domain.
- The golden test programs are by Matthias Lutter and others ([credits](testdata/programs/SOURCES.md)).
- The generator's lag-1 technique follows ideas from zb3's malbolge-tools.
