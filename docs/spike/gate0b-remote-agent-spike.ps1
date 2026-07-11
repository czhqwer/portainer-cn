param(
    [switch]$Apply,
    [switch]$InsecureTls,
    [string]$RemoteAgentURL = "",
    [string]$NodeName = "",
    [string]$CandidateHealthURL = "",
    [string]$BlockedCandidateHealthURL = "",
    [string]$EvidenceRoot = "docs/spike/evidence/gate0b/S4-remote-agent"
)

$ErrorActionPreference = "Stop"

$EvidencePath = Join-Path (Get-Location) $EvidenceRoot
$script:TranscriptStarted = $false
$TranscriptPath = Join-Path $EvidencePath "transcript.txt"

function Write-Header {
    Write-Host "Gate 0B remote Agent Spike helper"
    Write-Host "Apply mode: $Apply"
    Write-Host "InsecureTls: $InsecureTls"
    Write-Host "RemoteAgentURL: $RemoteAgentURL"
    Write-Host "NodeName provided: $([bool]$NodeName)"
    Write-Host "CandidateHealthURL provided: $([bool]$CandidateHealthURL)"
    Write-Host "BlockedCandidateHealthURL provided: $([bool]$BlockedCandidateHealthURL)"
    Write-Host "EvidenceRoot: $EvidenceRoot"
    Write-Host ""
}

function Start-Evidence {
    if (-not $Apply) {
        return
    }

    New-Item -ItemType Directory -Force -Path $EvidencePath | Out-Null
    Set-Content -Path (Join-Path $EvidencePath "metadata.txt") -Encoding UTF8 -Value @(
        "scenario=S4-remote-agent",
        "remote_agent_url=$RemoteAgentURL",
        "node_name_provided=$([bool]$NodeName)",
        "candidate_health_url_provided=$([bool]$CandidateHealthURL)",
        "blocked_candidate_health_url_provided=$([bool]$BlockedCandidateHealthURL)",
        "started_at=$((Get-Date).ToString('s'))"
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

function Test-Http {
    param(
        [string]$Url,
        [string]$FileName,
        [switch]$AllowFailure
    )

    $args = @("-sS", "-i", "-L", "--max-time", "10", "-o", "-", "-w", "`nstatus=%{http_code}`n")
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

function Join-URL {
    param(
        [string]$Base,
        [string]$Path
    )

    return "$($Base.TrimEnd('/'))/$($Path.TrimStart('/'))"
}

Write-Header

if (-not $Apply) {
    Write-Host "This script is in dry-run mode. Re-run with -Apply and a RemoteAgentURL to execute HTTP probes."
}

if ($Apply -and -not $RemoteAgentURL) {
    throw "RemoteAgentURL is required in -Apply mode."
}

Start-Evidence

try {
    Invoke-Step "Ping remote Agent endpoint" {
        [void](Test-Http (Join-URL $RemoteAgentURL "/ping") "remote-agent-ping.txt")
    }

    Invoke-Step "Probe remote candidate health URL when provided" {
        if ($CandidateHealthURL) {
            [void](Test-Http $CandidateHealthURL "candidate-health-allowed.txt" -AllowFailure)
        } else {
            Set-Content -Path (Join-Path $EvidencePath "candidate-health-allowed.txt") -Encoding UTF8 -Value @(
                "skipped=true",
                "reason=CandidateHealthURL not provided"
            )
        }
    }

    Invoke-Step "Probe blocked candidate health URL when provided" {
        if ($BlockedCandidateHealthURL) {
            [void](Test-Http $BlockedCandidateHealthURL "candidate-health-blocked.txt" -AllowFailure)
        } else {
            Set-Content -Path (Join-Path $EvidencePath "candidate-health-blocked.txt") -Encoding UTF8 -Value @(
                "skipped=true",
                "reason=BlockedCandidateHealthURL not provided"
            )
        }
    }

    Invoke-Step "Write remote Agent summary" {
        $agentStatus = Read-StatusCode "remote-agent-ping.txt"
        $agentVersion = Read-AgentHeader "remote-agent-ping.txt" "Portainer-Agent"
        $agentPlatform = Read-AgentHeader "remote-agent-ping.txt" "Portainer-Agent-Platform"
        $candidateStatus = Read-StatusCode "candidate-health-allowed.txt"
        $blockedStatus = Read-StatusCode "candidate-health-blocked.txt"

        $hasAgent = ($agentStatus -eq "204" -and $agentVersion -and $agentPlatform -eq "1")
        $hasCandidateProbe = [bool]$CandidateHealthURL
        $hasBlockedProbe = [bool]$BlockedCandidateHealthURL
        $passed = ($hasAgent -and $hasCandidateProbe -and $candidateStatus -eq "200")

        Set-Content -Path (Join-Path $EvidencePath "remote-agent-summary.txt") -Encoding UTF8 -Value @(
            "checked_at=$((Get-Date).ToString('s'))",
            "remote_agent_url=$RemoteAgentURL",
            "node_name=$NodeName",
            "agent_ping_status=$agentStatus",
            "agent_version=$agentVersion",
            "agent_platform=$agentPlatform",
            "candidate_health_url_provided=$hasCandidateProbe",
            "candidate_health_status=$candidateStatus",
            "blocked_candidate_health_url_provided=$hasBlockedProbe",
            "blocked_candidate_health_status=$blockedStatus",
            "expected_unreachable_reason=HEALTHCHECK_HOST_UNREACHABLE",
            "conclusion=$(if ($passed) { "REMOTE_AGENT_SPIKE_PASSED" } else { "REMOTE_AGENT_SPIKE_INCOMPLETE" })"
        )
    }

    Write-Host ""
    Write-Host "Remote Agent Spike probes completed."
} finally {
    Stop-Evidence
}
