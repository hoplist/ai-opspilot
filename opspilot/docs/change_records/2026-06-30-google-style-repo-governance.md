# 2026-06-30 Google-Style Repository Governance Alignment

## Goal

Align OpsPilot repository governance with large-company source-of-truth
practices without introducing a heavyweight monorepo or restructuring GitLab
immediately.

The immediate objective is to make `repo preflight` detect repository class,
business repository boundaries, and mutable deployment image tags before a
repository enters the automated release path.

## Decisions

- Keep GitLab as the human-maintained desired-state source for repositories,
  OpsPilot config, skills, CI templates, and GitOps manifests.
- Do not require a Google-style monorepo. OpsPilot keeps multiple repositories
  but gives each repository a clear class and boundary.
- Recommended repository classes:
  - `tpo/apps/...` for business application source.
  - `tpo/platform/...` for OpsPilot/platform code.
  - `tpo/deploy/...` for GitOps desired state.
  - `tpo/shared/...` for CI, Dockerfile, and service templates.
  - `tpo/ops/...` for operational assets and backups.
  - `tpo/sandbox/...` for disposable test/demo repositories.
- Legacy paths such as `tpo/devex/...` remain tolerated during migration and
  only produce warnings.
- Business repositories may temporarily contain generated `deploy/k8s` starter
  manifests, but long-term live deployment state belongs in GitOps/config
  repositories.
- Application deployment manifests must not use mutable `:latest` image tags.
  Standard release should write commit tags or digests.

## Implemented

- Added repository governance checks to `repo preflight`:
  - `repo_class`
  - `business_repo_boundary`
  - `immutable_image_tag`
- Added compatibility for the target platform path
  `tpo/platform/opspilot/opspilot-core` when direct BuildKit CI is used.
- Added tests for:
  - recommended `tpo/apps/...` app path.
  - legacy app path warning.
  - app deployment manifest using `:latest`.
- Updated CLI schema description for `repo preflight`.
- Added ADR for GitLab repository classes and source boundaries.

## Not Changed

- No GitLab group or project was moved in this code release.
- No GitLab permission, deploy token, CI variable, Argo CD source URL, or
  registry path was changed.
- No live cluster resource was modified.
- `deploy/k8s` in business repositories is not removed yet because current
  onboarding still uses it as a beginner-friendly generated starter.

## GitLab Landing Scope

The user approved direct GitLab changes after the design review. The safe
landing order is:

1. publish the preflight guardrails through the standard node206 GitLab Runner
   -> BuildKit -> Registry -> GitOps -> Argo CD flow;
2. refresh GitLab metadata and descriptions where needed;
3. run `repo preflight` against candidate repositories;
4. only then migrate high-impact paths such as `platform/opspilot`,
   `platform/gitops-manifests`, and `platform/opspilot-skills`.

Core path migration remains a separate operation because current runtime config
still references `platform/opspilot` and `platform/gitops-manifests`.

## Risk Boundary

- Path governance warnings do not block current test workflows.
- `:latest` is a blocker for application/sandbox repositories because it makes
  rollback and evidence correlation ambiguous.
- `:latest` is warning-only for platform/deploy/shared/ops repositories until
  their migration path is explicitly reviewed.

## Minimum Validation

- `go test ./cli ./internal/assets ./internal/configloader`
- `go test ./...`
- `go vet ./...`
- `go run ./cli --output human config validate --dir ./config/opspilot-config`
- `git diff --check`

## Rollback

- Revert the changes in `cli/repo_preflight.go`, `cli/repo_test.go`,
  `contracts/cli-schema.json`, and the newly added governance docs.
- No external state needs rollback because this stage does not mutate GitLab,
  registry, GitOps, Argo CD, or Kubernetes.
