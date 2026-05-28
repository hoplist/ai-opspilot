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
    $useBinary = Test-Path $binaryPath
    if ($useBinary) {
        $binary = Get-Item $binaryPath
        $sourceRoots = @(
            (Join-Path $repoRoot "opspilot\cli"),
            (Join-Path $repoRoot "opspilot\internal"),
            (Join-Path $repoRoot "opspilot\contracts")
        )
        $sourceFiles = foreach ($root in $sourceRoots) {
            if (Test-Path $root) {
                Get-ChildItem -Path $root -Recurse -File |
                    Where-Object { @(".go", ".json", ".yaml", ".yml") -contains $_.Extension }
            }
        }
        $latestSource = $sourceFiles | Sort-Object LastWriteTimeUtc -Descending | Select-Object -First 1
        if ($latestSource -and $latestSource.LastWriteTimeUtc -gt $binary.LastWriteTimeUtc) {
            Write-Warning "build\opspilot.exe is older than OpsPilot CLI source; using go run for this workspace."
            $useBinary = $false
        }
    }

    if ($useBinary) {
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
