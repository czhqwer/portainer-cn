param(
    [switch]$Apply,
    [switch]$Cleanup,
    [switch]$RecordAllContainers,
    [string]$Image = "nginx:alpine",
    [string]$ProjectSlug = "demo-shop",
    [string]$EnvironmentSlug = "prod-cn",
    [string]$ServiceSlug = "web-api",
    [string]$OldReleaseID = "2026071101",
    [string]$NewReleaseID = "2026071102",
    [int]$OfficialPort = 18081,
    [string]$EvidenceRoot = "docs/spike/evidence/gate0b/S5-versioned-naming"
)

$ErrorActionPreference = "Stop"

$LabelKey = "com.portainer-cn.platform.spike"
$LabelValue = "gate0b"
$BaseName = "pcn-$ProjectSlug-$EnvironmentSlug-$ServiceSlug"
$OldOfficialName = "$BaseName-r$OldReleaseID"
$NewCandidateName = "$BaseName-r$NewReleaseID-candidate"
$NewOfficialName = "$BaseName-r$NewReleaseID"
$EvidencePath = Join-Path (Get-Location) $EvidenceRoot
$script:TranscriptStarted = $false
$TranscriptPath = Join-Path $EvidencePath "transcript.txt"

function Write-Header {
    Write-Host "Gate 0B versioned naming Spike helper"
    Write-Host "Apply mode: $Apply"
    Write-Host "Cleanup mode: $Cleanup"
    Write-Host "RecordAllContainers: $RecordAllContainers"
    Write-Host "Image: $Image"
    Write-Host "OldOfficialName: $OldOfficialName"
    Write-Host "NewCandidateName: $NewCandidateName"
    Write-Host "NewOfficialName: $NewOfficialName"
    Write-Host "OfficialPort: $OfficialPort"
    Write-Host "EvidenceRoot: $EvidenceRoot"
    Write-Host ""
}

function Start-Evidence {
    if (-not $Apply) {
        return
    }

    New-Item -ItemType Directory -Force -Path $EvidencePath | Out-Null
    Set-Content -Path (Join-Path $EvidencePath "metadata.txt") -Encoding UTF8 -Value @(
        "scenario=S5-versioned-naming",
        "image=$Image",
        "project_slug=$ProjectSlug",
        "environment_slug=$EnvironmentSlug",
        "service_slug=$ServiceSlug",
        "old_release_id=$OldReleaseID",
        "new_release_id=$NewReleaseID",
        "old_official_name=$OldOfficialName",
        "new_candidate_name=$NewCandidateName",
        "new_official_name=$NewOfficialName",
        "official_port=$OfficialPort",
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

function DockerContainerListArgs {
    if ($RecordAllContainers) {
        return @("ps", "-a", "--format", "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}")
    }

    return @("ps", "-a", "--filter", "label=$LabelKey=$LabelValue", "--format", "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}")
}

function Get-HostPort {
    param(
        [string]$Name,
        [string]$ContainerPort
    )

    $mapping = & docker port $Name "$ContainerPort/tcp"
    if (-not $mapping) {
        throw "Cannot resolve host port for $Name $ContainerPort/tcp"
    }

    $firstMapping = @($mapping)[0]
    $port = ($firstMapping -split ":")[-1]
    if (-not $port) {
        throw "Cannot parse host port from mapping: $firstMapping"
    }

    return [int]$port
}

function Test-Http {
    param(
        [string]$Url,
        [string]$FileName
    )

    $args = @("-sS", "-i", "-L", "--max-time", "8", "-o", "-", "-w", "`nstatus=%{http_code}`n", $Url)
    Write-Host "curl.exe $($args -join ' ')"
    if (-not $Apply) {
        return 0
    }

    Add-CommandRecord "curl.exe $($args -join ' ')"
    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $output = & curl.exe @args 2>&1
        $exitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorActionPreference
    }
    $text = ($output | Out-String)
    Set-Content -Path (Join-Path $EvidencePath $FileName) -Encoding UTF8 -Value @(
        "exit_code=$exitCode",
        "captured_at=$((Get-Date).ToString('s'))",
        "url=$Url",
        "",
        $text.TrimEnd()
    )
    if ($text.Trim()) {
        Write-Host $text.TrimEnd()
    }
    if ($exitCode -ne 0) {
        throw "curl $Url failed with exit code $exitCode"
    }

    return [int]$exitCode
}

function Read-StatusCode {
    param([string]$FileName)

    $path = Join-Path $EvidencePath $FileName
    if (-not (Test-Path $path)) {
        return ""
    }
    $match = [regex]::Match((Get-Content -Raw -Encoding UTF8 $path), "status=(\d+)")
    if ($match.Success) {
        return $match.Groups[1].Value
    }
    return ""
}

Write-Header

if (-not $Apply) {
    Write-Host "This script is in dry-run mode. Re-run with -Apply to execute Docker commands."
    Write-Host "Use -Cleanup -Apply to remove containers created by this helper."
}

Start-Evidence

try {
    if ($Cleanup) {
        Invoke-Step "Remove versioned naming helper-created containers" {
            [void](Invoke-DockerCapture "cleanup-rm-old.txt" @("rm", "-f", $OldOfficialName) "docker rm -f $OldOfficialName" -AllowFailure)
            [void](Invoke-DockerCapture "cleanup-rm-candidate.txt" @("rm", "-f", $NewCandidateName) "docker rm -f $NewCandidateName" -AllowFailure)
            [void](Invoke-DockerCapture "cleanup-rm-new.txt" @("rm", "-f", $NewOfficialName) "docker rm -f $NewOfficialName" -AllowFailure)
            [void](Invoke-DockerCapture "containers-after-cleanup.txt" (DockerContainerListArgs) "docker ps -a <helper-filter>")
        }
        return
    }

    Invoke-Step "Collect initial Docker state and pull image" {
        [void](Invoke-DockerCapture "containers-before.txt" (DockerContainerListArgs) "docker ps -a <helper-filter>")
        [void](Invoke-DockerCapture "image-pull.txt" @("pull", $Image) "docker pull $Image")
    }

    Invoke-Step "Pre-clean deterministic versioned names" {
        [void](Invoke-DockerCapture "preclean-old.txt" @("rm", "-f", $OldOfficialName) "docker rm -f $OldOfficialName" -AllowFailure)
        [void](Invoke-DockerCapture "preclean-candidate.txt" @("rm", "-f", $NewCandidateName) "docker rm -f $NewCandidateName" -AllowFailure)
        [void](Invoke-DockerCapture "preclean-new.txt" @("rm", "-f", $NewOfficialName) "docker rm -f $NewOfficialName" -AllowFailure)
    }

    Invoke-Step "Create old official container with release-versioned name" {
        [void](Invoke-DockerCapture "old-official-run.txt" @(
            "run", "-d",
            "--name", $OldOfficialName,
            "--label", "$LabelKey=$LabelValue",
            "--label", "com.portainer-cn.platform.runtime-role=current",
            "--label", "com.portainer-cn.platform.release-id=$OldReleaseID",
            "-p", "$OfficialPort`:80",
            $Image
        ) "docker run -d --name $OldOfficialName --label $LabelKey=$LabelValue -p $OfficialPort`:80 $Image")
        [void](Invoke-DockerCapture "old-official-inspect.json" @("inspect", $OldOfficialName) "docker inspect $OldOfficialName")
        [void](Test-Http "http://127.0.0.1:$OfficialPort/" "old-official-healthcheck.txt")
    }

    Invoke-Step "Create and remove new release candidate with candidate suffix" {
        [void](Invoke-DockerCapture "candidate-run.txt" @(
            "run", "-d",
            "--name", $NewCandidateName,
            "--label", "$LabelKey=$LabelValue",
            "--label", "com.portainer-cn.platform.runtime-role=candidate",
            "--label", "com.portainer-cn.platform.release-id=$NewReleaseID",
            "-P",
            $Image
        ) "docker run -d --name $NewCandidateName --label $LabelKey=$LabelValue -P $Image")
        $candidatePort = Get-HostPort $NewCandidateName 80
        Set-Content -Path (Join-Path $EvidencePath "candidate-port.txt") -Encoding UTF8 -Value @(
            "candidate_name=$NewCandidateName",
            "container_port=80",
            "host_port=$candidatePort"
        )
        [void](Invoke-DockerCapture "candidate-inspect.json" @("inspect", $NewCandidateName) "docker inspect $NewCandidateName")
        [void](Test-Http "http://127.0.0.1:$candidatePort/" "candidate-healthcheck.txt")
        [void](Invoke-DockerCapture "candidate-rm-after-validation.txt" @("rm", "-f", $NewCandidateName) "docker rm -f $NewCandidateName")
    }

    Invoke-Step "Keep old release container name and create new official name" {
        [void](Invoke-DockerCapture "old-official-stop.txt" @("stop", $OldOfficialName) "docker stop $OldOfficialName")
        [void](Invoke-DockerCapture "new-official-run.txt" @(
            "run", "-d",
            "--name", $NewOfficialName,
            "--label", "$LabelKey=$LabelValue",
            "--label", "com.portainer-cn.platform.runtime-role=current",
            "--label", "com.portainer-cn.platform.release-id=$NewReleaseID",
            "-p", "$OfficialPort`:80",
            $Image
        ) "docker run -d --name $NewOfficialName --label $LabelKey=$LabelValue -p $OfficialPort`:80 $Image")
        [void](Invoke-DockerCapture "new-official-inspect.json" @("inspect", $NewOfficialName) "docker inspect $NewOfficialName")
        [void](Test-Http "http://127.0.0.1:$OfficialPort/" "new-official-healthcheck.txt")
        [void](Invoke-DockerCapture "containers-during-versioned-retention.txt" (DockerContainerListArgs) "docker ps -a <helper-filter>")
    }

    Invoke-Step "Write summary and cleanup helper containers" {
        $oldStatus = Read-StatusCode "old-official-healthcheck.txt"
        $candidateStatus = Read-StatusCode "candidate-healthcheck.txt"
        $newStatus = Read-StatusCode "new-official-healthcheck.txt"
        $passed = ($oldStatus -eq "200" -and $candidateStatus -eq "200" -and $newStatus -eq "200")
        Set-Content -Path (Join-Path $EvidencePath "versioned-naming-summary.txt") -Encoding UTF8 -Value @(
            "checked_at=$((Get-Date).ToString('s'))",
            "old_official_name=$OldOfficialName",
            "new_candidate_name=$NewCandidateName",
            "new_official_name=$NewOfficialName",
            "old_release_health_status=$oldStatus",
            "candidate_health_status=$candidateStatus",
            "new_release_health_status=$newStatus",
            "old_container_retained_during_new_run=true",
            "conclusion=$(if ($passed) { "VERSIONED_NAMING_SPIKE_PASSED" } else { "VERSIONED_NAMING_SPIKE_INCOMPLETE" })"
        )
        [void](Invoke-DockerCapture "cleanup-rm-old.txt" @("rm", "-f", $OldOfficialName) "docker rm -f $OldOfficialName" -AllowFailure)
        [void](Invoke-DockerCapture "cleanup-rm-new.txt" @("rm", "-f", $NewOfficialName) "docker rm -f $NewOfficialName" -AllowFailure)
        [void](Invoke-DockerCapture "containers-after.txt" (DockerContainerListArgs) "docker ps -a <helper-filter>")
    }

    Write-Host ""
    Write-Host "Versioned naming Spike completed."
} finally {
    Stop-Evidence
}
