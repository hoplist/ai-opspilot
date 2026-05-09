param(
    [string]$BundleRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path,
    [switch]$Apply
)

$ErrorActionPreference = "Stop"

$GitOpsRoot = Join-Path $BundleRoot "gitops-manifests"

function Step {
    param([string]$Title, [string[]]$Commands)
    Write-Host ""
    Write-Host "==> $Title"
    foreach ($cmd in $Commands) {
        Write-Host "  $cmd"
        if ($Apply) {
            Invoke-Expression $cmd
        }
    }
}

Write-Host "BundleRoot: $BundleRoot"
Write-Host "GitOpsRoot: $GitOpsRoot"
Write-Host "Apply:      $Apply"

Step "1. Check Kubernetes connection" @(
    "kubectl get nodes -o wide"
)

Step "2. Deploy MySQL" @(
    "cd `"$GitOpsRoot\source\deploy\db\mysql-31326`"",
    "kubectl apply -k ."
)

Step "3. Deploy Prometheus" @(
    "cd `"$GitOpsRoot\source\yaml\monitoring\prometheus`"",
    "powershell -ExecutionPolicy Bypass -File .\install.ps1"
)

Step "4. Deploy Argo CD Applications" @(
    "cd `"$GitOpsRoot`"",
    "kubectl apply -f apps\argocd-bootstrap-project.yaml",
    "kubectl apply -f apps\argocd-bootstrap-application.yaml",
    "kubectl apply -f apps\argocd-core-project.yaml",
    "kubectl apply -f apps\argocd-core-application.yaml"
)

Step "5. Deploy Observability/RCA Application" @(
    "cd `"$GitOpsRoot`"",
    "kubectl apply -f apps\observability-project.yaml",
    "kubectl apply -f apps\observability-application.yaml"
)

Step "6. Verify" @(
    "kubectl get application -n observability",
    "kubectl get pods -n observability -o wide",
    "kubectl get pods -n observability -o wide"
)

if (-not $Apply) {
    Write-Host ""
    Write-Host "Preview mode only. Add -Apply to execute the commands."
}
