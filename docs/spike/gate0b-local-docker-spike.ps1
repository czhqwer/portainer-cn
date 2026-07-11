param(
    [switch]$Apply,
    [switch]$Cleanup,
    [string]$Image = "nginx:alpine",
    [int]$OfficialPort = 18080
)

$ErrorActionPreference = "Stop"

$LabelKey = "com.portainer-cn.platform.spike"
$LabelValue = "gate0b"
$Prefix = "pcn-spike-gate0b"
$CandidateName = "$Prefix-candidate"
$OldName = "$Prefix-r1"
$NewName = "$Prefix-r2"
$BadName = "$Prefix-r2-bad"

function Write-Step {
    param([string]$Message)
    Write-Host ""
    Write-Host "==> $Message"
}

function Invoke-Step {
    param(
        [string]$Description,
        [scriptblock]$Action
    )

    Write-Step $Description
    if (-not $Apply) {
        Write-Host "[dry-run] $Description"
        return
    }

    & $Action
}

function Invoke-Docker {
    param([string[]]$Args)

    Write-Host "docker $($Args -join ' ')"
    if ($Apply) {
        & docker @Args
    }
}

function Remove-SpikeContainer {
    param([string]$Name)

    $exists = & docker ps -a --filter "name=^/$Name$" --format "{{.Names}}"
    if ($exists -eq $Name) {
        Invoke-Docker @("rm", "-f", $Name)
    }
}

function Get-HostPort {
    param(
        [string]$Name,
        [string]$ContainerPort
    )

    $port = & docker inspect $Name --format "{{(index (index .NetworkSettings.Ports `"$ContainerPort/tcp`") 0).HostPort}}"
    if (-not $port) {
        throw "Cannot resolve host port for $Name $ContainerPort/tcp"
    }

    return [int]$port
}

function Test-Http {
    param([string]$Url)

    Write-Host "GET $Url"
    $response = Invoke-WebRequest -UseBasicParsing -Uri $Url -TimeoutSec 10
    Write-Host "status=$($response.StatusCode)"
    if ($response.StatusCode -lt 200 -or $response.StatusCode -ge 400) {
        throw "Unexpected HTTP status $($response.StatusCode)"
    }
}

Write-Host "Gate 0B local Docker Spike helper"
Write-Host "Apply mode: $Apply"
Write-Host "Cleanup mode: $Cleanup"
Write-Host "Image: $Image"
Write-Host "OfficialPort: $OfficialPort"

if (-not $Apply) {
    Write-Host ""
    Write-Host "This script is in dry-run mode. Re-run with -Apply to execute Docker commands."
    Write-Host "Use -Cleanup -Apply to remove containers created by this helper."
}

if ($Cleanup) {
    Invoke-Step "Remove helper-created containers" {
        Remove-SpikeContainer $CandidateName
        Remove-SpikeContainer $NewName
        Remove-SpikeContainer $BadName
        Remove-SpikeContainer $OldName
    }
    return
}

Invoke-Step "Check Docker version" {
    & docker version
}

Invoke-Step "Pull public image" {
    Invoke-Docker @("pull", $Image)
}

Invoke-Step "Create candidate with random host port" {
    Remove-SpikeContainer $CandidateName
    Invoke-Docker @(
        "run", "-d",
        "--name", $CandidateName,
        "--label", "$LabelKey=$LabelValue",
        "--label", "com.portainer-cn.platform.runtime-role=candidate",
        "-P",
        $Image
    )
    $candidatePort = Get-HostPort $CandidateName 80
    Write-Host "candidate_url=http://127.0.0.1:$candidatePort/"
    Test-Http "http://127.0.0.1:$candidatePort/"
}

Invoke-Step "Stop and remove candidate after validation" {
    Invoke-Docker @("rm", "-f", $CandidateName)
}

Invoke-Step "Create old official container on the fixed port" {
    Remove-SpikeContainer $OldName
    Remove-SpikeContainer $NewName
    Invoke-Docker @(
        "run", "-d",
        "--name", $OldName,
        "--label", "$LabelKey=$LabelValue",
        "--label", "com.portainer-cn.platform.runtime-role=current",
        "-p", "$OfficialPort`:80",
        $Image
    )
    Test-Http "http://127.0.0.1:$OfficialPort/"
}

Invoke-Step "Switch fixed port from old official container to new versioned official container" {
    Invoke-Docker @("stop", $OldName)
    Invoke-Docker @(
        "run", "-d",
        "--name", $NewName,
        "--label", "$LabelKey=$LabelValue",
        "--label", "com.portainer-cn.platform.runtime-role=current",
        "-p", "$OfficialPort`:80",
        $Image
    )
    Test-Http "http://127.0.0.1:$OfficialPort/"
}

Invoke-Step "Simulate new official start failure and recover old container" {
    Invoke-Docker @("rm", "-f", $NewName)
    $failed = $false
    try {
        Invoke-Docker @(
            "run", "-d",
            "--name", $BadName,
            "--label", "$LabelKey=$LabelValue",
            "--label", "com.portainer-cn.platform.runtime-role=current",
            "-p", "$OfficialPort`:80",
            "nginx:__gate0b_missing_tag__"
        )
    } catch {
        $failed = $true
        Write-Host "expected_failure=$($_.Exception.Message)"
    }

    if (-not $failed) {
        throw "Expected bad image start to fail"
    }

    Invoke-Docker @("start", $OldName)
    Test-Http "http://127.0.0.1:$OfficialPort/"
}

Invoke-Step "Collect final helper container state" {
    & docker ps -a --filter "label=$LabelKey=$LabelValue" --format "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}"
}

Write-Host ""
Write-Host "Local Docker Spike subset completed. Run with -Cleanup -Apply to remove helper-created containers."
