param(
    [switch]$Apply,
    [switch]$Cleanup,
    [string]$Registry = "127.0.0.1:5000",
    [string]$RegistryUsername = $env:GATE0B_REGISTRY_USERNAME,
    [string]$RegistryPassword = $env:GATE0B_REGISTRY_PASSWORD,
    [string]$SourceImage = "nginx:alpine",
    [string]$Repository = "portainer-cn/gate0b-spike",
    [string]$Tag = "v0.1",
    [string]$EvidenceRoot = "docs/spike/evidence/gate0b/S7-private-registry"
)

$ErrorActionPreference = "Stop"

$PrivateImage = "$Registry/$Repository`:$Tag"
$MissingImage = "$Registry/$Repository-missing:missing"
$EvidencePath = Join-Path (Get-Location) $EvidenceRoot
$DockerConfigPath = Join-Path (Get-Location) ".tmp/gate0b-registry-docker-config"
$script:TranscriptStarted = $false
$TranscriptPath = Join-Path $EvidencePath "transcript.txt"

function Write-Header {
    Write-Host "Gate 0B private registry Spike helper"
    Write-Host "Apply mode: $Apply"
    Write-Host "Cleanup mode: $Cleanup"
    Write-Host "Registry: $Registry"
    Write-Host "Username provided: $([bool]$RegistryUsername)"
    Write-Host "Password provided: $([bool]$RegistryPassword)"
    Write-Host "SourceImage: $SourceImage"
    Write-Host "PrivateImage: $PrivateImage"
    Write-Host "EvidenceRoot: $EvidenceRoot"
    Write-Host ""
}

function Start-Evidence {
    if (-not $Apply) {
        return
    }

    New-Item -ItemType Directory -Force -Path $EvidencePath | Out-Null
    Set-Content -Path (Join-Path $EvidencePath "metadata.txt") -Encoding UTF8 -Value @(
        "scenario=S7-private-registry",
        "registry=$Registry",
        "username_provided=$([bool]$RegistryUsername)",
        "password_provided=$([bool]$RegistryPassword)",
        "source_image=$SourceImage",
        "private_image=$PrivateImage",
        "missing_image=$MissingImage",
        "started_at=$((Get-Date).ToString('s'))",
        "cleanup=$Cleanup"
    )
    Set-Content -Path (Join-Path $EvidencePath "commands.txt") -Encoding UTF8 -Value @()
    Start-Transcript -Path $TranscriptPath -Force | Out-Null
    $script:TranscriptStarted = $true
}

function Stop-Evidence {
    if ($script:TranscriptStarted) {
        Stop-Transcript | Out-Null
        $script:TranscriptStarted = $false
    }
}

function Add-CommandRecord {
    param([string]$CommandLine)

    if ($Apply) {
        Add-Content -Path (Join-Path $EvidencePath "commands.txt") -Encoding UTF8 -Value $CommandLine
    }
}

function Invoke-Step {
    param(
        [string]$Name,
        [scriptblock]$Block
    )

    Write-Host ""
    Write-Host "==> $Name"
    if ($Apply) {
        & $Block
        return
    }

    Write-Host "[dry-run] $Name"
}

function Invoke-DockerCapture {
    param(
        [string]$FileName,
        [string[]]$DockerArgs,
        [string]$CommandRecord,
        [switch]$AllowFailure
    )

    Write-Host $CommandRecord
    if (-not $Apply) {
        return 0
    }

    Add-CommandRecord $CommandRecord
    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $output = & docker @DockerArgs 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    $text = ($output | Out-String)
    $content = @(
        "exit_code=$exitCode",
        "captured_at=$((Get-Date).ToString('s'))",
        "",
        $text.TrimEnd()
    )
    Set-Content -Path (Join-Path $EvidencePath $FileName) -Encoding UTF8 -Value $content
    if ($text.Trim()) {
        Write-Host $text.TrimEnd()
    }
    if ($exitCode -ne 0 -and -not $AllowFailure) {
        throw "$CommandRecord failed with exit code $exitCode"
    }

    return [int]$exitCode
}

function Invoke-DockerCaptureInput {
    param(
        [string]$FileName,
        [string[]]$DockerArgs,
        [string]$InputText,
        [string]$CommandRecord,
        [switch]$AllowFailure
    )

    Write-Host $CommandRecord
    if (-not $Apply) {
        return 0
    }

    Add-CommandRecord $CommandRecord
    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $output = $InputText | & docker @DockerArgs 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    $text = ($output | Out-String)
    $content = @(
        "exit_code=$exitCode",
        "captured_at=$((Get-Date).ToString('s'))",
        "",
        $text.TrimEnd()
    )
    Set-Content -Path (Join-Path $EvidencePath $FileName) -Encoding UTF8 -Value $content
    if ($text.Trim()) {
        Write-Host $text.TrimEnd()
    }
    if ($exitCode -ne 0 -and -not $AllowFailure) {
        throw "$CommandRecord failed with exit code $exitCode"
    }

    return [int]$exitCode
}

function Prepare-DockerConfig {
    if (-not $Apply) {
        return
    }

    if (Test-Path $DockerConfigPath) {
        Remove-Item -Recurse -Force -LiteralPath $DockerConfigPath
    }
    New-Item -ItemType Directory -Force -Path $DockerConfigPath | Out-Null
}

function Remove-DockerConfig {
    if (Test-Path $DockerConfigPath) {
        Remove-Item -Recurse -Force -LiteralPath $DockerConfigPath
    }
}

function Read-ExitCode {
    param([string]$FileName)

    $path = Join-Path $EvidencePath $FileName
    if (-not (Test-Path $path)) {
        return ""
    }
    $match = [regex]::Match((Get-Content -Raw -Encoding UTF8 $path), "exit_code=(\d+)")
    if ($match.Success) {
        return $match.Groups[1].Value
    }
    return ""
}

function Read-RepoDigest {
    $path = Join-Path $EvidencePath "private-image-repodigests.txt"
    if (-not (Test-Path $path)) {
        return ""
    }

    $content = Get-Content -Raw -Encoding UTF8 $path
    $match = [regex]::Match($content, "$([regex]::Escape($Registry))/[^`"\s,]+@sha256:[a-f0-9]+")
    if ($match.Success) {
        return $match.Value
    }
    return ""
}

Write-Header

if (-not $Apply) {
    Write-Host "This script is in dry-run mode. Re-run with -Apply to execute Docker commands."
    Write-Host "Set GATE0B_REGISTRY_USERNAME and GATE0B_REGISTRY_PASSWORD in the parent shell before -Apply."
}

if ($Apply -and -not $Cleanup -and (-not $RegistryUsername -or -not $RegistryPassword)) {
    throw "RegistryUsername and RegistryPassword are required in -Apply mode. Prefer environment variables to avoid writing credentials to transcripts."
}

Start-Evidence

try {
    if ($Cleanup) {
        Invoke-Step "Remove local private image tag and temporary Docker config" {
            [void](Invoke-DockerCapture "cleanup-image-rm.txt" @("image", "rm", $PrivateImage) "docker image rm $PrivateImage" -AllowFailure)
            Remove-DockerConfig
        }
        return
    }

    Prepare-DockerConfig

    Invoke-Step "Login with correct credentials using temporary Docker config" {
        [void](Invoke-DockerCaptureInput "login-success.txt" @(
            "--config", $DockerConfigPath,
            "login", $Registry,
            "--username", $RegistryUsername,
            "--password-stdin"
        ) $RegistryPassword "docker --config <temp-docker-config> login $Registry --username <redacted> --password-stdin")
    }

    Invoke-Step "Verify wrong credentials fail with stable auth reason" {
        [void](Invoke-DockerCaptureInput "login-wrong-password.txt" @(
            "--config", $DockerConfigPath,
            "login", $Registry,
            "--username", $RegistryUsername,
            "--password-stdin"
        ) "definitely-wrong-password" "docker --config <temp-docker-config> login $Registry --username <redacted> --password-stdin # wrong password" -AllowFailure)
    }

    Invoke-Step "Push public image into private registry" {
        [void](Invoke-DockerCapture "source-pull.txt" @("pull", $SourceImage) "docker pull $SourceImage")
        [void](Invoke-DockerCapture "private-tag.txt" @("tag", $SourceImage, $PrivateImage) "docker tag $SourceImage $PrivateImage")
        [void](Invoke-DockerCapture "private-push.txt" @("--config", $DockerConfigPath, "push", $PrivateImage) "docker --config <temp-docker-config> push $PrivateImage")
    }

    Invoke-Step "Pull private image with correct credentials and resolve digest" {
        [void](Invoke-DockerCapture "private-local-rm-before-pull.txt" @("image", "rm", $PrivateImage) "docker image rm $PrivateImage" -AllowFailure)
        [void](Invoke-DockerCapture "private-pull.txt" @("--config", $DockerConfigPath, "pull", $PrivateImage) "docker --config <temp-docker-config> pull $PrivateImage")
        [void](Invoke-DockerCapture "private-image-repodigests.txt" @("image", "inspect", $PrivateImage, "--format", "{{json .RepoDigests}}") "docker image inspect $PrivateImage --format <RepoDigests>")
        [void](Invoke-DockerCapture "manifest-inspect.json" @("--config", $DockerConfigPath, "manifest", "inspect", $PrivateImage) "docker --config <temp-docker-config> manifest inspect $PrivateImage" -AllowFailure)
    }

    Invoke-Step "Verify missing private image maps to image pull failure" {
        [void](Invoke-DockerCapture "missing-image-pull.txt" @("--config", $DockerConfigPath, "pull", $MissingImage) "docker --config <temp-docker-config> pull $MissingImage" -AllowFailure)
    }

    Invoke-Step "Write sanitized summary and remove temporary Docker config" {
        $digest = Read-RepoDigest
        $loginExit = Read-ExitCode "login-success.txt"
        $wrongLoginExit = Read-ExitCode "login-wrong-password.txt"
        $pushExit = Read-ExitCode "private-push.txt"
        $pullExit = Read-ExitCode "private-pull.txt"
        $missingPullExit = Read-ExitCode "missing-image-pull.txt"

        $passed = ($loginExit -eq "0" -and $pushExit -eq "0" -and $pullExit -eq "0" -and $wrongLoginExit -ne "0" -and $missingPullExit -ne "0" -and $digest)
        $summary = @(
            "checked_at=$((Get-Date).ToString('s'))",
            "registry=$Registry",
            "private_image=$PrivateImage",
            "digest=$digest",
            "login_success_exit=$loginExit",
            "wrong_login_exit=$wrongLoginExit",
            "push_exit=$pushExit",
            "pull_exit=$pullExit",
            "missing_pull_exit=$missingPullExit",
            "wrong_login_reason=REGISTRY_AUTH_FAILED",
            "missing_image_reason=IMAGE_PULL_FAILED",
            "conclusion=$(if ($passed) { "PRIVATE_REGISTRY_SPIKE_PASSED" } else { "PRIVATE_REGISTRY_SPIKE_INCOMPLETE" })"
        )
        Set-Content -Path (Join-Path $EvidencePath "registry-summary.txt") -Encoding UTF8 -Value $summary
        [void](Invoke-DockerCapture "private-local-rm-after-summary.txt" @("image", "rm", $PrivateImage) "docker image rm $PrivateImage" -AllowFailure)
        Remove-DockerConfig
    }

    Write-Host ""
    Write-Host "Private registry Spike completed. Temporary Docker config removed."
} finally {
    Stop-Evidence
    Remove-DockerConfig
}
