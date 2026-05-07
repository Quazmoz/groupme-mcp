param(
    [string]$ImageName = "quazmoz/groupme-mcp",
    [string]$Tag = "dev",
    [string[]]$AdditionalTags = @(),
    [switch]$NoCache
)

$ErrorActionPreference = "Stop"

function Get-NormalizedTags {
    param(
        [string]$PrimaryTag,
        [string[]]$ExtraTags
    )

    $tags = New-Object System.Collections.Generic.List[string]
    foreach ($candidate in @($PrimaryTag) + $ExtraTags) {
        if ([string]::IsNullOrWhiteSpace($candidate)) {
            continue
        }

        foreach ($piece in ($candidate -split ",")) {
            $trimmed = $piece.Trim()
            if (-not [string]::IsNullOrWhiteSpace($trimmed) -and -not $tags.Contains($trimmed)) {
                $tags.Add($trimmed)
            }
        }
    }

    return $tags
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$allTags = Get-NormalizedTags -PrimaryTag $Tag -ExtraTags $AdditionalTags

if ($allTags.Count -eq 0) {
    throw "At least one image tag is required."
}

$gitShortSha = $null
if (Get-Command git -ErrorAction SilentlyContinue) {
    try {
        $candidate = (git -C $repoRoot rev-parse --short HEAD 2>$null).Trim()
        if (-not [string]::IsNullOrWhiteSpace($candidate) -and -not $allTags.Contains($candidate)) {
            $gitShortSha = $candidate
            $allTags.Add($candidate)
        }
    } catch {
    }
}

$primaryRef = "{0}:{1}" -f $ImageName, $allTags[0]
$buildArgs = @("build", "-t", $primaryRef)
if ($NoCache) {
    $buildArgs += "--no-cache"
}
$buildArgs += $repoRoot

docker @buildArgs
if ($LASTEXITCODE -ne 0) {
    throw "Docker build failed for $primaryRef."
}

for ($i = 1; $i -lt $allTags.Count; $i++) {
    $tagRef = "{0}:{1}" -f $ImageName, $allTags[$i]
    docker tag $primaryRef $tagRef
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to tag image as $tagRef."
    }
}

Write-Host "Built image tags:" -ForegroundColor Green
foreach ($builtTag in $allTags) {
    Write-Host (" - {0}:{1}" -f $ImageName, $builtTag)
}

if ($gitShortSha) {
    Write-Host ("Git SHA tag: {0}:{1}" -f $ImageName, $gitShortSha) -ForegroundColor DarkGray
}
