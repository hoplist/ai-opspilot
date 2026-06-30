# ADR-005: GitLab Repository Classes And Source Boundaries

## Status

Accepted

## Context

OpsPilot uses GitLab as the main place for application source, platform source,
GitOps manifests, runtime skills, CI templates, plain configuration, and
operational assets. The repository list became hard to understand because code
repositories, deploy repositories, skills, backups, and demo projects were
mixed together.

The goal is to align with large-company source-of-truth discipline while
keeping the current small-team/test-cluster operating model simple.

## Decision

Use explicit GitLab repository classes instead of introducing a single
monorepo:

- `tpo/apps/...`: business application source and tests.
- `tpo/platform/...`: OpsPilot and platform service source.
- `tpo/deploy/...`: GitOps desired state consumed by Argo CD.
- `tpo/shared/...`: CI, Dockerfile, service, and reusable templates.
- `tpo/ops/...`: operational assets and backups.
- `tpo/sandbox/...`: disposable demo and validation repositories.

OpsPilot `repo preflight` enforces this as a local governance check before
release automation:

- recommended classes pass.
- legacy paths warn but do not block.
- unknown paths warn and require classification.
- application manifests using mutable `:latest` image tags block release.
- starter `deploy/k8s` manifests in business repositories warn because GitOps
  remains the long-term live deployment source.

## Alternatives Considered

- **Google-style monorepo**: stronger consistency, but too disruptive for the
  current GitLab, GitOps, and small-team workflow.
- **Keep ad-hoc repositories**: zero migration cost, but users cannot quickly
  distinguish code, deploy state, skills, config, and backups.
- **Immediate GitLab restructuring**: cleaner end state, but risky because
  registry paths, CI includes, Argo CD URLs, deploy tokens, and release mappings
  must be changed together.

## Consequences

Positive:

- GitLab remains the human-readable source of truth.
- Repository purpose becomes machine-checkable.
- Existing workflows keep running while warnings guide migration.
- Future permissions, audit, and service catalog mapping have stable anchors.

Negative:

- During transition, legacy and recommended layouts coexist.
- Business repositories still temporarily contain generated deploy starter
  files until GitOps promotion is fully automated.

## Validation

The minimum validation for this ADR is:

- `repo preflight` reports `repo_class`.
- legacy paths produce warnings, not blockers.
- app deployments with `:latest` are blockers.
- test and vet gates pass.
