<#
.SYNOPSIS
    Build MelHttp and run its tests, with a pass/fail summary at the end.

.DESCRIPTION
    Default: build, unit tests, a Hello World smoke test, and the acceptance
    goals that need no Docker (a few minutes; the Angular/React/Vue goals need
    Node.js and npm).

    -Quick  build + unit tests + smoke test only (about a minute).
    -Full   everything: also the Docker and Let's Encrypt (ACME) goals, the
            reference-interpreter checks and the race detector (needs Docker
            Desktop running; about 10 minutes).

    Run from an elevated (Administrator) prompt to include the Windows service goal.

.EXAMPLE
    .\scripts\test.ps1 -Quick
.EXAMPLE
    .\scripts\test.ps1
.EXAMPLE
    .\scripts\test.ps1 -Full
#>
param(
    [switch]$Quick,
    [switch]$Full
)

. "$PSScriptRoot\_common.ps1"

$results = [ordered]@{}
function Stage([string]$Name, [scriptblock]$Body) {
    Write-Step $Name
    $watch = [Diagnostics.Stopwatch]::StartNew()
    try {
        & $Body
        $results[$Name] = 'PASS ({0:n0}s)' -f $watch.Elapsed.TotalSeconds
    } catch {
        $results[$Name] = 'FAIL  ' + $_.Exception.Message
        Write-Host $_.Exception.Message -ForegroundColor Red
    }
}
function Skip([string]$Name, [string]$Why) {
    $results[$Name] = "SKIP  $Why"
}

Stage 'Build melc and melhttpd' { Build-Binaries }

Stage 'Unit tests' { Invoke-Checked go @('test', './...') }

Stage 'Smoke test: Hello World in Malbolge' {
    $out = & $Melc run (Join-Path $Root 'testdata\programs\hello.mb')
    if ($out -ne 'Hello, world.') { throw "expected 'Hello, world.' but got '$out'" }
    Write-Host "    $out"
}

if (-not $Quick) {
    $docker = Test-Docker
    if ($Full -and -not $docker) {
        Write-Host "Docker is not running: the Docker goals will be skipped. Start Docker Desktop for -Full." -ForegroundColor Yellow
    }
    $env:MELHTTP_DOCKER = if ($Full -and $docker) { '1' } else { '' }
    Write-Host ""
    Write-Host "    (the acceptance goals take a few minutes; only failures are printed)" -ForegroundColor DarkGray
    Stage 'Acceptance goals (G1-G15)' {
        Invoke-Checked go @('test', '-tags', 'acceptance', '-count=1', '-timeout', '90m', './acceptance')
    }
    Remove-Item Env:\MELHTTP_DOCKER -ErrorAction SilentlyContinue
}

if ($Full) {
    if (Test-Docker) {
        Stage 'Reference interpreter (1998 C original, in Docker)' {
            Invoke-Checked go @('test', '-tags', 'reference', '-count=1', './internal/malbolge', './internal/gen')
        }
        Stage 'Race detector (Linux container)' {
            Invoke-Checked docker @('run', '--rm', '-v', "${Root}:/src", '-w', '/src', '-e', 'GOFLAGS=-buildvcs=false',
                'golang:1.27', 'sh', '-c', 'go generate ./internal/webvm >/dev/null && go test -race -count=1 ./...')
        }
    } else {
        Skip 'Reference interpreter (1998 C original, in Docker)' 'Docker is not running'
        Skip 'Race detector (Linux container)' 'Docker is not running'
    }
}

Write-Host ""
Write-Host "Summary" -ForegroundColor Cyan
$failed = $false
foreach ($k in $results.Keys) {
    $v = $results[$k]
    $color = if ($v.StartsWith('PASS')) { 'Green' } elseif ($v.StartsWith('SKIP')) { 'Yellow' } else { 'Red' }
    if ($v.StartsWith('FAIL')) { $failed = $true }
    Write-Host ("  {0,-52} {1}" -f $k, $v) -ForegroundColor $color
}
if ($failed) { exit 1 }
