# 2026-05-28 CLI Wrapper Stale Binary Guard

## Context

Post-release validation showed the node200 backend had the new skills registry,
but the local `opspilot.ps1` wrapper still preferred an older
`build\opspilot.exe`. That made `opspilot skills registry` fail locally even
though the backend and source code were already updated.

## Change

- The workspace wrapper now checks whether `build\opspilot.exe` is older than
  OpsPilot CLI, internal package, or contract source files.
- If the binary is stale, the wrapper falls back to `go run ./opspilot/cli` and
  prints a warning.
- Packaged users with only the binary and no source tree are unaffected.

## Intent

Developers and AI workflows should not accidentally test a stale CLI after code
changes. Release images still go through the standard GitLab Runner -> BuildKit
-> Registry -> GitOps -> Argo CD flow; local builds remain only for CLI
validation and distribution packaging.
