param(
    [ValidateSet("build", "test", "check", "clean")]
    [string]$Action = "check",

    [switch]$CleanAfter
)

$ErrorActionPreference = "Stop"

$RepoRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$GoCacheDir = Join-Path $RepoRoot ".gocache"
$GoTmpDir = Join-Path $RepoRoot ".gotmp"

function Ensure-LocalGoDirs {
    New-Item -ItemType Directory -Force $GoCacheDir | Out-Null
    New-Item -ItemType Directory -Force $GoTmpDir | Out-Null
    $env:GOCACHE = $GoCacheDir
    $env:GOTMPDIR = $GoTmpDir
}

function Remove-LocalGoDirs {
    if (Test-Path $GoCacheDir) {
        Remove-Item -Recurse -Force $GoCacheDir
    }
    if (Test-Path $GoTmpDir) {
        Remove-Item -Recurse -Force $GoTmpDir
    }
}

Push-Location $RepoRoot

try {
    if ($Action -eq "clean") {
        Remove-LocalGoDirs
        Write-Host "Removed local Go cache directories." -ForegroundColor Green
        exit 0
    }

    Ensure-LocalGoDirs

    switch ($Action) {
        "build" {
            Write-Host "Building with local Go cache..." -ForegroundColor Cyan
            go build ./...
        }
        "test" {
            Write-Host "Running tests with local Go cache..." -ForegroundColor Cyan
            go test ./...
        }
        "check" {
            Write-Host "Building with local Go cache..." -ForegroundColor Cyan
            go build ./...
            Write-Host "Running tests with local Go cache..." -ForegroundColor Cyan
            go test ./...
        }
    }
}
finally {
    Pop-Location
    if ($CleanAfter) {
        Remove-LocalGoDirs
        Write-Host "Cleaned local Go cache directories." -ForegroundColor Green
    }
}
