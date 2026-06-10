# 2026-06-10 CLI second-stage split

## Goal

Continue the maintainability work after the first CLI architecture split.

This phase keeps behavior stable and focuses on the next three files that would
slow future iteration:

1. Split `cli/main_test.go` by feature area.
2. Split `cli/inspect.go` into pod, service, cluster, and evidence-pack files.
3. Split `cli/onboard_templates.go` by generated artifact family.

## Scope

- Same Go package, same public commands, same JSON output fields.
- No Kubernetes, GitLab, GitOps, credential, or runtime configuration changes.
- Avoid unrelated local files and demo directories.

## Planned File Boundaries

### Tests

- `cli/main_test.go`: shared test helpers and top-level command smoke tests.
- `cli/inspect_test.go`: inspect, metrics, evidence, and output behavior.
- `cli/onboard_test.go`: onboarding detection, config, generated files, and
  middleware/storage planning.
- `cli/repo_test.go`: repository preflight, autofix, and code-precheck rules.
- `cli/skills_test.go`: skills registry, skills candidates, and credentials
  catalog CLI behavior.

### Inspect

- `cli/inspect.go`: inspect command dispatch and shared inspect result models.
- `cli/inspect_pod.go`: pod inspect and pod fix evidence.
- `cli/inspect_service.go`: service inspect, release context, and service fix.
- `cli/inspect_cluster.go`: cluster inspect and filesystem/restart aggregation.
- `cli/evidence_pack.go`: AI-readable evidence pack rendering.

### Templates

- `cli/onboard_templates.go`: shared template entry points.
- `cli/onboard_template_config.go`: `opspilot.service.yaml`, middleware config,
  and storage config templates.
- `cli/onboard_template_ci.go`: Dockerfile and GitLab CI include templates.
- `cli/onboard_template_k8s.go`: namespace, limits, quota, service account,
  deployment, service, kustomization, and quality templates.
- `cli/onboard_template_middleware.go`: MySQL, PostgreSQL, Redis, and MinIO
  generated middleware manifests.

## Validation

- `go test ./...`
- `go vet ./...`
- Rebuild CLI binary.
- Smoke test core commands against the deployed OpsPilot backend.
- Release through the standard GitLab Runner -> BuildKit -> Registry -> GitOps
  -> Argo CD flow.

## Implementation Result

### Tests

- `cli/main_test.go` now only keeps top-level schema/version/global flag tests.
- Feature tests were split into:
  - `cli/inspect_test.go`
  - `cli/skills_test.go`
  - `cli/release_cli_test.go`
  - `cli/onboard_test.go`
  - `cli/repo_test.go`
  - `cli/lifecycle_test.go`
  - `cli/quality_cli_test.go`
  - `cli/test_helpers_test.go`

### Inspect

- `cli/inspect.go` now keeps only inspect command dispatch.
- Shared data models moved to `cli/inspect_models.go`.
- Fix planning moved to `cli/inspect_fix.go`.
- Pod, service, and cluster inspection moved to:
  - `cli/inspect_pod.go`
  - `cli/inspect_service.go`
  - `cli/inspect_cluster.go`
- AI-readable evidence pack rendering moved to `cli/evidence_pack.go`.

### Templates

- `cli/onboard_templates.go` now only documents the split.
- Template implementations moved to:
  - `cli/onboard_template_config.go`
  - `cli/onboard_template_ci.go`
  - `cli/onboard_template_k8s.go`
  - `cli/onboard_template_middleware.go`

### Local Validation

- `go test ./...` passed.
- `go vet ./...` passed.
- `scripts/build-cli.ps1` rebuilt `D:\code\auto_inspection\build\opspilot.exe`.
- CLI smoke checks passed for:
  - `skills validate`
  - `capabilities`
  - `release status --service opspilot-core`
  - `repo precheck --repo opspilot --project tpo/devex/opspilot/opspilot-core --warn-only`
