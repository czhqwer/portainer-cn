param(
    [switch]$Apply,
    [switch]$Cleanup,
    [switch]$RecordAllContainers,
    [string]$Image = "nginx:alpine",
    [string]$ProbeImage = "curlimages/curl:8.10.1",
    [string]$DockerCliImage = "docker:27-cli",
    [string]$EvidenceRoot = "docs/spike/evidence/gate0b/S2-containerized-docker"
)

$ErrorActionPreference = "Stop"

$LabelKey = "com.portainer-cn.platform.spike"
$LabelValue = "gate0b"
$Prefix = "pcn-spike-gate0b-s2"
$CandidateName = "$Prefix-candidate"
$EvidencePath = Join-Path (Get-Location) $EvidenceRoot
$script:TranscriptStarted = $false
$TranscriptPath = Join-Path $EvidencePath "transcript.txt"

function Write-Header {
    Write-Host "Gate 0B containerized Docker socket Spike helper"
    Write-Host "Apply mode: $Apply"
    Write-Host "Cleanup mode: $Cleanup"
    Write-Host "RecordAllContainers: $RecordAllContainers"
    Write-Host "Image: $Image"
    Write-Host "ProbeImage: $ProbeImage"
    Write-Host "DockerCliImage: $DockerCliImage"
    Write-Host "EvidenceRoot: $EvidenceRoot"
    Write-Host ""
}

function Start-Evidence {
    if (-not $Apply) {
        return
    }

    New-Item -ItemType Directory -Force -Path $EvidencePath | Out-Null
    Set-Content -Path (Join-Path $EvidencePath "metadata.txt") -Encoding UTF8 -Value @(
        "scenario=S2-containerized-docker",
        "image=$Image",
        "probe_image=$ProbeImage",
        "docker_cli_image=$DockerCliImage",
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

function Invoke-Docker {
    param([string[]]$DockerArgs)

    Write-Host "docker $($DockerArgs -join ' ')"
    if ($Apply) {
        Add-CommandRecord "docker $($DockerArgs -join ' ')"
        $previousErrorActionPreference = $ErrorActionPreference
        $ErrorActionPreference = "Continue"
        try {
            $output = & docker @DockerArgs 2>&1
            $exitCode = $LASTEXITCODE
        } finally {
            $ErrorActionPreference = $previousErrorActionPreference
        }
        $text = ($output | Out-String)
        if ($text.Trim()) {
            Write-Host $text.TrimEnd()
        }
        if ($exitCode -ne 0) {
            throw "docker $($DockerArgs -join ' ') failed with exit code $exitCode"
        }
    }
}

function Invoke-DockerCapture {
    param(
        [string]$FileName,
        [string[]]$DockerArgs,
        [switch]$AllowFailure
    )

    Write-Host "docker $($DockerArgs -join ' ')"
    if (-not $Apply) {
        return 0
    }

    Add-CommandRecord "docker $($DockerArgs -join ' ')"
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
        throw "docker $($DockerArgs -join ' ') failed with exit code $exitCode"
    }

    return $exitCode
}

function DockerContainerListArgs {
    if ($RecordAllContainers) {
        return @("ps", "-a", "--format", "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}")
    }

    return @("ps", "-a", "--filter", "label=$LabelKey=$LabelValue", "--format", "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}")
}

function Save-DockerOutput {
    param(
        [string]$FileName,
        [string[]]$DockerArgs
    )

    [void](Invoke-DockerCapture $FileName $DockerArgs)
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

function Get-BridgeGateway {
    $gateway = & docker network inspect bridge --format "{{(index .IPAM.Config 0).Gateway}}"
    if (-not $gateway) {
        return ""
    }

    return "$gateway".Trim()
}

function Test-ContainerHttp {
    param(
        [string]$Name,
        [string]$Url,
        [switch]$AllowFailure
    )

    $fileName = "probe-$Name.txt"
    $args = @(
        "run", "--rm",
        "--label", "$LabelKey=$LabelValue",
        $ProbeImage,
        "-sS", "-L", "--max-time", "8",
        "-o", "-",
        "-w", "`nstatus=%{http_code}`n",
        $Url
    )
    $exitCode = Invoke-DockerCapture $fileName $args -AllowFailure:$AllowFailure
    $content = Get-Content -Raw -Encoding UTF8 (Join-Path $EvidencePath $fileName)
    $statusMatch = [regex]::Match($content, "status=(\d+)")
    $status = ""
    if ($statusMatch.Success) {
        $status = $statusMatch.Groups[1].Value
    }

    return [pscustomobject]@{
        Name = $Name
        Url = $Url
        ExitCode = $exitCode
        Status = $status
        Passed = ($exitCode -eq 0 -and $status -eq "200")
    }
}

Write-Header

if (-not $Apply) {
    Write-Host "This script is in dry-run mode. Re-run with -Apply to execute Docker commands."
    Write-Host "Use -Cleanup -Apply to remove containers created by this helper."
}

Start-Evidence

try {
    if ($Cleanup) {
        Invoke-Step "Remove S2 helper-created containers" {
            [void](Invoke-DockerCapture "cleanup-rm-candidate.txt" @("rm", "-f", $CandidateName) -AllowFailure)
            Save-DockerOutput "containers-after-cleanup.txt" (DockerContainerListArgs)
        }
        return
    }

    Invoke-Step "Collect Docker version and context" {
        Save-DockerOutput "docker-version.txt" @("version")
        Save-DockerOutput "docker-context.txt" @("context", "ls")
        Save-DockerOutput "containers-before.txt" (DockerContainerListArgs)
    }

    Invoke-Step "Pull public image and probe images" {
        Invoke-Docker @("pull", $Image)
        Invoke-Docker @("pull", $ProbeImage)
        Invoke-Docker @("pull", $DockerCliImage)
    }

    Invoke-Step "Verify Docker socket from a containerized backend surrogate" {
        Save-DockerOutput "container-docker-socket.txt" @(
            "run", "--rm",
            "--label", "$LabelKey=$LabelValue",
            "-v", "/var/run/docker.sock:/var/run/docker.sock",
            $DockerCliImage,
            "docker", "version"
        )
    }

    Invoke-Step "Create candidate with random host port" {
        [void](Invoke-DockerCapture "preclean-candidate.txt" @("rm", "-f", $CandidateName) -AllowFailure)
        Invoke-Docker @(
            "run", "-d",
            "--name", $CandidateName,
            "--label", "$LabelKey=$LabelValue",
            "--label", "com.portainer-cn.platform.runtime-role=candidate",
            "-P",
            $Image
        )
        $candidatePort = Get-HostPort $CandidateName 80
        Write-Host "candidate_host_port=$candidatePort"
        Save-DockerOutput "candidate-inspect.json" @("inspect", $CandidateName)
        Set-Content -Path (Join-Path $EvidencePath "candidate-port.txt") -Encoding UTF8 -Value @(
            "container=$CandidateName",
            "container_port=80",
            "host_port=$candidatePort"
        )
    }

    Invoke-Step "Probe candidate host port from containerized backend surrogate" {
        $candidatePortText = Get-Content -Raw -Encoding UTF8 (Join-Path $EvidencePath "candidate-port.txt")
        $candidatePort = [regex]::Match($candidatePortText, "host_port=(\d+)").Groups[1].Value
        $bridgeGateway = Get-BridgeGateway
        Set-Content -Path (Join-Path $EvidencePath "bridge-network.txt") -Encoding UTF8 -Value @(
            "bridge_gateway=$bridgeGateway"
        )

        $results = @()
        $results += Test-ContainerHttp "loopback" "http://127.0.0.1:$candidatePort/" -AllowFailure
        if ($bridgeGateway) {
            $results += Test-ContainerHttp "bridge-gateway" "http://$bridgeGateway`:$candidatePort/" -AllowFailure
        }
        $results += Test-ContainerHttp "host-docker-internal" "http://host.docker.internal:$candidatePort/" -AllowFailure

        $summary = @(
            "checked_at=$((Get-Date).ToString('s'))",
            "candidate_host_port=$candidatePort",
            "bridge_gateway=$bridgeGateway",
            ""
        )
        foreach ($result in $results) {
            $summary += "$($result.Name)|url=$($result.Url)|exit_code=$($result.ExitCode)|status=$($result.Status)|passed=$($result.Passed)"
        }

        $passed = @($results | Where-Object { $_.Passed })
        $summary += ""
        $summary += "reachable_candidates=$($passed.Count)"
        if ($passed.Count -eq 0) {
            $summary += "conclusion=NO_CONTAINERIZED_HEALTHCHECK_HOST_REACHABLE"
            Write-Warning "No candidate health URL was reachable from the containerized backend surrogate."
        } else {
            $recommendedUrl = $passed[0].Url
            $recommendedHost = ([uri]$recommendedUrl).Host
            $summary += "conclusion=CONTAINERIZED_HEALTHCHECK_HOST_FOUND"
            $summary += "recommended_healthcheck_url=$recommendedUrl"
            $summary += "recommended_healthcheck_host=$recommendedHost"
        }

        Set-Content -Path (Join-Path $EvidencePath "containerized-health-summary.txt") -Encoding UTF8 -Value $summary
    }

    Invoke-Step "Stop and remove candidate after validation" {
        Invoke-Docker @("rm", "-f", $CandidateName)
    }

    Invoke-Step "Collect final helper container state" {
        Save-DockerOutput "containers-after.txt" (DockerContainerListArgs)
    }

    Write-Host ""
    Write-Host "Containerized Docker Spike subset completed. Run with -Cleanup -Apply to remove helper-created containers if needed."
} finally {
    Stop-Evidence
}
