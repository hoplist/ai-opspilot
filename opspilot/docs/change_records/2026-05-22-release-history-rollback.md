# Release History And GitOps Rollback

Date: 2026-05-22

## Context

Operators need OpsPilot to answer two release questions directly:

- What historical releases exist for a service?
- How can a deployed service be rolled back after a release?

Rollback must follow the agreed release path:

```text
node206 GitLab -> GitLab Runner -> BuildKit rootless -> Registry -> GitOps -> node200 Argo CD
```

It must not patch live Kubernetes resources directly.

## Changes

- Added `release history` to read GitOps commit history for a configured
  service manifest and extract the image/tag from each revision.
- Added `release rollback --confirm` to submit a GitOps commit that changes the
  configured Deployment container image.
- Rollback target can be a tag, full image, GitOps revision, or short GitOps
  revision.
- Added Core API routes:
  - `GET /api/release/history`
  - `POST /api/release/rollback`
- Added GitLab API support for GitOps file commit history and commit-based file
  updates.
- Updated CLI schema to `0.1.13-release-history-rollback`.
- Documented rollback boundaries and post-rollback verification steps.

## Boundaries

- OpsPilot does not build images during rollback.
- OpsPilot does not call `kubectl set image`, `kubectl rollout undo`,
  `kubectl apply`, or `argocd app sync`.
- Argo CD remains responsible for applying the GitOps commit to node200.
- Rollback requires an explicit `--confirm` flag.
- The configured GitLab token must have write access to the GitOps repository
  for rollback.

## Usage

```powershell
.\opspilot\scripts\opspilot.ps1 --output human release history --service opspilot-core --limit 10
.\opspilot\scripts\opspilot.ps1 --output human release rollback --service opspilot-core --to <tag-or-revision> --confirm
.\opspilot\scripts\opspilot.ps1 --output human release status --service opspilot-core
```
