# Generating Malbolge: compiling sites and files

`melc` turns anything into Malbolge programs that print it. This guide covers
everyday use; [generator.md](generator.md) explains how it works inside.

```
melc build  [-o out] [--preset name] [--run-build] [--spa=true|false] [-seed N] [-v] <dir>
melc watch  [-o out] [--preset name] [-serve :8080] [-interval 500ms] <dir>
melc gen    [-o out.mb] [-raw] [-type mime] [-seed N] [-stats] <file>
melc run    [-steps N] [-stats] <program.mb | chunk-dir.mb>
melc check  <program.mb | chunk-dir.mb>...
```

## Compile a whole site: `melc build`

```bash
melc build -o site ./public                 # any folder of HTML/CSS/JS/images/fonts/...
melhttpd -root site
```

`melc build` handles each kind of source file as follows:

| Source file | Output |
|---|---|
| any file `x` (HTML, JS, CSS, JSON, SVG, PNG, JPEG, fonts, `.wasm`, …) | `x.mb`: a program printing a MelCGI header (Content-Type from the extension) and then the file's exact bytes. Large files become a chunk directory `x.mb/000.mb, 001.mb, …`. |
| `x.mb`, `x.raw.mb`, `x.mb/` | copied as-is (hand-written Malbolge; see below) |
| `x.wasi`, `x.raw.wasi` | copied as-is after checking it is WebAssembly (WASI handlers; see below) |
| `melhttp.json` | merged into the generated site config |
| dotfiles and dot-directories | skipped, except `.well-known/` |

Every generated program is **run before it is written** and compared
byte-for-byte with its source. A mismatch stops the build; it has never
happened, and it would be a bug worth reporting.

**Safety.**

- `-o` must not be inside the source, or contain it.
- melc only replaces directories it created itself; they carry a
  `.melc-build` marker.
- File names that differ only in case are rejected, because they would
  collide on Windows and macOS.

**Rebuilds are incremental and in place.** Only changed files are
recompiled. Unchanged outputs are left untouched, so a running melhttpd keeps
their cached responses. You can rebuild while the site is being served, on
every OS.

## Framework apps: presets

Frameworks put their static build somewhere specific, and single-page apps
need SPA fallback. Presets handle both:

```bash
melc build --preset auto --run-build -o site ./my-app   # detect, npm ci if needed, npm run build, compile
melc build --preset angular -o site ./my-angular-app      # compile an existing build
```

| Preset | Output it compiles | SPA |
|---|---|---|
| `angular` | `outputPath` from angular.json, then `/browser` | yes |
| `vite`, also `react`, `vue`, `svelte`, `solid`, `preact`, `lit` | `dist/` | yes |
| `cra` (Create React App) | `build/` | yes |
| `next` (with `output: 'export'`) | `out/` | no |
| `nuxt` (`nuxt generate`) | `.output/public/` | no |
| `astro` | `dist/` | no |
| `gatsby`, `hugo` | `public/` | no |
| `jekyll` | `_site/` | no |
| `static` | the directory itself | no |

`--spa=true|false` overrides the preset. More detail is in
[frameworks.md](frameworks.md).

## Develop with live rebuilds: `melc watch`

```bash
melc watch -serve :8080 ./my-site            # edit files; refresh the browser
```

- `melc watch` rebuilds whenever a file changes, polling every
  `-interval` (default 500 ms).
- With `-serve`, it also serves the output, and changes show up on the next
  request.
- For framework apps, run the framework's own watcher (`ng build --watch`,
  `vite build --watch`) next to `melc watch --preset …`, which watches the
  build output.

## One file: `melc gen`

```bash
melc gen page.html                       # → page.html.mb (or a page.html.mb/ chunk directory)
melc gen -o site/about.html.mb about.html
melc gen -type application/ld+json data.jsonld
melc gen -raw -o hello.mb hello.txt      # no header: the program prints only the file
melc gen -raw -o - hello.txt > hello.mb  # "-": write to stdout (single programs only)
melc gen -stats logo.png                 # report program count and size
```

`-seed N` produces a different program with the same output, with random
padding and random instructions in the cells that never execute.

## Run and check programs

```bash
melc run hello.mb                        # any Malbolge program, stdin → stdout
echo "Hi" | melc run -stats cat.mb       # -stats: instructions, bytes, whether it read input
melc run -steps 1000000 loop.mb          # stop runaway programs (exit status 3)
melc check site/index.html.mb            # validate without running
```

Exit statuses:

| Status | Meaning |
|---|---|
| 0 | halted normally |
| 1 | error |
| 2 | usage |
| 3 | step limit reached |
| 4 | reached an invalid instruction |

## How big and how fast

| Content | Malbolge per byte | Example |
|---|---|---|
| ASCII text (HTML, JS, CSS, JSON) | ~7.1–7.6 cells | Angular's 270 KB main bundle → ~2 MB |
| Binary, UTF-8 with accents, UTF-16 | ~8–11 cells | a 128 KB woff2 icon font → ~1.1 MB |
| Tiny files (a few bytes) | a few hundred cells minimum | |

- **Program size:** one program holds at most 59049 cells (about 7 KB of
  output), so bigger files are split into chunks automatically.
- **Compile speed:** about 12 MB/s for ASCII and about 1 MB/s for binary data,
  spread across all CPU cores. The 1.34 MB Angular showcase compiles in
  0.3 s.
- **Serving:** melhttpd runs each page once at startup and then serves it
  from memory, so program size does not affect serving speed.

## Hand-written Malbolge

Write MelCGI programs yourself, if you dare:

- **Name:** `page.html.mb` serves `/page.html`.
- **Input:** a block of `NAME=value` lines (method, path, query, headers),
  then a blank line, then the request body.
- **Output:** a CGI header block (`Content-Type: …`, optional `Status: …`), a
  blank line, then the body.
- **Validation:** melhttpd parses the header block strictly; a malformed
  block becomes a 502.

The full contract is in [melcgi.md](melcgi.md). If your program doesn't print
headers, name it `page.html.raw.mb`. Its whole output is then the body, and
the type comes from `.html`.

Programs that never read stdin are cached after their first run. Programs
that do read it run on every request with the request on stdin, for example
the echo demo in `testsites/cgi/`.

## WASI handlers (WebAssembly)

For real logic, write a handler in any language that compiles to WASI and
save it as `page.html.wasi`:

```bash
GOOS=wasip1 GOARCH=wasm go build -o my-site/api/hello.html.wasi ./handler
melc build -o site my-site && melhttpd -root site -wasi
```

It uses the same MelCGI contract as Malbolge programs and runs in a sandbox.
See [wasi.md](wasi.md).

## Troubleshooting

| Message | Fix |
|---|---|
| `refusing to replace … : it exists and was not created by melc build` | choose an empty or new `-o` directory |
| `… differ only in case` | rename one of the files |
| `… preset: no build output found` | build the app first, or add `--run-build` |
| `self-check failed (please report this bug)` | please open an issue with the file that triggered it |
| a page returns 502 | a hand-written program printed invalid MelCGI headers; run `melc run page.mb` to see its output |
