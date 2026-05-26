# 2026-05-26 Simple service entrypoints

## Background

The current OpsPilot workflow is technically complete enough for operators,
but still exposes too many platform details to ordinary developers:
Dockerfile, GitLab CI, BuildKit, GitOps paths, Argo CD applications,
namespaces, Pod names, PromQL, and log backends.

The next simplification milestone is to make the main user model service-first:

```text
onboard repository
release service
inspect service
```

Users should not need to know which Pod, namespace, GitOps file, or GitLab job
is involved for the first round of release and troubleshooting.

## Target Commands

```powershell
opspilot onboard repo tpo/devex/skillshub/skillshub-api --write --output human
opspilot release service skillshub-api --output human
opspilot inspect service skillshub-api --output human
```

Short form is also valid for release:

```powershell
opspilot release skillshub-api --output human
```

## Implemented In This Change

- Added `onboard repo` as a beginner entrypoint over the existing repository
  detection and generated platform files.
- `onboard repo` detects language, port, namespace, middleware intent, and
  release mapping from the GitLab project path.
- With `--write`, `onboard repo` writes the platform-owned files and then runs
  the same onboarding preflight check used by the lower-level commands.
- Added `release service` as a service-level release evidence summary.
- Added `release <service>` shorthand for `release service <service>`.
- `release service` aggregates the existing release status, latest GitLab jobs,
  and GitOps release history when those evidence sources are configured.
- Added `inspect service` as the main troubleshooting entrypoint for ordinary
  users.
- `inspect service` starts from the release service mapping, finds matching
  Pods, then summarizes status, restarts, CPU, memory, Kubernetes log presence,
  ELK log presence, release gaps, and evidence gaps.
- Bumped CLI/Core schema version to
  `0.1.18-release-trigger-natural-language` after the trigger and natural
  language follow-up.

## Current Boundary

The first service entrypoint pass made `release service` read-only. The follow-up
in the same milestone removed that boundary by adding an OpsPilot release
trigger endpoint.

The release trigger still follows the standard platform path:

```text
OpsPilot API
-> node206 GitLab pipeline
-> node206 GitLab Runner
-> BuildKit rootless image build
-> Registry push
-> GitOps update
-> node200 Argo CD deploy
```

## Follow-up Completion

Added:

```text
POST /api/release/trigger
opspilot release service <service> --trigger --ref main
opspilot release trigger --service <service> --ref main
opspilot ask "检查 opspilot-core 是否正常"
opspilot ask "发布 opspilot-core" --dry-run
```

The natural-language entrypoint is intentionally deterministic. It maps a small
set of Chinese/English intents onto stable service-level commands:

- 检查 / 排查 / 状态 -> `inspect service`
- 发布 / 上线 / release / deploy -> `release service --trigger`
- 历史 / 发布记录 -> `release history`
- 回退 / rollback -> `release rollback`

It first reads configured release services from OpsPilot health evidence, then
falls back to extracting service-like names such as `opspilot-core`.

## Runtime Requirement

Release trigger, GitLab job evidence, GitOps history, and rollback require
`opspilot-release-secrets` to provide:

```text
OPSPILOT_GITLAB_TOKEN
```

The token must have enough GitLab API permissions to create pipelines for the
service project and read/write the configured GitOps repository when rollback
is used.

## Validation

Ran:

```powershell
go test ./opspilot/...
.\opspilot\scripts\build-cli.ps1
```

Smoke checked against the current OpsPilot backend:

```powershell
.\opspilot\scripts\opspilot.ps1 --output human release service opspilot-core
.\opspilot\scripts\opspilot.ps1 --output human inspect service opspilot-core
.\opspilot\scripts\opspilot.ps1 --output human ask "检查 opspilot-core 是否正常"
.\opspilot\scripts\opspilot.ps1 --output human ask "发布 opspilot-core" --dry-run
```
