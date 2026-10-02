# Deployment

`melhttpd` and `melc` are single static binaries (pure Go, no cgo). They run
on Linux, Windows, macOS and FreeBSD, on amd64 and arm64.

## Configuration

Every flag can also be set with an environment variable: `MELHTTP_` plus the
flag name in upper case, with `-` replaced by `_`. Flags win over the
environment.

| Flag | Env | Default | |
|---|---|---|---|
| `-addr` | `MELHTTP_ADDR` | `:8080` | listen address |
| `-root` | `MELHTTP_ROOT` | `site` | directory built by `melc build` |
| `-spa` | `MELHTTP_SPA` | false | SPA fallback (also enabled by `melhttp.json`) |
| `-expose-source` | `MELHTTP_EXPOSE_SOURCE` | false | serve program source under `/_source/` |
| `-no-cache` | `MELHTTP_NO_CACHE` | false | run the VM on every request |
| `-warm` | `MELHTTP_WARM` | true | run every program at startup |
| `-max-steps` | `MELHTTP_MAX_STEPS` | 2000000000 | instructions per request |
| `-max-output` | `MELHTTP_MAX_OUTPUT` | 268435456 | output bytes per request |
| `-timeout` | `MELHTTP_TIMEOUT` | 30s | wall time per program run |
| `-max-body` | `MELHTTP_MAX_BODY` | 1048576 | request body bytes passed to programs |
| `-concurrency` | `MELHTTP_CONCURRENCY` | 2×CPUs | simultaneous VM runs |
| `-cache-mb` | `MELHTTP_CACHE_MB` | 512 | response cache size |
| `-allow-sensitive-headers` | `MELHTTP_ALLOW_SENSITIVE_HEADERS` | false | pass Cookie/Authorization to programs |
| `-log-format` | `MELHTTP_LOG_FORMAT` | text | `text` or `json` |
| `-obfuscate` | `MELHTTP_OBFUSCATE` | false | Malbolge transport for browsers that opt in ([transport.md](transport.md)) |
| `-obfuscate-variants` | `MELHTTP_OBFUSCATE_VARIANTS` | 2 | encodings kept per page |
| `-playground` | `MELHTTP_PLAYGROUND` | false | in-browser playground at `/_melhttp/playground.html` |
| `-wasi` | `MELHTTP_WASI` | false | run `*.wasi` WebAssembly handlers ([wasi.md](wasi.md)) |
| `-wasi-memory-mb` | `MELHTTP_WASI_MEMORY_MB` | 64 | memory per WASI run |
| `-hsts` | `MELHTTP_HSTS` | 0 | HSTS max-age on HTTPS responses |
| `-metrics-addr` | `MELHTTP_METRICS_ADDR` | off | Prometheus metrics at `/metrics` on a **private** address, e.g. `127.0.0.1:9090` |
| `-healthcheck` | — | | probe `-addr` and exit 0/1 (for container health checks) |

`GET /healthz` returns `200 ok`.

## Docker

```bash
docker compose up --build -d --wait      # eight demo sites on ports 8080-8087
docker build -t melhttp .                # just the image (≈53 MB, distroless, non-root)
docker run --rm -p 8080:8080 -e MELHTTP_ROOT=/srv/sites/classic melhttp
```

To serve your own site, compile it with `melc` (installed locally, or with
`docker compose run --rm melc build -o /work/site /work/my-site`) and mount
it:

```bash
docker run --rm -p 8080:8080 -v "$PWD/site:/srv/site:ro" -e MELHTTP_ROOT=/srv/site melhttp
```

Multi-arch images: `docker buildx build --platform linux/amd64,linux/arm64 -t you/melhttp --push .`

## Linux (systemd)

See [`deploy/melhttpd.service`](../deploy/melhttpd.service). It is a hardened
unit with a dedicated user, read-only paths and a minimal capability set.

## macOS (launchd)

See [`deploy/com.melhttp.melhttpd.plist`](../deploy/com.melhttp.melhttpd.plist).

## Windows

See [`deploy/windows.md`](../deploy/windows.md) for Task Scheduler, NSSM and
the firewall rule.

## Release archives

```bash
go run ./tools/dist            # dist/melhttp_<version>_<os>_<arch>.{tar.gz,zip} + SHA256SUMS
```

## HTTPS

HTTPS is enabled by exactly one of three certificate sources. The plain HTTP
listener on `-addr` then answers `/healthz` and ACME challenges, and redirects
everything else to HTTPS (`308`, path and query kept; turn off with
`-https-redirect=false`). HTTPS speaks HTTP/2 and accepts TLS 1.2+ (`-tls-min 1.3`
to require 1.3). `-hsts 8760h` adds `Strict-Transport-Security` to HTTPS responses.

| Mode | Flags |
|---|---|
| Automatic Let's Encrypt certificates | `-acme-domains example.com,www.example.com -acme-email you@example.com` |
| Certificate files (reloaded automatically when renewed) | `-tls-cert fullchain.pem -tls-key privkey.pem` |
| Throwaway self-signed certificate (development) | `-tls-self-signed` |

| Flag | Env | Default | |
|---|---|---|---|
| `-tls-addr` | `MELHTTP_TLS_ADDR` | `:8443` | HTTPS listen address |
| `-https-port` | `MELHTTP_HTTPS_PORT` | the `-tls-addr` port | public HTTPS port used in redirects (443 is omitted) |
| `-acme-cache` | `MELHTTP_ACME_CACHE` | `autocert-cache` | where certificates and the account key are kept; **keep it private and persistent** |
| `-acme-directory` | `MELHTTP_ACME_DIRECTORY` | Let's Encrypt | any RFC 8555 CA (Let's Encrypt staging, ZeroSSL, step-ca, …) |
| `-acme-ca-root` | `MELHTTP_ACME_CA_ROOT` | | extra root (PEM) to trust for that directory |

**Let's Encrypt requirements.** DNS for every domain must point at the server, and
ports **80 and 443** must reach melhttpd. HTTP-01 challenges arrive on 80, and
TLS-ALPN-01 challenges on 443. Certificates are requested on the first HTTPS
request for a name and renewed automatically. Using `-acme-domains` accepts the
CA's terms of service.

**Low ports without root.** melhttpd listens on 8080/8443 by default. Map 80/443
to them (Docker `-p 80:8080 -p 443:8443`, or a firewall redirect), or give the
binary `CAP_NET_BIND_SERVICE` (see the systemd unit) and use `-addr :80 -tls-addr :443`.

**Docker.**

```bash
MELHTTP_DOMAIN=example.com MELHTTP_EMAIL=you@example.com docker compose --profile https up -d
```

That runs the `https` service: ports 80/443, HSTS, and certificates in the
`melhttp-certs` volume.

If you prefer a TLS-terminating proxy (Caddy, nginx, a cloud load balancer),
run melhttpd on plain HTTP behind it as before.
