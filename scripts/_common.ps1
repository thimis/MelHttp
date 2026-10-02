# Shared helpers for scripts\test.ps1 and scripts\serve.ps1 (dot-sourced).
# Works in Windows PowerShell 5.1 and PowerShell 7 (Windows, Linux, macOS).

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$OnWindows = ($PSVersionTable.PSEdition -eq 'Desktop') -or $IsWindows
$Exe = if ($OnWindows) { '.exe' } else { '' }
$Bin = Join-Path $Root 'bin'
$Melc = Join-Path $Bin "melc$Exe"
$Melhttpd = Join-Path $Bin "melhttpd$Exe"

function Write-Step([string]$Message) {
    Write-Host ""
    Write-Host "==> $Message" -ForegroundColor Cyan
}

# Ensure-Go finds the Go toolchain, re-reading PATH in case Go was installed
# after this terminal was opened.
function Ensure-Go {
    if (Get-Command go -ErrorAction SilentlyContinue) { return }
    if ($OnWindows) {
        $env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' +
                    [Environment]::GetEnvironmentVariable('Path', 'User')
    }
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        throw "Go was not found. Install it (Windows: winget install GoLang.Go) and open a new terminal."
    }
}

# Invoke-Checked runs a native command from the repository root and throws
# if it fails.
function Invoke-Checked([string]$File, [string[]]$Arguments) {
    Push-Location $Root
    # Tools such as go and docker print progress on stderr; in Windows
    # PowerShell that becomes an error record (fatal under "Stop") when output
    # is redirected. Success is judged by the exit code alone.
    $saved = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        & $File @Arguments
        if ($LASTEXITCODE -ne 0) { throw "$File $($Arguments -join ' ') failed (exit code $LASTEXITCODE)" }
    } finally {
        Pop-Location
        $ErrorActionPreference = $saved
    }
}

# Build-Binaries builds melc and melhttpd into bin\. The browser VM
# (melhttp.wasm) is regenerated only when it is missing or older than the
# Go code it is built from.
function Build-Binaries {
    Ensure-Go
    $wasm = Join-Path $Root 'internal\webvm\assets\melhttp.wasm'
    $sources = @('internal\malbolge', 'internal\gen', 'internal\obfs', 'cmd\melwasm') |
        ForEach-Object { Get-ChildItem (Join-Path $Root $_) -Filter *.go }
    $newest = ($sources | Measure-Object -Property LastWriteTime -Maximum).Maximum
    if (-not (Test-Path $wasm) -or (Get-Item $wasm).LastWriteTime -lt $newest) {
        Write-Host "    building the browser VM (melhttp.wasm)"
        Invoke-Checked go @('generate', './internal/webvm')
    }
    Invoke-Checked go @('build', '-o', 'bin/', './cmd/melc', './cmd/melhttpd', './tools/testreport')
}

$Report = Join-Path $Bin "testreport$Exe"

# Invoke-GoTests runs "go test -json <args>" through tools/testreport, which
# prints readable results and lists every skipped and failed test. It
# returns the report's last lines (totals and skips) and throws on failure.
function Invoke-GoTests([string[]]$GoArgs, [switch]$Each) {
    Push-Location $Root
    $saved = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    # Pipe text between the two programs as UTF-8 (Windows PowerShell's
    # default is ASCII, which would garble non-ASCII test output).
    $savedEncoding = $OutputEncoding
    $OutputEncoding = New-Object System.Text.UTF8Encoding $false
    try {
        $reportArgs = @()
        if ($Each) { $reportArgs += '-each' }
        & go test -json @GoArgs | & $Report @reportArgs | Tee-Object -Variable lines | Out-Host
        $code = $LASTEXITCODE
    } finally {
        Pop-Location
        $ErrorActionPreference = $saved
        $OutputEncoding = $savedEncoding
    }
    $script:LastReport = $lines
    if ($code -ne 0) { throw "tests failed (details above)" }
}

function Test-Docker {
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { return $false }
    docker info --format '{{.ServerVersion}}' *> $null
    return ($LASTEXITCODE -eq 0)
}

# Quote-Arg quotes an argument for Start-Process (which joins arguments with
# spaces and does not quote them itself).
function Quote-Arg([string]$Arg) {
    if ($Arg -match '[\s"]') { return '"' + ($Arg -replace '"', '\"') + '"' }
    return $Arg
}

# Wait-Healthy polls a /healthz URL until it answers 200 or the process exits.
function Wait-Healthy([string]$Url, $Process, [int]$Seconds = 60) {
    $deadline = (Get-Date).AddSeconds($Seconds)
    while ((Get-Date) -lt $deadline) {
        if ($Process -and $Process.HasExited) { return $false }
        try {
            $r = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 2
            if ($r.StatusCode -eq 200) { return $true }
        } catch { }
        Start-Sleep -Milliseconds 250
    }
    return $false
}
