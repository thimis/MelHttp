# Installing MelHttp

MelHttp is two programs:

| Program | What it does |
|---|---|
| `melhttpd` | the web server |
| `melc` | the compiler and toolbox: turns sites into Malbolge, runs and checks programs, watches for changes |

Both are single, self-contained executables with no runtime dependencies.
They run on **Linux, Windows, macOS and FreeBSD**, on **x86-64 (amd64)** and
**ARM64**.

Choose one way to install them:

- [Prebuilt binaries](#prebuilt-binaries): download, unpack, done.
- [Docker](#docker): no install at all, with eight demo sites included.
- [From source](#from-source): needs Go 1.26 or newer.

Then [check the installation](#check-the-installation) and continue with
[generating.md](generating.md) (compiling sites) and [hosting.md](hosting.md)
(putting them online).

---

## Prebuilt binaries

Every tagged release on GitHub (`github.com/thimis/MelHttp/releases`) has one
archive per platform, plus `SHA256SUMS`:

| OS | x86-64 | ARM64 |
|---|---|---|
| Linux | `melhttp_<version>_linux_amd64.tar.gz` | `melhttp_<version>_linux_arm64.tar.gz` (Raspberry Pi 4/5 with a 64-bit OS, Graviton, Ampere) |
| Windows | `melhttp_<version>_windows_amd64.zip` | `melhttp_<version>_windows_arm64.zip` |
| macOS | `melhttp_<version>_darwin_amd64.tar.gz` (Intel) | `melhttp_<version>_darwin_arm64.tar.gz` (Apple silicon) |
| FreeBSD | `melhttp_<version>_freebsd_amd64.tar.gz` | `melhttp_<version>_freebsd_arm64.tar.gz` |

> No release published yet? Every CI run uploads the same archives as the
> `melhttp-dist` workflow artifact, or build them yourself with
> `go run ./tools/dist` (see [From source](#from-source)).

### Linux

```bash
VERSION=1.0.0; ARCH=amd64            # or arm64 (check with: uname -m → x86_64 / aarch64)
curl -LO https://github.com/thimis/MelHttp/releases/download/v$VERSION/melhttp_${VERSION}_linux_${ARCH}.tar.gz
curl -LO https://github.com/thimis/MelHttp/releases/download/v$VERSION/SHA256SUMS
sha256sum --check --ignore-missing SHA256SUMS
tar xzf melhttp_${VERSION}_linux_${ARCH}.tar.gz
sudo install -m 0755 melhttp_${VERSION}_linux_${ARCH}/melhttpd melhttp_${VERSION}_linux_${ARCH}/melc /usr/local/bin/
```

### macOS

```bash
VERSION=1.0.0; ARCH=arm64            # arm64 = Apple silicon, amd64 = Intel (check with: uname -m)
curl -LO https://github.com/thimis/MelHttp/releases/download/v$VERSION/melhttp_${VERSION}_darwin_${ARCH}.tar.gz
tar xzf melhttp_${VERSION}_darwin_${ARCH}.tar.gz
sudo install -m 0755 melhttp_${VERSION}_darwin_${ARCH}/{melhttpd,melc} /usr/local/bin/
# The binaries are not notarized; if macOS refuses to open them:
sudo xattr -d com.apple.quarantine /usr/local/bin/melhttpd /usr/local/bin/melc
```

### Windows

1. Download `melhttp_<version>_windows_amd64.zip`, or `…_arm64.zip` for
   Windows on ARM.
2. Unpack it to a folder such as `C:\melhttp`.
3. Add that folder to your `PATH`. In PowerShell:

   ```powershell
   Expand-Archive .\melhttp_1.0.0_windows_amd64.zip C:\melhttp
   Move-Item C:\melhttp\melhttp_1.0.0_windows_amd64\* C:\melhttp\
   [Environment]::SetEnvironmentVariable("Path", $env:Path + ";C:\melhttp", "User")   # then open a new terminal
   ```

4. Verify the download: `Get-FileHash .\melhttp_*.zip` should match the
   archive's line in `SHA256SUMS`.

The executables are not code-signed, so SmartScreen may warn the first time.
Choose **More info → Run anyway**.

To run as a Windows service, see [deploy/windows.md](../deploy/windows.md).

### FreeBSD

```sh
VERSION=1.0.0; ARCH=amd64            # or arm64
fetch https://github.com/thimis/MelHttp/releases/download/v$VERSION/melhttp_${VERSION}_freebsd_${ARCH}.tar.gz
tar xzf melhttp_${VERSION}_freebsd_${ARCH}.tar.gz
install -m 0755 melhttp_${VERSION}_freebsd_${ARCH}/melhttpd melhttp_${VERSION}_freebsd_${ARCH}/melc /usr/local/bin/
```

An `rc.d` script is in [deploy/freebsd/melhttpd](../deploy/freebsd/melhttpd).

---

## Docker

There is nothing to install besides Docker:

```bash
git clone https://github.com/thimis/MelHttp.git && cd MelHttp
docker compose up --build -d --wait      # eight demo sites on http://localhost:8080 … 8087
```

After a tagged release, the image is also on GitHub's container registry,
built for amd64 and arm64:

```bash
docker run --rm -p 8080:8080 ghcr.io/thimis/melhttp:latest                    # the Angular showcase
docker run --rm -p 8080:8080 -v "$PWD/site:/srv/site:ro" -e MELHTTP_ROOT=/srv/site ghcr.io/thimis/melhttp
```

The image also contains `melc`, so you can compile a site without installing
anything:

```bash
docker run --rm -v "$PWD:/work" -w /work --entrypoint /usr/local/bin/melc ghcr.io/thimis/melhttp build -o site ./my-site
```

---

## From source

You need **Go 1.26 or newer** and **git**.

| OS | Install Go |
|---|---|
| Windows | `winget install GoLang.Go` (then open a new terminal) |
| macOS | `brew install go` |
| Debian / Ubuntu | the distribution's package is often too old: download from <https://go.dev/dl/> and follow its instructions, or use `snap install go --classic` |
| Fedora | `sudo dnf install golang` |
| Arch | `sudo pacman -S go` |
| FreeBSD | `pkg install go` (check `go version` ≥ 1.26) |

Then build:

```bash
git clone https://github.com/thimis/MelHttp.git
cd MelHttp
go generate ./internal/webvm          # builds the browser VM (needed for -obfuscate and -playground)
go build -o bin/ ./cmd/melc ./cmd/melhttpd
```

The binaries are in `bin/`; copy them anywhere on your `PATH`.

**Release archives.** `go run ./tools/dist` cross-compiles archives for all 8
platforms into `dist/`, from any OS.

**`go install`** works too, without the browser VM:

```bash
go install github.com/thimis/MelHttp/cmd/melc@latest github.com/thimis/MelHttp/cmd/melhttpd@latest
```

`go install` cannot run `go generate`, so that `melhttpd` refuses
`-obfuscate` and `-playground`; everything else works. To get those two
features, build from a clone as shown above.

### Optional tools

| Tool | Needed for |
|---|---|
| Node.js (LTS) and npm | building framework sites (Angular, React, Vue, …) with `melc build --run-build` |
| Docker | the Docker image, the reference-interpreter tests, the ACME test |
| `GOOS=wasip1` (built into Go) | compiling Go programs into WASI handlers |

---

## Check the installation

Linux, macOS, FreeBSD:

```bash
melc version && melhttpd -version
mkdir demo && echo '<h1>It works</h1>' > demo/index.html
melc build -o site demo            # compile to Malbolge
melhttpd -root site                # http://localhost:8080 → "It works", printed by Malbolge
```

Windows (PowerShell):

```powershell
melc version; melhttpd -version
mkdir demo; Set-Content -Encoding utf8 demo\index.html '<h1>It works</h1>'
melc build -o site demo
melhttpd -root site
```

`curl -i http://localhost:8080/` shows `X-Powered-By: Malbolge` and the number
of Malbolge instructions behind the page in `X-Malbolge-Steps`.
