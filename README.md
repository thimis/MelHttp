# MelHttp

A web server where **every byte you serve is produced by a [Malbolge](https://en.wikipedia.org/wiki/Malbolge) program**.

> Work in progress — see [docs/roadmap.md](docs/roadmap.md).

## How it works

1. `melc` compiles your site (any file — HTML, JS, CSS, fonts, images) into Malbolge programs that print it.
2. `melhttpd` runs those programs in a sandboxed, in-process Malbolge VM through **MelCGI**, a safe CGI-style contract.
3. Deterministic pages are executed once and cached, so serving is about as fast as a plain static server.

Details: [docs/architecture.md](docs/architecture.md).
