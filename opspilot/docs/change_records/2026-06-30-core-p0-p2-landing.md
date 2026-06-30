# Core P0-P2 Landing

Date: 2026-06-30

## Scope

This stage lands the remaining core changes without a separate review gate.

## P0: OpsPilot Core Repository Move

Target:

```text
platform/opspilot
-> tpo/platform/opspilot/opspilot-core
```

Actions:

- Created local git bundle backup before GitLab mutation:
  `D:\code\auto_inspection\backups\opspilot-before-core-move-*.bundle`.
- Tried GitLab project transfer first.
- Transfer returned `400` and the project had many Container Registry tags, so
  direct transfer was not used.
- Created new GitLab project:
  `tpo/platform/opspilot/opspilot-core`.
- Copied masked `GITOPS_TOKEN` CI/CD variable from the old project to the new
  project without printing token values.
- Copied the core CI base images to the new project registry:
  - `tpo/platform/opspilot/opspilot-core/ci-alpine:3.20`;
  - `tpo/platform/opspilot/opspilot-core/ci-golang:1.23-alpine`;
  - `tpo/platform/opspilot/opspilot-core/ci-buildkit:rootless`.
- Updated source references that represent GitLab project identity:
  - `.gitlab-ci.yml` repo precheck project;
  - `config/opspilot-config/services/platform/opspilot-core.yaml`;
  - `config/opspilot-config/credentials/platform.yaml`;
  - `config/opspilot-config/.gitlab-ci.yml`;
  - current docs and tests.

Boundary:

- Registry image paths remain under `192.168.48.206:5050/platform/opspilot/*`
  for this stage. Moving registry paths is separated because it affects CI base
  images, imagePullSecrets, rollback history, and offline-kit.
- Old `platform/opspilot` remains as compatibility and registry-history holder.

## P1: Release Reconcile And Catalog First

Actions:

- `release status` now emits `evidence.reconcile`.
- When GitOps desired image differs from the cluster image:
  - status moves from `healthy` to `progressing`;
  - stage becomes `argocd`;
  - output explains whether Argo CD is likely stale/pending and what to do.
- Service catalog now wins over legacy `OPSPILOT_RELEASE_SERVICES`.
- Legacy env mapping remains only as fallback when no service catalog is
  configured.

## P2: Productization Follow-Up

Recorded as active productization direction:

- Observability adapters stay config-driven and optional.
- Missing Prometheus/ELK/OpenSearch/OpenObserve/APISIX evidence must remain
  explicit and non-blocking unless the service policy marks it required.
- High-risk actions stay plan-first unless an explicit confirmed endpoint is
  added with audit and minimum validation.
- Offline-kit and private registry migration remain separate because they need
  image inventory and install validation.

## P3: Recorded Only

Not implemented in this stage:

- Full registry path migration from `platform/opspilot/*` to
  `tpo/platform/opspilot/opspilot-core/*`.
- Argo CD automatic hard-refresh endpoint.
- Formal authentication/approval workflow.
- Full multi-cluster remote execution rollout.
- Full ES/Kibana datasource routing across all regions.

## Minimum Validation

- `go test ./internal/release ./internal/catalog ./core ./cli`
- `go test ./...`
- `go vet ./...`
- `go run ./cli --output human config validate --dir ./config/opspilot-config`
- `repo preflight`
- GitLab pipeline through the new project.
- `release status --service opspilot-core`

## Rollback

- Keep old `platform/opspilot` project and registry tags.
- Reset local `node206` remote to
  `http://192.168.48.206:8929/platform/opspilot.git`.
- Revert GitLab project path references in source/config/docs.
- Restore source from the git bundle if needed.
