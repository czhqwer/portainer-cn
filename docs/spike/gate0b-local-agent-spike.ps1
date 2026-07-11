param(
    [switch]$Apply,
    [switch]$Cleanup,
    [switch]$RecordAllContainers,
    [string]$AgentImage = "portainer/agent:2.43.0",
    [string]$CandidateImage = "nginx:alpine",
    [int]$AgentHostPort = 19001,
    [string]$EvidenceRoot = "docs/spike/evidence/gate0b/S3-local-agent"
)

$ErrorActionPreference = "Stop"

$LabelKey = "com.portainer-cn.platform.spike"
$LabelValue = "gate0b"
$Prefix = "pcn-spike-gate0b-s3"
$AgentName = "$Prefix-agent"
$CandidateName = "$Prefix-candidate"
$EvidencePath = Join-Path (Get-Location) $EvidenceRoot
$script:TranscriptStarted = $false
$TranscriptPath = Join-Path $EvidencePath "transcript.txt"

function Write-Header {
    Write-Host "Gate 0B local Agent Spike helper"
    Write-Host "Apply mode: $Apply"
    Write-Host "Cleanup mode: $Cleanup"
    Write-Host "RecordAllContainers: $RecordAllContainers"
    Write-Host "AgentImage: $AgentImage"
    Write-Host "CandidateImage: $CandidateImage"
    Write-Host "AgentHostPort: $AgentHostPort"
    Write-Host "EvidenceRoot: $EvidenceRoot"
    Write-Host ""
}

function Start-Evidence {
    if (-not $Apply) {
        return
    }

    New-Item -ItemType Directory -Force -Path $EvidencePath | Out-Null
    Set-Content -Path (Join-Path $EvidencePath "metadata.txt") -Encoding UTF8 -Value @(
        "scenario=S3-local-agent",
        "agent_image=$AgentImage",
        "candidate_image=$CandidateImage",
        "agent_host_port=$AgentHostPort",
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
        [string]$FileName,
        [switch]$InsecureTls,
        [switch]$AllowFailure
    )

    $args = @("-sS", "-i", "-L", "--max-time", "8", "-o", "-", "-w", "`nstatus=%{http_code}`n")
    if ($InsecureTls) {
        $args = @("-k") + $args
    }
    $args += $Url

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
    if ($exitCode -ne 0 -and -not $AllowFailure) {
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

function Read-AgentHeader {
    param(
        [string]$FileName,
        [string]$HeaderName
    )

    $path = Join-Path $EvidencePath $FileName
    if (-not (Test-Path $path)) {
        return ""
    }
    $match = [regex]::Match((Get-Content -Raw -Encoding UTF8 $path), "(?im)^$([regex]::Escape($HeaderName)):\s*(.+)$")
    if ($match.Success) {
        return $match.Groups[1].Value.Trim()
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
        Invoke-Step "Remove S3 helper-created containers" {
            [void](Invoke-DockerCapture "cleanup-rm-agent.txt" @("rm", "-f", $AgentName) "docker rm -f $AgentName" -AllowFailure)
            [void](Invoke-DockerCapture "cleanup-rm-candidate.txt" @("rm", "-f", $CandidateName) "docker rm -f $CandidateName" -AllowFailure)
            [void](Invoke-DockerCapture "containers-after-cleanup.txt" (DockerContainerListArgs) "docker ps -a <helper-filter>")
        }
        return
    }

    Invoke-Step "Collect initial Docker state" {
        [void](Invoke-DockerCapture "docker-version.txt" @("version") "docker version")
        [void](Invoke-DockerCapture "containers-before.txt" (DockerContainerListArgs) "docker ps -a <helper-filter>")
    }

    Invoke-Step "Pull Agent and candidate images" {
        [void](Invoke-DockerCapture "agent-image-pull.txt" @("pull", $AgentImage) "docker pull $AgentImage")
        [void](Invoke-DockerCapture "candidate-image-pull.txt" @("pull", $CandidateImage) "docker pull $CandidateImage")
    }

    Invoke-Step "Start local Agent container" {
        [void](Invoke-DockerCapture "preclean-agent.txt" @("rm", "-f", $AgentName) "docker rm -f $AgentName" -AllowFailure)
        [void](Invoke-DockerCapture "agent-run.txt" @(
            "run", "-d",
            "--name", $AgentName,
            "--label", "$LabelKey=$LabelValue",
            "--label", "com.portainer-cn.platform.runtime-role=agent",
            "-p", "$AgentHostPort`:9001",
            "-v", "/var/run/docker.sock:/var/run/docker.sock",
            "-v", "/var/lib/docker/volumes:/var/lib/docker/volumes",
            $AgentImage
        ) "docker run -d --name $AgentName --label $LabelKey=$LabelValue -p $AgentHostPort`:9001 -v /var/run/docker.sock:/var/run/docker.sock -v /var/lib/docker/volumes:/var/lib/docker/volumes $AgentImage")
        Start-Sleep -Seconds 5
        [void](Invoke-DockerCapture "agent-inspect.json" @("inspect", $AgentName) "docker inspect $AgentName")
        [void](Invoke-DockerCapture "agent-logs.txt" @("logs", "--tail", "120", $AgentName) "docker logs --tail 120 $AgentName" -AllowFailure)
    }

    Invoke-Step "Ping local Agent endpoint" {
        [void](Test-Http "https://127.0.0.1:$AgentHostPort/ping" "agent-ping.txt" -InsecureTls)
    }

    Invoke-Step "Create candidate with random host port and verify backend reachability" {
        [void](Invoke-DockerCapture "preclean-candidate.txt" @("rm", "-f", $CandidateName) "docker rm -f $CandidateName" -AllowFailure)
        [void](Invoke-DockerCapture "candidate-run.txt" @(
            "run", "-d",
            "--name", $CandidateName,
            "--label", "$LabelKey=$LabelValue",
            "--label", "com.portainer-cn.platform.runtime-role=candidate",
            "-P",
            $CandidateImage
        ) "docker run -d --name $CandidateName --label $LabelKey=$LabelValue -P $CandidateImage")
        $candidatePort = Get-HostPort $CandidateName 80
        Set-Content -Path (Join-Path $EvidencePath "candidate-port.txt") -Encoding UTF8 -Value @(
            "container=$CandidateName",
            "container_port=80",
            "host_port=$candidatePort",
            "healthcheck_host=127.0.0.1"
        )
        [void](Invoke-DockerCapture "candidate-inspect.json" @("inspect", $CandidateName) "docker inspect $CandidateName")
        [void](Test-Http "http://127.0.0.1:$candidatePort/" "candidate-healthcheck-from-portainer-host.txt")
    }

    Invoke-Step "Stop and remove candidate and Agent" {
        [void](Invoke-DockerCapture "candidate-rm.txt" @("rm", "-f", $CandidateName) "docker rm -f $CandidateName" -AllowFailure)
        [void](Invoke-DockerCapture "agent-rm.txt" @("rm", "-f", $AgentName) "docker rm -f $AgentName" -AllowFailure)
        [void](Invoke-DockerCapture "containers-after.txt" (DockerContainerListArgs) "docker ps -a <helper-filter>")
    }

    Invoke-Step "Write local Agent summary" {
        $agentStatus = Read-StatusCode "agent-ping.txt"
        $agentVersion = Read-AgentHeader "agent-ping.txt" "Portainer-Agent"
        $agentPlatform = Read-AgentHeader "agent-ping.txt" "Portainer-Agent-Platform"
        $candidateStatus = Read-StatusCode "candidate-healthcheck-from-portainer-host.txt"
        $passed = ($agentStatus -eq "204" -and $agentVersion -and $agentPlatform -eq "1" -and $candidateStatus -eq "200")
        $summary = @(
            "checked_at=$((Get-Date).ToString('s'))",
            "agent_image=$AgentImage",
            "agent_endpoint=https://127.0.0.1:$AgentHostPort",
            "agent_ping_status=$agentStatus",
            "agent_version=$agentVersion",
            "agent_platform=$agentPlatform",
            "node_name_default=blank-for-single-local-agent",
            "candidate_health_status=$candidateStatus",
            "candidate_healthcheck_host=127.0.0.1",
            "conclusion=$(if ($passed) { "LOCAL_AGENT_SPIKE_PASSED" } else { "LOCAL_AGENT_SPIKE_INCOMPLETE" })"
        )
        Set-Content -Path (Join-Path $EvidencePath "local-agent-summary.txt") -Encoding UTF8 -Value $summary
    }

    Write-Host ""
    Write-Host "Local Agent Spike completed."
} finally {
    Stop-Evidence
}
