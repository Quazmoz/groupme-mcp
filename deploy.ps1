$ErrorActionPreference = "Stop"

Write-Host "Using scripts/docker-push.ps1 for secure manual publishing..." -ForegroundColor Green

$scriptPath = Join-Path $PSScriptRoot "scripts\docker-push.ps1"
if (-not (Test-Path $scriptPath)) {
    throw "Missing publish script: $scriptPath"
}

& $scriptPath -Tag dev -Build
