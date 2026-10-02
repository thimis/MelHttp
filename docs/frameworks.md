# Frameworks

MelHttp serves any static website, so every framework that can produce a
static build works. `melc build --preset <name>` knows where each framework
puts its output and whether the site needs single-page-app (SPA) fallback.
With `--preset auto` (the default) melc detects the framework from the
project files.

```bash
melc build --preset auto --run-build -o site ./my-app   # build the app, then compile it to Malbolge
melhttpd -root site                                      # SPA mode comes from site/melhttp.json
```

`--run-build` runs `npm ci` when `node_modules` is missing, then the
framework's build command. Without it, melc compiles the existing build
output.

| Preset | Detected by | Build command | Output | SPA |
|---|---|---|---|---|
| `angular` | `angular.json` | `npm run build` | `outputPath` from angular.json, then `/browser` | yes |
| `vite` (aliases `react`, `vue`, `svelte`, `solid`, `preact`, `lit`) | `vite` or `@sveltejs/kit` in package.json | `npm run build` | `dist/` | yes |
| `cra` (Create React App) | `react-scripts` | `npm run build` | `build/` | yes |
| `next` | `next` | `npm run build` (needs `output: 'export'`) | `out/` | no |
| `nuxt` | `nuxt` | `npm run generate` | `.output/public/` or `dist/` | no |
| `astro` | `astro` | `npm run build` | `dist/` | no |
| `gatsby` | `gatsby` | `npm run build` | `public/` | no |
| `hugo` | `hugo.toml`/`hugo.yaml` | `hugo --minify` | `public/` | no |
| `jekyll` | `_config.yml` + `Gemfile` | `jekyll build` | `_site/` | no |
| `static` | anything else | — | the directory itself | no |

Override SPA mode with `--spa=true` or `--spa=false`.

## What happens to the files

**Every** file of the build output is compiled into a Malbolge program that
prints a MelCGI header (`Content-Type` from the extension) followed by the
file's exact bytes. That includes HTML, JS, CSS, source maps, SVG, PNG/JPEG,
WebP, fonts and `.wasm`. Large files become chunk directories. Every program
is run once at build time to check that its output matches the file before
anything is written.

`melc build` also writes:

- `melhttp.json`: site settings for melhttpd (`spa`, and anything in your own
  `melhttp.json` such as extra `headers` or `immutable` patterns);
- `.melc-manifest.json`: content hashes, so rebuilds only recompile changed
  files;
- `.melc-build`: a marker. melc only ever replaces directories that carry it.

## Tested frameworks

The repository contains real Angular 22, React (Vite) and Vue (Vite) apps in
`testsites/`. Goals G6 and G10 build each one with its preset, crawl every
file of the build output through melhttpd (byte-for-byte), check that deep
links reload, and run Playwright browser tests against the Malbolge-served
app.

## SPA fallback rules

With SPA mode on, a GET or HEAD for an unknown path whose last segment has no
dot (such as `/dashboard` or `/forms/step/2`) serves `/index.html` with status
200. Paths that look like files (`/missing.js`) still return 404, so broken
asset links stay visible.
