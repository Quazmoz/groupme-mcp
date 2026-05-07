param(
    [string]$ImageName = "quazmoz/groupme-mcp",
    [string]$Tag = "dev",
    [string[]]$AdditionalTags = @(),
    [string]$Username,
    [switch]$Build,
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

function Test-DockerReady {
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
        throw "Docker CLI is not installed or not available on PATH."
    }

    docker info *> $null
    if ($LASTEXITCODE -ne 0) {
        throw "Docker daemon is not reachable. Start Docker Desktop or the Docker service and try again."
    }
}

function Get-DefaultDockerHubUsername {
    try {
        $detected = (docker info --format '{{.Username}}' 2>$null).Trim()
        if (-not [string]::IsNullOrWhiteSpace($detected) -and $detected -ne "<no value>") {
            return $detected
        }
    } catch {
    }

    return "quazmoz"
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$allTags = Get-NormalizedTags -PrimaryTag $Tag -ExtraTags $AdditionalTags
if ($allTags.Count -eq 0) {
    throw "At least one image tag is required."
}

Test-DockerReady

if ($Build) {
    & (Join-Path $PSScriptRoot "docker-build.ps1") -ImageName $ImageName -Tag $Tag -AdditionalTags $AdditionalTags -NoCache:$NoCache
}

$gitShortSha = $null
if (Get-Command git -ErrorAction SilentlyContinue) {
    try {
        $candidate = (git -C $repoRoot rev-parse --short HEAD 2>$null).Trim()
        if (-not [string]::IsNullOrWhiteSpace($candidate)) {
            $gitShortSha = $candidate
        }
    } catch {
    }
}

if ([string]::IsNullOrWhiteSpace($Username)) {
    $defaultUsername = Get-DefaultDockerHubUsername
    $enteredUsername = Read-Host "Docker Hub username [$defaultUsername]"
    if ([string]::IsNullOrWhiteSpace($enteredUsername)) {
        $Username = $defaultUsername
    } else {
        $Username = $enteredUsername.Trim()
    }
}

$secureToken = Read-Host "Docker Hub PAT or password" -AsSecureString
$bstr = [IntPtr]::Zero
$plainToken = $null
$loggedIn = $false

try {
    $bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureToken)
    $plainToken = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)
    $plainToken | docker login --username $Username --password-stdin
    if ($LASTEXITCODE -ne 0) {
        throw "Docker login failed for Docker Hub user '$Username'."
    }
    $loggedIn = $true

    foreach ($currentTag in $allTags) {
        $imageRef = "{0}:{1}" -f $ImageName, $currentTag
        docker image inspect $imageRef *> $null
        if ($LASTEXITCODE -ne 0) {
            throw "Local image tag $imageRef was not found. Build it first or use -Build."
        }

        docker push $imageRef
        if ($LASTEXITCODE -ne 0) {
            throw "Failed to push $imageRef."
        }
    }

    if ($gitShortSha) {
        $shaRef = "{0}:{1}" -f $ImageName, $gitShortSha
        docker image inspect $shaRef *> $null
        if ($LASTEXITCODE -eq 0 -and -not $allTags.Contains($gitShortSha)) {
            docker push $shaRef
            if ($LASTEXITCODE -ne 0) {
                throw "Failed to push $shaRef."
            }
        }
    }

    Write-Host "Pushed image tags:" -ForegroundColor Green
    foreach ($pushedTag in $allTags) {
        Write-Host (" - {0}:{1}" -f $ImageName, $pushedTag)
    }
    if ($gitShortSha) {
        $shaRef = "{0}:{1}" -f $ImageName, $gitShortSha
        docker image inspect $shaRef *> $null
        if ($LASTEXITCODE -eq 0) {
            Write-Host (" - {0}" -f $shaRef)
        }
    }
} finally {
    if ($bstr -ne [IntPtr]::Zero) {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
    }
    Remove-Variable plainToken -ErrorAction SilentlyContinue

    if ($loggedIn) {
        docker logout *> $null
    }
}
