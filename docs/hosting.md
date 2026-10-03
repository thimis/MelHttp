# Hosting

How to put a MelHttp site online. Every setup has the same three steps:

1. **Compile** your site: `melc build -o site ./my-site` (see [generating.md](generating.md)).
2. **Run** `melhttpd -root site` somewhere reachable.
3. **Add HTTPS**, either with melhttpd's own Let's Encrypt support or with a
   TLS proxy or platform in front of it.

| Setup | Best for | Section |
|---|---|---|
| Linux server with systemd | a VPS or home server, direct HTTPS | [Linux](#linux-server-systemd) |
| Docker / Docker Compose | any machine with Docker | [Docker](#docker-and-docker-compose) |
| Behind Caddy or nginx | sharing the server with other sites | [Reverse proxy](#behind-a-reverse-proxy) |
| Windows Server | Windows hosts | [Windows](#windows-server) |
| macOS | a Mac mini under the desk | [macOS](#macos) |
| FreeBSD | FreeBSD hosts | [FreeBSD](#freebsd) |
| Container platforms, Kubernetes | Fly.io, Render, Railway, Cloud Run, k8s | [Platforms](#container-platforms-and-kubernetes) |

All configuration is in [deployment.md](deployment.md). Every flag also has a
`MELHTTP_*` environment variable.

---

## Linux server (systemd)

These steps take a fresh Debian or Ubuntu VPS to HTTPS. Other distributions
differ only in the package and firewall commands.

1. **DNS.** Point an `A` record (and `AAAA` for IPv6) for `example.com` at the
   server.
2. **Install** the binaries ([install.md](install.md#linux)).
3. **Create a user and a home for the site:**

   ```bash
   sudo useradd --system --no-create-home --shell /usr/sbin/nologin melhttp
   sudo mkdir -p /srv/melhttp/site /srv/melhttp/certs
   sudo chown -R melhttp: /srv/melhttp/certs && sudo chmod 700 /srv/melhttp/certs
   ```

4. **Upload the site.** Compile locally and copy it:

   ```bash
   melc build -o site ./my-site && rsync -a --delete site/ server:/srv/melhttp/site/
   ```

   Or compile on the server: `melc build -o /srv/melhttp/site ./my-site`.
5. **Service.** Copy [deploy/melhttpd.service](../deploy/melhttpd.service) to
   `/etc/systemd/system/` and set the HTTPS flags in it:

   ```ini
   Environment=MELHTTP_ADDR=:80
   Environment=MELHTTP_TLS_ADDR=:443
   Environment=MELHTTP_ROOT=/srv/melhttp/site
   Environment=MELHTTP_ACME_DOMAINS=example.com,www.example.com
   Environment=MELHTTP_ACME_EMAIL=you@example.com
   Environment=MELHTTP_ACME_CACHE=/srv/melhttp/certs
   Environment=MELHTTP_HSTS=8760h
   ReadWritePaths=/srv/melhttp/certs
   ```

   The unit grants `CAP_NET_BIND_SERVICE`, so ports 80 and 443 work without
   root.
6. **Start it and open the firewall:**

   ```bash
   sudo systemctl daemon-reload && sudo systemctl enable --now melhttpd
   sudo ufw allow 80,443/tcp      # or firewall-cmd --add-service={http,https} --permanent
   journalctl -u melhttpd -f       # watch the warm-up and the first certificate
   ```

The certificate is requested on the first HTTPS request for each name and
renewed automatically. Port 80 stays open for ACME challenges and redirects
everything else to HTTPS.

## Docker and Docker Compose

```bash
# HTTPS with Let's Encrypt; ports 80/443 must reach this host
MELHTTP_DOMAIN=example.com MELHTTP_EMAIL=you@example.com docker compose --profile https up -d
```

The `https` service in [compose.yaml](../compose.yaml) serves the Angular demo
by default. To serve your own site, mount it and point `MELHTTP_ROOT` at it:

```yaml
  https:
    volumes:
      - melhttp-certs:/var/lib/melhttp/certs
      - ./site:/srv/site:ro
    environment:
      MELHTTP_ROOT: /srv/site
```

The image is distroless and runs as non-root, with a read-only root
filesystem and all capabilities dropped. It reports its health via
`melhttpd -healthcheck`.

## Behind a reverse proxy

Run melhttpd on plain HTTP on a local port and let the proxy handle TLS:

```bash
melhttpd -root /srv/melhttp/site -addr 127.0.0.1:8080
```

**Caddy** (gets certificates automatically):

```
example.com {
    reverse_proxy 127.0.0.1:8080
}
```

**nginx:**

```nginx
server {
    listen 443 ssl http2;
    server_name example.com;
    ssl_certificate     /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem;
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Behind a proxy, `REMOTE_ADDR` is the proxy's address. MelCGI programs receive
the client address as `HTTP_X_FORWARDED_FOR`.

## Windows Server

```powershell
# elevated PowerShell
melhttpd -service install -root C:\melhttp\site -addr :80 -tls-addr :443 `
  -acme-domains example.com -acme-email you@example.com -acme-cache C:\melhttp\certs -hsts 8760h
melhttpd -service start
New-NetFirewallRule -DisplayName "melhttpd" -Direction Inbound -Protocol TCP -LocalPort 80,443 -Action Allow
```

The service starts at boot, restarts after failures and logs to
`melhttpd.log` next to the executable. Details:
[deploy/windows.md](../deploy/windows.md).

## macOS

Use [deploy/com.melhttp.melhttpd.plist](../deploy/com.melhttp.melhttpd.plist)
with `launchctl bootstrap system …`. Edit its `EnvironmentVariables` the same
way as the systemd unit. Allow incoming connections in **System Settings →
Network → Firewall**.

## FreeBSD

```sh
install -m 0555 deploy/freebsd/melhttpd /usr/local/etc/rc.d/
sysrc melhttpd_enable=YES melhttpd_flags="-root /usr/local/www/melhttp -addr :8080"
service melhttpd start
```

## Container platforms and Kubernetes

Any platform that runs a container image works: Fly.io, Render, Railway,
Google Cloud Run, Azure Container Apps, AWS App Runner/ECS, or Kubernetes.

- **Image:** `ghcr.io/thimis/melhttp` after a tagged release, or one you build
  from the `Dockerfile` with your site compiled in. Replace the `sites` stage,
  or `COPY` your `melc build` output to `/srv/site` and set `MELHTTP_ROOT`.
- **Port:** 8080. The platform terminates TLS.
- **Health check:** `GET /healthz`.
- **State:** none. The site is read-only and caches are in memory, so scaling
  out is just more replicas.
- **Kubernetes:** a hardened example Deployment and Service are in
  [deploy/kubernetes.yaml](../deploy/kubernetes.yaml).

---

## Updating a live site

`melc build -o site ./my-site` updates an existing output directory **in
place**, even while melhttpd is serving it:

- unchanged pages keep their files and their cached responses;
- changed pages are swapped in;
- deleted pages disappear.

melhttpd notices within a second. No restart is needed.

During development, `melc watch -serve :8080 ./my-site` rebuilds on every
change and serves the result.

## Monitoring

- `GET /healthz` returns `200 ok`; `melhttpd -healthcheck` probes it (for
  Docker).
- `-metrics-addr 127.0.0.1:9090` serves Prometheus metrics at `/metrics` on
  a private address: requests by status, latency histogram, Malbolge
  instructions, cache hits, WASI runs, and more. Scrape configuration:

  ```yaml
  scrape_configs:
    - job_name: melhttp
      static_configs: [{ targets: ["127.0.0.1:9090"] }]
  ```

- Logs go to stderr (journald, Docker), or to a file with `-log-file`.
  `-log-format json` gives structured logs.

## Checklist before going public

- [ ] HTTPS on (`-acme-domains`, or a TLS proxy), and `-hsts` once you're sure.
- [ ] The ACME cache directory is private and persistent.
- [ ] `-metrics-addr` bound to localhost or a private network, never public.
- [ ] `-expose-source` only if you want visitors to read your Malbolge.
- [ ] `-wasi` only if you serve WASI handlers you trust.
- [ ] Limits reviewed: `-max-body`, `-timeout`, `-concurrency`, `-cache-mb`.
- [ ] See [security.md](security.md).
