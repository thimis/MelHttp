# MelHttp

A web server where **every byte you serve is printed by a [Malbolge](https://en.wikipedia.org/wiki/Malbolge) program**, the esoteric language designed in 1998 to be the hardest to program in.

It serves real sites: the repository includes an Angular Material app, React and Vue apps, and a classic multi-page site. Their HTML, JavaScript, CSS, fonts and images are all compiled to Malbolge and run in a sandboxed VM. In browser tests they work exactly like the originals, and cached pages are served faster than Go's own static file server.

> **Status: alpha (0.1.0).** It works and is heavily tested, but flags, the site layout and the MelCGI contract may still change before 1.0. See the [changelog](CHANGELOG.md).

## Quick start: the Angular showcase

Serve the included Angular Material app, with every byte printed by Malbolge, in one of two ways.

**With Docker** (nothing else to install):

```bash
git clone https://github.com/thimis/MelHttp.git
cd MelHttp
docker compose up --build -d --wait angular     # the first build takes a few minutes
```

Open http://localhost:8080. Stop it with `docker compose down`.

**With Go and Node.js** ([Go](https://go.dev/dl/) 1.26+ and [Node.js](https://nodejs.org/) LTS; on Windows: `winget install GoLang.Go` and `winget install OpenJS.NodeJS.LTS`, then open a new terminal):

```powershell
git clone https://github.com/thimis/MelHttp.git
cd MelHttp
.\scripts\serve.ps1 angular -Open               # Windows
```

```bash
scripts/serve.sh angular --open                 # Linux, macOS, Git Bash
```

The first run installs the app's packages, builds it, compiles every file to Malbolge and opens http://localhost:8080. Later runs start in seconds. Stop it with Ctrl+C.

**Then try:**

- **Malbolge corner** (in the app's menu): see the Malbolge program behind the page.
- **Malbolge transport:** add `-Obfuscate` (`--obfuscate`) to the serve command. With Docker, run `docker compose up -d --wait angular-transport` and open http://localhost:8088; stop it with `docker compose --profile transport down`.
  Reload the page once. From then on, every file reaches the browser as a Malbolge program and is decoded there. In DevTools → Network, the rows with a ⚙ icon show the Malbolge on the wire.

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

**Step-by-step guides for every OS:** [docs/install.md](docs/install.md).

## Try it in one minute

From a clone of the repository, two scripts do the work: PowerShell for Windows, bash for Linux, macOS and Git Bash. They build `melc` and `melhttpd` themselves, so all you need is [Go](https://go.dev/dl/).

```powershell
.\scripts\serve.ps1 classic -Open        # Windows
```

```bash
scripts/serve.sh classic --open          # Linux, macOS, Git Bash
```

That opens a demo site whose every byte came out of the Malbolge VM. Try `curl -I http://localhost:8080/` (`curl.exe` in PowerShell) to see `X-Powered-By: Malbolge` and `X-Malbolge-Steps: …`.

Other demos to try instead of `classic`:

| Demo | What it shows |
|---|---|
| `hello` | a single page |
| `cgi` | programs that read the request |
| `angular`, `react`, `vue` | real framework apps (need Node.js; built on first use) |
| `transport` | pages travel as Malbolge programs to the browser; plus the playground |
| `wasi` | a Go program compiled to WebAssembly |

Add `-Obfuscate` (`--obfuscate`) to send any site over the Malbolge transport, for example `.\scripts\serve.ps1 angular -Obfuscate -Open`. Reload the page once, and from then on every page and file reaches the browser as a Malbolge program.

`.\scripts\serve.ps1 -Docker` (`scripts/serve.sh --docker`) starts all eight demo sites in Docker.

## Build your own app

The PowerShell commands are shown. With bash, use `scripts/serve.sh` and lowercase options with two dashes (`--watch`, `--open`).

### 1. A website from plain files

Make a folder with an `index.html`, plus any CSS, JavaScript, images or fonts, in any sub-folders. Then:

```powershell
.\scripts\serve.ps1 C:\my-site -Watch -Open
```

Your site opens at http://localhost:8080. Every time you save a file, it is recompiled to Malbolge and the next refresh shows the change. Stop with **Ctrl+C**.

Without the scripts, using installed binaries:

```bash
melc watch -serve :8080 ./my-site     # develop: rebuild and serve on every change
melc build -o site ./my-site          # or build once...
melhttpd -root site                   # ...and serve
```

### 2. A React, Vue, Svelte, Angular… app

Create or use any app that builds to static files (Node.js required):

```bash
npm create vite@latest my-app -- --template react-ts --no-interactive   # or: vue-ts, svelte-ts, …
```

Then point the script at the project folder:

```powershell
.\scripts\serve.ps1 C:\path\to\my-app -Open
```

MelHttp detects the framework from `package.json`, installs dependencies, runs `npm run build`, compiles the output to Malbolge, and turns on single-page-app routing so deep links like `/settings` work.

- **After changing the app:** add `-Rebuild` to build it again.
- **Without the script:** `melc build --preset auto --run-build -o site ./my-app`, then `melhttpd -root site`.
- **Supported frameworks:** Angular, Vite (React, Vue, Svelte, Solid, Preact, Lit), Create React App, Next (static export), Nuxt, Astro, Gatsby, Hugo, Jekyll. → [docs/frameworks.md](docs/frameworks.md)

### 3. Dynamic pages

Compiled files are static. For pages that react to the request, add a **WASI handler**: a small program in any language that compiles to WebAssembly. For example, in Go:

```go
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	vars := map[string]string{}
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() && in.Text() != "" { // the request: NAME=value lines, a blank line, then the body
		k, v, _ := strings.Cut(in.Text(), "=")
		vars[k] = v
	}
	fmt.Print("Content-Type: text/plain\n\n") // the response: headers, a blank line, the body
	fmt.Printf("Hello %s! It is %s.\n", vars["REMOTE_ADDR"], time.Now().Format(time.Kitchen))
}
```

Compile it into your site and serve with `-wasi`:

```bash
GOOS=wasip1 GOARCH=wasm go build -o my-site/api/hello.txt.wasi .   # PowerShell: $env:GOOS="wasip1"; $env:GOARCH="wasm"; go build …
melc build -o site my-site && melhttpd -root site -wasi              # http://localhost:8080/api/hello.txt
```

- **Sandbox:** handlers get no files, network or environment; memory and time are capped. → [docs/wasi.md](docs/wasi.md)
- **Pure Malbolge:** hand-written programs work too, as `name.ext.mb` (MelCGI headers and body) or `name.ext.raw.mb` (body only). → [docs/melcgi.md](docs/melcgi.md)

### 4. Put it online

Build the site, copy the `site` folder to your server, and start `melhttpd` with your domain for free, automatic HTTPS (ports 80 and 443 must reach the server):

```bash
melc build -o site ./my-site
melhttpd -root site -addr :80 -tls-addr :443 -acme-domains example.com -acme-email you@example.com
```

Rebuilding into the same `site` folder updates the live site in place, with no restart.

For running it permanently, [docs/hosting.md](docs/hosting.md) walks through:

- a Linux server with systemd;
- Docker Compose;
- a Windows service (`melhttpd -service install …`);
- macOS and FreeBSD;
- Caddy or nginx in front;
- Kubernetes.

## Run the tests

```powershell
.\scripts\test.ps1 -Quick     # Windows        (scripts/test.sh --quick on Linux/macOS/Git Bash)
```

Every run ends with a summary table, one PASS, FAIL or SKIP line per stage:

| Command | What runs | Time | Needs |
|---|---|---|---|
| `test.ps1 -Quick` | build, all unit tests, a Hello World smoke test | about 1 min | Go |
| `test.ps1` | the above, plus the acceptance goals G1–G15 (real binaries, real sites, real browsers) | a few min | Go, Node.js |
| `test.ps1 -Full` | the above, plus Docker and Let's Encrypt (ACME) goals, the 1998 reference interpreter, and the race detector | about 10 min | Go, Node.js, Docker running |

The bash equivalents are `scripts/test.sh --quick`, `scripts/test.sh` and `scripts/test.sh --full`.

- **Single goal:** `go test -tags acceptance -run G6 -v ./acceptance` runs just the Angular goal. The full list of goals is in [docs/testing.md](docs/testing.md).
- **Windows service goal (G15):** it is skipped unless you run from an Administrator PowerShell.
- **The browser goals** download Playwright's Chromium on first use.
- **If something fails:**
  - "port already in use": another server is running; stop it.
  - "Docker is not running": start Docker Desktop.
  - "npm is required": install Node.js LTS.

The failure output above the summary names the test and shows the details.

What the tests cover:

- the VM, differential-tested against the original 1998 interpreter;
- the converter, with completeness proofs that every byte value can be printed;
- every test site crawled byte for byte;
- Playwright browser tests of the Angular, React and Vue apps and the Malbolge transport;
- a certificate from an ACME test CA, WASI handlers, the Windows service and Docker;
- fuzzing, fault injection, the race detector, and builds for 8 platforms and WebAssembly.

More in [docs/testing.md](docs/testing.md) and [docs/security.md](docs/security.md).

## Reference

**`melhttpd` flags.** Every flag also has a `MELHTTP_*` environment variable; the full list is in [docs/deployment.md](docs/deployment.md).

| Flag | Default | Meaning |
|---|---|---|
| `-root` / `-addr` | `site` / `:8080` | site directory, listen address |
| `-acme-domains`, `-tls-cert`/`-tls-key`, `-tls-self-signed` | off | HTTPS on `-tls-addr` (`:8443`); plain HTTP then redirects; `-hsts 8760h` adds HSTS |
| `-spa` | off | serve `index.html` for unknown paths (usually set by `melc build`) |
| `-obfuscate` / `-playground` | off | Malbolge transport for browsers / in-browser playground → [transport](docs/transport.md) |
| `-obfuscate-inject` | off | `-obfuscate` for any site, unchanged: adds the transport script to every HTML page |
| `-wasi` | off | run `*.wasi` WebAssembly handlers → [WASI](docs/wasi.md) |
| `-metrics-addr` | off | Prometheus metrics on a private address |
| `-service install` | | Windows: install as a service → [Windows](deploy/windows.md) |

**`melc` commands.** → [docs/generating.md](docs/generating.md)

```bash
melc build [-o site] [--preset auto] [--run-build] <dir>   # a whole site or framework app
melc watch [-o site] [-serve :8080] <dir>                  # rebuild (and serve) on every change
melc gen page.html                                         # one file → page.html.mb
melc run hello.mb                                          # run any Malbolge program (stdin → stdout)
melc check site/index.html.mb                              # validate without running
```

**On disk:**

- `about.html.mb`, or a chunk directory `about.html.mb/000.mb, 001.mb, …`, serves `/about.html`;
- `*.raw.mb` programs print only a body;
- `*.wasi` files are WebAssembly handlers.

**Output size:** text costs about 7.5 cells per byte, and binary or UTF-8 about 8–11.

All documentation: [docs/](docs/README.md).

## Credits

- Malbolge was created by Ben Olmstead (1998); his reference interpreter is public domain.
- The golden test programs are by Matthias Lutter and others ([credits](testdata/programs/SOURCES.md)).
- The generator's lag-1 technique follows ideas from zb3's malbolge-tools.
