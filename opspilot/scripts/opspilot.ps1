 [CmdletBinding(PositionalBinding = $false)]
param(
    [string]$BackendUrl = $env:OPSPILOT_BACKEND_URL,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$CliArgs
)

if ([string]::IsNullOrWhiteSpace($BackendUrl)) {
    $BackendUrl = "http://192.168.48.200:32180"
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$binaryPath = Join-Path $repoRoot "build\opspilot.exe"

Push-Location $repoRoot
try {
    if (Test-Path $binaryPath) {
        & $binaryPath --backend-url $BackendUrl @CliArgs
    }
    else {
        & go run ./opspilot/cli --backend-url $BackendUrl @CliArgs
    }
    exit $LASTEXITCODE
}
finally {
    Pop-Location
}
