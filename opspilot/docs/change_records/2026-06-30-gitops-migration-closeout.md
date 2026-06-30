# GitOps Migration Closeout And Current Entry Docs

Date: 2026-06-30

## Goal

Close out the `tpo/deploy/gitops-manifests` migration and make the current
OpsPilot entry points easier to understand without reading all historical
change records.

## Changes

- Added `docs/current-state.md` as the current source of truth for:
  - source, config, skills, GitOps, and sandbox repositories;
  - standard release flow;
  - rollback command path;
  - credential catalog boundaries;
  - known non-blocking evidence gaps.
- Rewrote `docs/README.md` because the previous file was mojibake and could not
  be used as a reliable documentation entry.
- Rewrote `docs/developer-standard-flow.md` to remove mojibake and document the
  current developer workflow in plain Chinese.
- Updated non-historical docs that still referenced old config/skills GitLab
  paths:
  - `docs/architecture/retention-and-dr.md`;
  - `docs/release-evidence-chain.md`;
  - `docs/gitlab-repository-governance.md`.
- Added release `gap_details` to the core release status payload so AI and
  human users can understand missing evidence without guessing.
- Added human CLI rendering for `gap_details`.

## Verified State Before Change

- `opspilot release status --service opspilot-core` reported:
  - `status=healthy`;
  - image `192.168.48.206:5050/platform/opspilot/opspilot-core:227b6a74`;
  - GitLab pipeline `success`;
  - GitOps `matches_cluster`;
  - Argo CD `Synced / Healthy`;
  - Kubernetes `ready=1 desired=1 updated=1 available=1`.
- All Argo CD Applications pointed to
  `http://192.168.48.206:8929/tpo/deploy/gitops-manifests.git`.
- All restricted AppProjects allowed
  `http://192.168.48.206:8929/tpo/deploy/gitops-manifests.git`.

## Evidence Gaps Policy

The following gaps are not release blockers:

- `pod_metrics_missing`
  - release health can still be decided from Kubernetes, GitLab, Registry,
    GitOps, and Argo CD;
  - resource trend RCA is weaker until Prometheus label/scrape/source mapping
    is verified.
- `elk_logs_missing`
  - release health can still use Kubernetes pod logs as fallback;
  - cross-service RCA is weaker until ELK/OpenSearch/OpenObserve datasource
    mapping is configured.
- `quality_job_not_found`
  - optional quality evidence is not available;
  - it does not block the standard release path.

## Deferred

- `platform/opspilot` was moved in the follow-up core migration stage by
  creating `tpo/platform/opspilot/opspilot-core`. The old project remains only
  for registry history and compatibility.
- Do not force-connect Prometheus/ELK for all services. Missing optional
  evidence must remain explicit and non-blocking.
- Do not archive old GitLab redirect paths until the platform core migration
  plan is reviewed separately.

## Minimum Validation

- `go test ./internal/release ./cli`
- `go vet ./...`
- `go run ./cli --output human config validate --dir ./config/opspilot-config`
- `git diff --check`
- `opspilot release status --service opspilot-core --output human`

## Rollback

- Revert:
  - `internal/release/release.go`;
  - `cli/release_cli.go`;
  - `cli/release_cli_test.go`;
  - `internal/release/release_test.go`;
  - documentation files changed in this record.
- No external GitLab, GitOps, Argo CD, or Kubernetes state is changed by this
  stage.
