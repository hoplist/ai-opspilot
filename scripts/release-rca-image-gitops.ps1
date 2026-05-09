param(
    [string]$SourceRoot = "D:\code\auto_inspection",
    [string]$GitOpsRoot = "D:\code\auto_inspection\worktrees\gitops-manifests",
    [string]$LocalRegistry = "localhost:5002",
    [string]$ClusterRegistry = "192.168.48.1:5002",
    [string]$ImageName = "auto-inspection-rca",
    [string]$Tag = ("{0:yyyyMMdd-HHmmss}" -f (Get-Date)),
    [string]$ArgoApp = "observability",
    [string]$ArgoNamespace = "argocd",
    [string]$Namespace = "observability",
    [string]$Deployment = "auto-inspection-rca",
    [switch]$PlanOnly,
    [switch]$SkipDocker,
    [switch]$SkipGitOpsPatch,
    [switch]$Commit,
    [switch]$Push,
    [switch]$WaitArgo,
    [switch]$Verify
)

$ErrorActionPreference = "Stop"

function Write-Step {
    param([string]$Message)
    Write-Host ""
    Write-Host "==> $Message"
}

function Require-Command {
    param([string]$Name)
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "Required command not found: $Name"
    }
}

function Update-ImageInFile {
    param(
        [string]$Path,
        [string]$Image
    )

    if (-not (Test-Path $Path)) {
        throw "GitOps deployment file not found: $Path"
    }

    $content = Get-Content -Raw -Encoding UTF8 $Path
    $updated = $content -replace 'image:\s+\S+/auto-inspection-rca:\S+', "image: $Image"

    if ($updated -eq $content) {
        throw "No auto-inspection-rca image line was updated in: $Path"
    }

    Set-Content -Encoding UTF8 -NoNewline -Path $Path -Value $updated
}

$localImage = "$LocalRegistry/$ImageName`:$Tag"
$clusterImage = "$ClusterRegistry/$ImageName`:$Tag"
$gitopsFiles = @(
    "clusters/test/observability/auto-inspection-rca/deployment.yaml",
    "source/deploy/rca-service/deployment.yaml",
    "source/yaml/rca-service/deployment.yaml"
)

Write-Step "Release plan"
Write-Host "SourceRoot:     $SourceRoot"
Write-Host "GitOpsRoot:     $GitOpsRoot"
Write-Host "Local image:    $localImage"
Write-Host "Cluster image:  $clusterImage"
Write-Host "Argo app:       $ArgoNamespace/$ArgoApp"
Write-Host "Deployment:     $Namespace/$Deployment"
Write-Host "Commit:         $Commit"
Write-Host "Push:           $Push"
Write-Host "WaitArgo:       $WaitArgo"
Write-Host "Verify:         $Verify"

if ($PlanOnly) {
    Write-Host ""
    Write-Host "PlanOnly enabled. No changes were made."
    exit 0
}

Require-Command git
Require-Command kubectl

if (-not $SkipDocker) {
    Require-Command docker

    Write-Step "Build RCA image"
    Push-Location $SourceRoot
    try {
        docker build -t $localImage .
        docker push $localImage
    }
    finally {
        Pop-Location
    }
}

if (-not $SkipGitOpsPatch) {
    Write-Step "Patch GitOps deployment image"
    foreach ($file in $gitopsFiles) {
        $path = Join-Path $GitOpsRoot $file
        Update-ImageInFile -Path $path -Image $clusterImage
        Write-Host "updated $file"
    }

    Write-Step "Validate GitOps render"
    Push-Location $GitOpsRoot
    try {
        kubectl kustomize clusters/test/observability | Out-Null
        kubectl apply --dry-run=server -k clusters/test/observability | Out-Null
        git diff -- $gitopsFiles
    }
    finally {
        Pop-Location
    }
}

if ($Commit) {
    Write-Step "Commit GitOps changes"
    Push-Location $GitOpsRoot
    try {
        git add $gitopsFiles
        git commit -m "Release RCA image $Tag"
        if ($Push) {
            git push origin main
        }
    }
    finally {
        Pop-Location
    }
}
elseif ($Push) {
    throw "-Push requires -Commit"
}

if ($WaitArgo) {
    Write-Step "Wait for Argo CD sync"
    Push-Location $GitOpsRoot
    try {
        $target = (git rev-parse HEAD).Trim()
    }
    finally {
        Pop-Location
    }

    $deadline = (Get-Date).AddMinutes(10)
    do {
        $status = kubectl get application $ArgoApp -n $ArgoNamespace -o jsonpath='{.status.sync.revision} {.status.sync.status} {.status.health.status}'
        Write-Host $status
        if ($status -match [regex]::Escape($target) -and $status -match "Synced" -and $status -match "Healthy") {
            break
        }
        Start-Sleep -Seconds 10
    } while ((Get-Date) -lt $deadline)

    if ((Get-Date) -ge $deadline) {
        throw "Timed out waiting for Argo CD app $ArgoApp to sync revision $target"
    }
}

if ($Verify) {
    Write-Step "Verify deployment and health"
    kubectl rollout status "deployment/$Deployment" -n $Namespace --timeout=240s

    $images = kubectl get deploy $Deployment -n $Namespace -o jsonpath='{.spec.template.spec.containers[*].image}'
    Write-Host "deployment images: $images"
    if ($images -notmatch [regex]::Escape($clusterImage)) {
        throw "Deployment does not reference expected image: $clusterImage"
    }

    $pod = kubectl get pods -n $Namespace -l app.kubernetes.io/name=$Deployment -o jsonpath='{.items[0].metadata.name}'
    Write-Host "pod: $pod"
    kubectl exec -n $Namespace $pod -c backend -- sh -lc 'test -f /opt/rca/backend_server.py && test -f /opt/rca/docs/cn/deployment/rca-backend-mcp-image-deployment.md && echo image-content-ok'

    $health = Invoke-WebRequest -UseBasicParsing -TimeoutSec 30 -Uri "http://192.168.48.200:32180/api/health"
    Write-Host "backend health status: $($health.StatusCode)"
}

Write-Step "Done"
Write-Host "Released image: $clusterImage"
