param(
    [switch]$Apply,
    [switch]$Cleanup,
    [string]$Image = "nginx:alpine",
    [int]$OfficialPort = 18080,
    [string]$EvidenceRoot = "docs/spike/evidence/gate0b/S1-local-docker"
)

$ErrorActionPreference = "Stop"

$LabelKey = "com.portainer-cn.platform.spike"
$LabelValue = "gate0b"
$Prefix = "pcn-spike-gate0b"
$CandidateName = "$Prefix-candidate"
$OldName = "$Prefix-r1"
$NewName = "$Prefix-r2"
$BadName = "$Prefix-r2-bad"
$EvidencePath = if ([System.IO.Path]::IsPathRooted($EvidenceRoot)) {
    $EvidenceRoot
} else {
    Join-Path (Get-Location) $EvidenceRoot
}
$TranscriptPath = Join-Path $EvidencePath "transcript.txt"
$TranscriptStarted = $false

function Start-Evidence {
    if (-not $Apply) {
        return
    }

    New-Item -ItemType Directory -Force -Path $EvidencePath | Out-Null
    Set-Content -Path (Join-Path $EvidencePath "metadata.txt") -Encoding UTF8 -Value @(
        "scenario=S1-local-docker",
        "image=$Image",
        "official_port=$OfficialPort",
        "started_at=$((Get-Date).ToString('s'))",
        "cleanup=$Cleanup"
    )
    Start-Transcript -Path $TranscriptPath -Force | Out-Null
    $script:TranscriptStarted = $true
}

function Stop-Evidence {
    if ($script:TranscriptStarted) {
        Stop-Transcript | Out-Null
        $script:TranscriptStarted = $false
    }
}

function Save-CommandOutput {
    param(
        [string]$FileName,
        [string]$Command,
        [string[]]$Args
    )

    if (-not $Apply) {
        return
    }

    $line = "$Command $($Args -join ' ')"
    Write-Host $line
    Add-Content -Path (Join-Path $EvidencePath "commands.txt") -Encoding UTF8 -Value $line
    $output = & $Command @Args 2>&1
    $exitCode = $LASTEXITCODE
    $text = ($output | Out-String)
    if ($text.Trim()) {
        Write-Host $text.TrimEnd()
    }
    Set-Content -Path (Join-Path $EvidencePath $FileName) -Encoding UTF8 -Value $text
    if ($exitCode -ne 0) {
        throw "$line failed with exit code $exitCode"
    }
}

trap {
    Stop-Evidence
    throw
}

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
        Add-Content -Path (Join-Path $EvidencePath "commands.txt") -Encoding UTF8 -Value "docker $($Args -join ' ')"
        $output = & docker @Args 2>&1
        $exitCode = $LASTEXITCODE
        $text = ($output | Out-String)
        if ($text.Trim()) {
            Write-Host $text.TrimEnd()
        }
        if ($exitCode -ne 0) {
            throw "docker $($Args -join ' ') failed with exit code $exitCode"
        }
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
    param(
        [string]$Url,
        [string]$FileName = "healthcheck.txt"
    )

    Write-Host "GET $Url"
    $response = Invoke-WebRequest -UseBasicParsing -Uri $Url -TimeoutSec 10
    Write-Host "status=$($response.StatusCode)"
    if ($Apply) {
        Set-Content -Path (Join-Path $EvidencePath $FileName) -Encoding UTF8 -Value @(
            "url=$Url",
            "status=$($response.StatusCode)",
            "checked_at=$((Get-Date).ToString('s'))",
            "",
            $response.Content
        )
    }
    if ($response.StatusCode -lt 200 -or $response.StatusCode -ge 400) {
        throw "Unexpected HTTP status $($response.StatusCode)"
    }
}

Write-Host "Gate 0B local Docker Spike helper"
Write-Host "Apply mode: $Apply"
Write-Host "Cleanup mode: $Cleanup"
Write-Host "Image: $Image"
Write-Host "OfficialPort: $OfficialPort"
Write-Host "EvidenceRoot: $EvidenceRoot"

if (-not $Apply) {
    Write-Host ""
    Write-Host "This script is in dry-run mode. Re-run with -Apply to execute Docker commands."
    Write-Host "Use -Cleanup -Apply to remove containers created by this helper."
}

Start-Evidence

if ($Cleanup) {
    Invoke-Step "Remove helper-created containers" {
        Remove-SpikeContainer $CandidateName
        Remove-SpikeContainer $NewName
        Remove-SpikeContainer $BadName
        Remove-SpikeContainer $OldName
        Save-CommandOutput "containers-after-cleanup.txt" "docker" @("ps", "-a", "--filter", "label=$LabelKey=$LabelValue", "--format", "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}")
    }
    Stop-Evidence
    return
}

Invoke-Step "Check Docker version" {
    Save-CommandOutput "docker-version.txt" "docker" @("version")
    Save-CommandOutput "docker-context.txt" "docker" @("context", "ls")
    Save-CommandOutput "containers-before.txt" "docker" @("ps", "-a", "--format", "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}")
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
    Save-CommandOutput "candidate-inspect.json" "docker" @("inspect", $CandidateName)
    Test-Http "http://127.0.0.1:$candidatePort/" "candidate-healthcheck.txt"
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
    Save-CommandOutput "official-r1-inspect.json" "docker" @("inspect", $OldName)
    Test-Http "http://127.0.0.1:$OfficialPort/" "official-r1-healthcheck.txt"
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
    Save-CommandOutput "official-r2-inspect.json" "docker" @("inspect", $NewName)
    Test-Http "http://127.0.0.1:$OfficialPort/" "official-r2-healthcheck.txt"
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
    Save-CommandOutput "official-r1-recovered-inspect.json" "docker" @("inspect", $OldName)
    Test-Http "http://127.0.0.1:$OfficialPort/" "official-r1-recovered-healthcheck.txt"
}

Invoke-Step "Collect final helper container state" {
    Save-CommandOutput "containers-after.txt" "docker" @("ps", "-a", "--filter", "label=$LabelKey=$LabelValue", "--format", "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}")
}

Stop-Evidence

Write-Host ""
Write-Host "Local Docker Spike subset completed. Run with -Cleanup -Apply to remove helper-created containers."
