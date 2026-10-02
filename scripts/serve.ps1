<#
.SYNOPSIS
    Compile a site to Malbolge and serve it with one command.

.DESCRIPTION
    SITE is one of the demo sites, or the path to your own folder:

      hello       one page
      classic     multi-page site with images, Unicode and WebAssembly (default)
      cgi         MelCGI demos: programs that read the request
      angular     Angular Material showcase (needs Node.js; built on first use)
      react       React (Vite) app (needs Node.js)
      vue         Vue (Vite) app (needs Node.js)
      transport   pages travel to the browser as Malbolge programs, plus the playground
      wasi        a Go program compiled to WebAssembly, run as a sandboxed handler
      <path>      your own folder (a framework project with package.json is built first)

    Stop the server with Ctrl+C.

.EXAMPLE
    .\scripts\serve.ps1
    Serves the classic demo site on http://localhost:8080.
.EXAMPLE
    .\scripts\serve.ps1 angular -Open
    Builds the Angular showcase (first time only) and opens it in the browser.
.EXAMPLE
    .\scripts\serve.ps1 angular -Obfuscate -Open
    Serves the Angular app over the Malbolge transport: after one reload, every page and
    file travels to the browser as a Malbolge program. Works with any site.
.EXAMPLE
    .\scripts\serve.ps1 C:\my-site -Watch -Open
    Serves your folder and rebuilds it whenever a file changes.
.EXAMPLE
    .\scripts\serve.ps1 classic -Https
    Also serves HTTPS with a self-signed certificate on https://localhost:8443.
.EXAMPLE
    .\scripts\serve.ps1 -Docker
    Starts all eight demo sites in Docker (ports 8080-8087). Stop them with -Docker -Stop.
#>
param(
    [Parameter(Position = 0)][string]$Site = 'classic',
    [int]$Port = 8080,
    [int]$HttpsPort = 8443,
    [switch]$Https,     # also serve HTTPS with a self-signed certificate
    [switch]$Watch,     # rebuild on every change (your own folders)
    [switch]$Open,      # open the browser when the site is ready
    [switch]$Metrics,   # Prometheus metrics on http://127.0.0.1:9090/metrics
    [switch]$Obfuscate, # Malbolge transport for any site: pages travel to the browser as Malbolge
    [switch]$Rebuild,   # rebuild framework apps even if they were built before
    [switch]$Docker,    # all demo sites in Docker
    [switch]$Stop       # with -Docker: stop the containers
)

. "$PSScriptRoot\_common.ps1"

if ($Obfuscate -and ($Watch -or $Docker)) {
    throw "-Obfuscate works with a single site, not with -Watch or -Docker (in Docker, the transport demo is on port 8086)."
}

# ---- Docker: all demo sites ---------------------------------------------------
if ($Docker) {
    if (-not (Test-Docker)) { throw "Docker is not running. Start Docker Desktop and try again." }
    Push-Location $Root
    try {
        if ($Stop) {
            docker compose down
            return
        }
        Write-Step "Building and starting the demo sites in Docker (the first build takes a few minutes)"
        docker compose up --build -d --wait
        if ($LASTEXITCODE -ne 0) { throw "docker compose failed" }
    } finally {
        Pop-Location
    }
    Write-Host ""
    Write-Host "  http://localhost:8080  Angular Material showcase (Malbolge corner shows page source)"
    Write-Host "  http://localhost:8081  classic multi-page site"
    Write-Host "  http://localhost:8082  MelCGI demos (try /echo.txt?hi=1)"
    Write-Host "  http://localhost:8083  hello"
    Write-Host "  http://localhost:8084  React"
    Write-Host "  http://localhost:8085  Vue"
    Write-Host "  http://localhost:8086  Malbolge transport (reload once) and /_melhttp/playground.html"
    Write-Host "  http://localhost:8087  WASI handler at /hello.html"
    Write-Host ""
    Write-Host "Stop them with: .\scripts\serve.ps1 -Docker -Stop"
    if ($Open) { Start-Process 'http://localhost:8080' }
    return
}

# ---- Resolve the site -----------------------------------------------------------
Write-Step "Building melc and melhttpd"
Build-Binaries

$preset = 'static'
$extra = @()
$hint = $null
$isFramework = $false
switch ($Site.ToLower()) {
    'hello'     { $src = 'testsites\hello' }
    'classic'   { $src = 'testsites\classic' }
    'cgi'       { $src = 'testsites\cgi' }
    'angular'   { $src = 'testsites\angular-showcase'; $preset = 'angular'; $isFramework = $true; $extra += '-expose-source' }
    'react'     { $src = 'testsites\react-vite'; $preset = 'vite'; $isFramework = $true }
    'vue'       { $src = 'testsites\vue-vite'; $preset = 'vite'; $isFramework = $true }
    { $_ -in 'transport', 'obfuscated' } {
        $src = 'testsites\obfuscated\site'; $extra += '-obfuscate', '-playground'
        $hint = "Reload the page once; then pages travel as Malbolge (how to see it: docs\transport.md, 'Seeing it in your browser'). Playground: /_melhttp/playground.html"
    }
    'wasi' {
        $src = 'testsites\wasi'; $extra += '-wasi'
        $hint = "Open /hello.html: a Go program compiled to WebAssembly answers each request."
        Write-Step "Compiling the WASI demo handler"
        Push-Location $Root
        try {
            $env:GOOS = 'wasip1'; $env:GOARCH = 'wasm'
            go build -o testsites/wasi/hello.html.wasi ./testsites/wasi/src
            if ($LASTEXITCODE -ne 0) { throw "building the WASI handler failed" }
        } finally {
            Remove-Item Env:\GOOS, Env:\GOARCH -ErrorAction SilentlyContinue
            Pop-Location
        }
    }
    default {
        if (-not (Test-Path $Site -PathType Container)) {
            throw "Unknown site '$Site'. Use hello, classic, cgi, angular, react, vue, transport, wasi, or a folder path. See: Get-Help .\scripts\serve.ps1"
        }
        $src = (Resolve-Path $Site).Path
        if (Test-Path (Join-Path $src 'package.json')) { $preset = 'auto'; $isFramework = $true }
    }
}
if (-not [IO.Path]::IsPathRooted($src)) { $src = Join-Path $Root $src }
$name = Split-Path -Leaf $src
if ($name -eq 'site') { $name = Split-Path -Leaf (Split-Path -Parent $src) }
$out = Join-Path $Root "out\$name"

$buildArgs = @('--preset', $preset)
if ($isFramework -and ($Rebuild -or -not (Test-Path (Join-Path $src 'dist')))) { $buildArgs += '--run-build' }

$serverArgs = @('-root', $out, '-addr', ":$Port") + $extra
$urls = @("http://localhost:$Port")
if ($Https) {
    $serverArgs += '-tls-self-signed', '-tls-addr', ":$HttpsPort", '-https-redirect=false'
    $urls += "https://localhost:$HttpsPort  (self-signed: accept the browser warning)"
}
if ($Obfuscate -and $extra -notcontains '-obfuscate') {
    $serverArgs += '-obfuscate-inject'
    $hint = "Malbolge transport is on: reload the page once, then every page and file travels as Malbolge " +
            "and your browser decodes it. How to see it (Chrome, Edge, Firefox): docs\transport.md, " +
            "'Seeing it in your browser'. Use the http:// address: browsers refuse service workers on self-signed HTTPS."
}
if ($Metrics) {
    $serverArgs += '-metrics-addr', '127.0.0.1:9090'
    $urls += "http://127.0.0.1:9090/metrics"
}

# ---- Watch mode: melc watch serves and rebuilds -------------------------------
if ($Watch) {
    Write-Step "Watching $src (Ctrl+C to stop)"
    $watchArgs = @('watch', '-o', $out, '-serve', ":$Port") + $buildArgs.Where({ $_ -ne '--run-build' }) + @($src)
    $p = Start-Process -FilePath $Melc -ArgumentList ($watchArgs | ForEach-Object { Quote-Arg $_ }) -NoNewWindow -PassThru
    try {
        if ($Open -and (Wait-Healthy "http://127.0.0.1:$Port/healthz" $p)) { Start-Process "http://localhost:$Port" }
        Wait-Process -Id $p.Id
    } finally {
        if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
    }
    return
}

# ---- Build and serve ----------------------------------------------------------------
Write-Step "Compiling $src to Malbolge"
Invoke-Checked $Melc (@('build', '-o', $out) + $buildArgs + @($src))

Write-Step "Serving $name (Ctrl+C to stop)"
$p = Start-Process -FilePath $Melhttpd -ArgumentList ($serverArgs | ForEach-Object { Quote-Arg $_ }) -NoNewWindow -PassThru
try {
    if (-not (Wait-Healthy "http://127.0.0.1:$Port/healthz" $p)) {
        throw "melhttpd did not start (is port $Port already in use? try -Port 8090)"
    }
    Write-Host ""
    foreach ($u in $urls) { Write-Host "  $u" -ForegroundColor Green }
    if ($hint) { Write-Host "  $hint" -ForegroundColor DarkGray }
    Write-Host ""
    if ($Open) { Start-Process "http://localhost:$Port" }
    Wait-Process -Id $p.Id
} finally {
    if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
}
