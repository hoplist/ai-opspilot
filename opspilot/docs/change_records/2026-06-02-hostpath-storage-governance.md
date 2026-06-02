# 2026-06-02 HostPath storage governance

## Goal

Make OpsPilot-generated services safer on node200-like single-node or
hostPath-heavy clusters. Many real services still mount host directories for
logs, uploads, runtime files, or local cache. Kubernetes cannot apply reliable
per-directory disk limits to arbitrary hostPath volumes, so the first version
standardizes paths, generates storage intent automatically, and makes the risk
visible to OpsPilot preflight and future inspection.

## Decisions

- Developers should not hand-write raw hostPath paths for normal onboarding.
- OpsPilot generates storage intent in `opspilot.service.yaml`.
- OpsPilot translates that intent into Kubernetes volumes and volumeMounts.
- Platform-managed hostPath paths use:

```text
/data/opspilot/hostpath/<namespace>/<service>/<volume>
```

- Cache/temp storage should use `emptyDir.sizeLimit` instead of hostPath.
- Logs/runtime/uploads use hostPath in the first version, with metadata for
  soft governance.
- Repository preflight allows only platform-managed hostPath paths.
- Non-platform hostPath remains a blocker because it bypasses governance.

## Storage Intent

Generated `opspilot.service.yaml` may include:

```yaml
storage:
  logs:
    purpose: logs
    mode: hostPath
    mountPath: /app/logs
    hostPath: /data/opspilot/hostpath/cicd-devex-demo/demo-api/logs
    sizeHint: 10Gi
    retentionDays: 7
  cache:
    purpose: cache
    mode: emptyDir
    mountPath: /tmp/cache
    sizeLimit: 1Gi
```

## Detection Rules

The first version keeps detection conservative:

- `logs`, `LOG_DIR`, `LOG_PATH`, `/logs` -> logs hostPath.
- `upload`, `uploads`, `runtime`, `files`, `conversations` -> runtime
  hostPath.
- `cache`, `tmp`, `temp` -> `emptyDir` with `sizeLimit`.

If nothing is detected, no storage volume is generated.

## Generated Manifest Behavior

- Deployment annotations record:
  - `opspilot.io/storage-managed=true`
  - `opspilot.io/storage-hostpath-root=/data/opspilot/hostpath/...`
  - `opspilot.io/storage-soft-limit=<sizeHint summary>`
- HostPath volumes are generated only under the platform path prefix.
- EmptyDir volumes include `sizeLimit`.
- Storage is not added to ResourceQuota as a hard hostPath limit because that
  would be misleading. HostPath must be managed by directory monitoring or a
  future node agent.

## Preflight

Preflight now checks:

- Deployment hostPath paths under `/data/opspilot/hostpath/` are allowed with
  warning-level metadata checks.
- Any other hostPath path is still a blocker.
- `emptyDir` without `sizeLimit` is a blocker.
- Storage intent exists when generated deployment contains platform-managed
  storage metadata.

## Limits

This first version is soft governance. It does not enforce hard directory
quotas on hostPath. A future `opspilot-hostpath-agent` can add:

- per-directory size and inode reporting;
- log retention cleanup;
- optional XFS project quota for hard limits.

## Validation

- Generate a service with detected logs/cache/runtime storage.
- Verify generated Deployment has hostPath and emptyDir volumes.
- Verify non-platform hostPath fails `repo preflight`.
- Verify platform hostPath passes preflight.
- Run `go test ./opspilot/...`.

## Implemented

- `onboard detect` now records storage intent from repository signals.
- `onboard generate` and `repo autofix` now persist `storage:` in
  `opspilot.service.yaml`.
- Generated Deployments now include storage annotations, `volumeMounts`, and
  `volumes`.
- Platform hostPath uses
  `/data/opspilot/hostpath/<namespace>/<service>/<volume>`.
- `repo preflight` now blocks raw/non-platform hostPath and unbounded
  `emptyDir`.
- `onboard check` uses the same Deployment storage policy checks for local
  onboarding.
- `repo preflight` reports detected storage intent as evidence for AI review.

## Verified

2026-06-02:

```powershell
go test ./opspilot/...
git diff --check
```

Both commands passed.
