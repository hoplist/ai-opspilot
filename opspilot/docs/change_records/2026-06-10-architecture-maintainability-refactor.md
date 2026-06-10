# 2026-06-10 architecture maintainability refactor

## Goal

Address the architecture review findings in priority order and keep OpsPilot
maintainable as it grows.

This phase focuses on code organization, not behavior changes:

1. Split the remaining large CLI entrypoint by domain.
2. Split onboarding and repository governance code by responsibility.
3. Keep the backend modular single-service architecture instead of splitting
   into microservices prematurely.
4. Preserve the standard release flow and re-run architecture/API/code review
   after rollout.

## Findings Being Addressed

- `cli/main.go` is still too large for a thin client and mixes command
  routing, HTTP calls, output helpers, natural language handling, inspect,
  release, quality, and evidence-pack rendering.
- `cli/onboard.go` mixes repository detection, config parsing, template
  rendering, middleware planning, and storage planning.
- `cli/repo.go` mixes repository preflight, code precheck, source scanning,
  and evidence output.
- Core backend route files are reasonably split and should remain a modular
  single service for now.

## Non-Goals

- Do not split OpsPilot into microservices in this phase.
- Do not change public CLI behavior.
- Do not change cluster, GitLab, GitOps, or credential runtime configuration.
- Do not touch unrelated demo directories or pre-existing docs index edits.

## Refactor Plan

### CLI

- Keep `cli/main.go` as command routing and top-level compatibility only.
- Move natural language command handling to `cli/ask.go`.
- Move command-to-endpoint parsing to `cli/command_parse.go`.
- Move inspect and evidence-pack rendering to `cli/inspect.go`.
- Move filesystem metric handling to `cli/metrics.go`.
- Move release commands to `cli/release_cli.go`.
- Move quality commands to `cli/quality_cli.go`.
- Move capability/doctor helpers to `cli/system.go`.
- Move low-level HTTP and output helpers to `cli/httpclient.go` and
  `cli/output.go`.

### Onboarding

- Keep the public command entrypoints stable.
- Split detection/config logic, templates, middleware, and storage helpers into
  separate files under the CLI package before a later package-level extraction.
- Implemented same-package split:
  - `cli/onboard_models.go`: service, resource, middleware, storage, namespace,
    and GitOps planning data structures.
  - `cli/onboard_check.go`: detect/generate/service/check command flows and
    guardrail checks.
  - `cli/onboard_detect.go`: repository/language/Dockerfile/namespace detection.
  - `cli/onboard_config.go`: `opspilot.service.yaml` parsing and defaults.
  - `cli/onboard_discovery.go`: repository signal scan and middleware/storage
    evidence detection.
  - `cli/onboard_storage.go`: storage normalization and hostPath defaults.
  - `cli/onboard_middleware.go`: middleware normalization, catalog lookup, and
    credential plan hints.
  - `cli/onboard_templates.go`: generated Dockerfile, CI, Kubernetes, middleware,
    quality, kustomization, and release mapping templates.
  - `cli/onboard_files.go`: generated file planning and write helpers.

### Repository Governance

- Split preflight and code-precheck logic into separate files under the CLI
  package before a later package-level extraction.
- Implemented same-package split:
  - `cli/repo_models.go`: preflight, autofix, and code-precheck result models.
  - `cli/repo.go`: public repo command entrypoints.
  - `cli/repo_preflight.go`: repository policy checks, deployment guardrails,
    storage/middleware checks, and Dockerfile generation decision.
  - `cli/repo_codeprecheck.go`: source scanning and risky code pattern detection.
  - `cli/repo_output.go`: human/evidence output writers.
  - `cli/repo_utils.go`: shared string helpers.

### API Compatibility

- Added `handleAPI` in `core/http.go`.
- Every current `/api/...` route now registers a `/api/v1/...` alias while
  preserving the old path.
- Added regression coverage for `/api` and `/api/v1` alias behavior.
- This specifically covers credentials, clusters, datasources, skills, release,
  quality, Kubernetes, metrics, logs, evidence, and system capability endpoints.

## Validation Plan

- `go test ./...`
- `go vet ./...`
- CLI smoke tests:
  - `opspilot skills validate --output human`
  - `opspilot credentials catalog --output human`
  - `opspilot release status --service opspilot-core --output human`
  - `opspilot capabilities --output human`
- Standard release:
  node206 GitLab Runner -> BuildKit -> Registry -> GitOps -> node200 Argo CD.
- Post-release architecture/API/code review with relevant skills.
