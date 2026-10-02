<#
.SYNOPSIS
    Build MelHttp and run its tests, with a pass/fail summary at the end.

.DESCRIPTION
    Default: build, unit tests, a Hello World smoke test, and the acceptance
    goals G1-G15 (a few minutes; the Angular/React/Vue goals need Node.js).
    Goals that need Docker are skipped - and listed as "Not run" - unless
    you use -Full.

    -Quick  build + unit tests + smoke test only (about a minute).
    -Full   everything: also the Docker and Let's Encrypt (ACME) goals, the
            reference-interpreter checks and the race detector. Strict: a
            missing tool (Docker, Node.js, git) is a failure, not a skip, so a
            passing -Full run means everything really ran. Needs Docker
            Desktop running; about 10 minutes.

    The Windows service goal (G15) needs an elevated (Administrator) prompt;
    otherwise it is listed under "Not run".

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
$notRun = New-Object System.Collections.Generic.List[string]

function Stage([string]$Name, [scriptblock]$Body) {
    Write-Step $Name
    $watch = [Diagnostics.Stopwatch]::StartNew()
    $script:LastReport = $null
    try {
        & $Body
        $results[$Name] = 'PASS ({0:n0}s)' -f $watch.Elapsed.TotalSeconds
    } catch {
        $results[$Name] = 'FAIL  ' + $_.Exception.Message
        Write-Host $_.Exception.Message -ForegroundColor Red
    }
    # Carry the test totals and every skipped test into the final summary.
    if ($script:LastReport) {
        $totals = $script:LastReport | Where-Object { $_ -like 'Tests: *' } | Select-Object -Last 1
        if ($totals) { $results[$Name] += '  ' + ($totals -replace '^Tests: ', '') }
        $inSkipped = $false
        foreach ($l in $script:LastReport) {
            if ($l -like 'Skipped*') { $inSkipped = $true; continue }
            if ($inSkipped -and $l -like '  *') { $notRun.Add($l.Trim()) } else { $inSkipped = $false }
        }
    }
}

if ($Full) {
    $env:MELHTTP_STRICT = '1'
    if (Test-Docker) { $env:MELHTTP_DOCKER = '1' } else {
        Write-Host "Docker is not running: -Full needs it (start Docker Desktop). The Docker stages will fail." -ForegroundColor Yellow
        $env:MELHTTP_DOCKER = '1'
    }
} else {
    Remove-Item Env:\MELHTTP_STRICT, Env:\MELHTTP_DOCKER -ErrorAction SilentlyContinue
}

try {
    Stage 'Build melc and melhttpd' { Build-Binaries }

    Stage 'Unit tests' { Invoke-GoTests @('./...') }

    Stage 'Smoke test: Hello World in Malbolge' {
        $out = & $Melc run (Join-Path $Root 'testdata\programs\hello.mb')
        if ($out -ne 'Hello, world.') { throw "expected 'Hello, world.' but got '$out'" }
        Write-Host "    $out"
    }

    if (-not $Quick) {
        Write-Host "    (the acceptance goals take a few minutes; each goal is listed as it finishes)" -ForegroundColor DarkGray
        Stage 'Acceptance goals (G1-G15)' {
            Invoke-GoTests @('-tags', 'acceptance', '-count=1', '-timeout', '90m', './acceptance') -Each
        }
    }

    if ($Full) {
        Stage 'Reference interpreter (1998 C original, in Docker)' {
            if (-not (Test-Docker)) { throw 'Docker is not running' }
            Invoke-GoTests @('-tags', 'reference', '-count=1', './internal/malbolge', './internal/gen')
        }
        Stage 'Race detector (Linux container)' {
            if (-not (Test-Docker)) { throw 'Docker is not running' }
            Invoke-Checked docker @('run', '--rm', '-v', "${Root}:/src", '-w', '/src', '-e', 'GOFLAGS=-buildvcs=false',
                'golang:1.27', 'sh', '-c', 'go generate ./internal/webvm >/dev/null && go test -race -count=1 ./...')
        }
    }
} finally {
    Remove-Item Env:\MELHTTP_STRICT, Env:\MELHTTP_DOCKER -ErrorAction SilentlyContinue
}

Write-Host ""
Write-Host "Summary" -ForegroundColor Cyan
$failed = $false
foreach ($k in $results.Keys) {
    $v = $results[$k]
    $color = if ($v.StartsWith('PASS')) { 'Green' } else { 'Red' }
    if (-not $v.StartsWith('PASS')) { $failed = $true }
    Write-Host ("  {0,-52} {1}" -f $k, $v) -ForegroundColor $color
}
if ($notRun.Count -gt 0) {
    Write-Host ""
    Write-Host "Not run (skipped, so not verified by this run):" -ForegroundColor Yellow
    foreach ($s in $notRun) { Write-Host "  $s" -ForegroundColor Yellow }
}
if ($failed) { exit 1 }
