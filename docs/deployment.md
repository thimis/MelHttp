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
| `-healthcheck` | — | | probe `-addr` and exit 0/1 (for container health checks) |

`GET /healthz` returns `200 ok`.

## Docker

```bash
docker compose up --build -d --wait      # six demo sites on ports 8080-8085
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

Native TLS is the next roadmap item (see [roadmap.md](roadmap.md)). Until
then, put melhttpd behind a TLS-terminating proxy such as Caddy, nginx or a
cloud load balancer. For example, Caddy obtains Let's Encrypt certificates
automatically:

```
example.com {
    reverse_proxy 127.0.0.1:8080
}
```
