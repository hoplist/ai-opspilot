 [CmdletBinding()]
param(
    [string]$Output = "",
    [string]$TargetOS = $env:GOOS,
    [string]$TargetArch = $env:GOARCH
)

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path

if ([string]::IsNullOrWhiteSpace($TargetOS)) {
    $TargetOS = "windows"
}
if ([string]::IsNullOrWhiteSpace($TargetArch)) {
    $TargetArch = "amd64"
}
if ([string]::IsNullOrWhiteSpace($Output)) {
    $ext = ""
    if ($TargetOS -eq "windows") {
        $ext = ".exe"
    }
    $Output = Join-Path $repoRoot ("build\opspilot-" + $TargetOS + "-" + $TargetArch + $ext)
    if ($TargetOS -eq "windows" -and $TargetArch -eq "amd64") {
        $Output = Join-Path $repoRoot "build\opspilot.exe"
    }
}

New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Output) | Out-Null

Push-Location $repoRoot
try {
    $env:GOOS = $TargetOS
    $env:GOARCH = $TargetArch
    $env:CGO_ENABLED = "0"
    & go build -trimpath -ldflags "-s -w" -o $Output ./opspilot/cli
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
    Write-Host "Built $Output"
}
finally {
    Pop-Location
}
