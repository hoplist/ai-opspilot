# 2026-06-02 CLI Image And Log Polish

## Background

After the standard release flow retest, OpsPilot correctly deployed the new
GitOps image, but Kubernetes `containerStatuses.image` could still show the
previous tag when the previous and current tags pointed to the same image
digest. That can confuse users into thinking the rollout did not happen.

The local wrapper also reported that `build/opspilot.exe` was older than the
CLI source, and service inspection output described missing logs as raw
capability gaps instead of a simple user-facing degradation.

## Scope

- Rebuild the local Windows CLI binary after source changes.
- Expose image evidence more clearly:
  - desired/spec image;
  - runtime status image;
  - runtime image digest/id.
- Improve missing log evidence output:
  - distinguish missing ELK from empty Kubernetes logs;
  - explain that Pod, event, metric, and release checks remain usable;
  - make multi-container Pod log errors easier to act on.

## Expected Validation

- `go test ./opspilot/...`
- `.\opspilot\scripts\build-cli.ps1`
- `.\opspilot\scripts\opspilot.ps1 --version`
- Standard release flow:
  node206 GitLab -> node206 GitLab Runner -> BuildKit rootless -> GitLab
  Registry -> GitOps -> node200 Argo CD.

## Implemented

- Rebuilt the local Windows CLI binary at `build/opspilot.exe`.
- Added `--version` / `version` CLI output.
- Added Pod container image evidence fields:
  - `spec_image`;
  - `status_image`;
  - `image_id`.
- Kept the existing `image` field for compatibility, while preferring
  `spec_image` for rollout evidence.
- Updated `inspect pod` human output to show container image evidence and an
  explanation when Kubernetes status shows a different tag from Pod spec.
- Updated `inspect service` human output to include the primary image tag in
  the Pod table.
- Updated Kubernetes log reads to default to the first Pod container when the
  caller does not specify a container, reducing multi-container Pod log errors.
- Improved log evidence findings so missing Kubernetes/ELK logs are reported as
  degraded evidence instead of a hard service failure.

## Local Validation

2026-06-02:

```powershell
git diff --check
go test ./opspilot/...
.\opspilot\scripts\build-cli.ps1
.\opspilot\scripts\opspilot.ps1 --version
.\opspilot\scripts\opspilot.ps1 --output human inspect service --service opspilot-core
```

The rebuilt CLI no longer triggers the stale binary wrapper warning. The
service inspection output now explains missing log evidence as a partial
degradation while keeping Pod, event, metric, and release checks usable.
