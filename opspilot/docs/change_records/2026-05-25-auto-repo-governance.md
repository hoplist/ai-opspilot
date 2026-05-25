# Automated Repository Governance

Date: 2026-05-25

## Context

Many service repositories are created by vibecoding workflows and are not
consistent enough for reliable release automation. Common gaps include missing
Dockerfiles, local-only Dockerfiles, hand-written GitLab CI, missing
`deploy/k8s` manifests, unclear namespace ownership, and missing health checks.

The target model is fully automated. Developers should not need platform-owner
approval to release, but they also should not be able to bypass the standard
release path.

## Target Flow

```text
developer push
-> OpsPilot repository preflight
-> OpsPilot automatic repository fix when policy-safe
-> GitLab CI platform template
-> BuildKit rootless image build
-> CI Bot updates GitOps
-> node200 Argo CD deploys
-> OpsPilot verifies release status
-> OpsPilot rollback is available through GitOps
```

## Changes Implemented

- Added a repository governance preflight command that detects:
  - Dockerfile presence and risky/local-only patterns.
  - Whether `.gitlab-ci.yml` uses the platform BuildKit/GitOps template.
  - Kubernetes namespace, Deployment, Service, and Kustomize manifests.
  - namespace ownership from GitLab project path or namespace catalog.
  - runtime port and health check path.
- Added an automatic fix command that can generate or replace platform-managed
  files without a human approval step.
- Business repositories stay on a minimal `.gitlab-ci.yml` include while the
  actual BuildKit/GitOps logic stays in the platform CI template.
- Unsafe or cross-boundary changes are policy failures instead of waiting
  for manual approval.
- Added platform CI templates for Go, Node, and Python services:
  - `ci/templates/buildkit-gitops.go.yml`
  - `ci/templates/buildkit-gitops.node.yml`
  - `ci/templates/buildkit-gitops.python.yml`
- Bumped CLI schema to `0.1.14-auto-repo-governance`.

## Boundaries

- OpsPilot should not directly mutate Kubernetes for normal release or rollback.
- GitOps remains the deployment source of truth.
- Developers should not receive direct write permissions to GitOps.
- `GITOPS_TOKEN` stays in protected CI variables or the OpsPilot controlled
  environment, not in service repositories.
- Automatic repair should leave Git history and machine-readable evidence.

## Initial Commands

```powershell
opspilot repo preflight --repo . --project tpo/devex/demo/demo-api
opspilot repo autofix --repo . --project tpo/devex/demo/demo-api --write
opspilot repo preflight --repo . --project tpo/devex/demo/demo-api
```

Use `--force` when a repository already contains local-only Dockerfile, CI, or
manifest surfaces that should be replaced by the platform standard:

```powershell
opspilot repo autofix --repo . --project tpo/devex/demo/demo-api --write --force
```

## Verification

- Added unit tests for missing repository gaps, automated file generation, and
  forced replacement of a risky Dockerfile.
- Ran `go test ./opspilot/...`.
