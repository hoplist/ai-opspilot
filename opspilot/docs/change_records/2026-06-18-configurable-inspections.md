# 2026-06-18 Configurable Inspections

## Goal

Make OpsPilot inspections configurable from the GitLab-managed runtime config so cluster checks, business-flow checks, and datasource gaps can be maintained without changing code.

## Scope

- Add an `Inspection` config kind and bulk `inspections:` support.
- Add read-only inspection catalog, run, and generate entrypoints.
- Keep manual YAML maintenance as the source of truth, with generate returning a draft only.
- Report missing adapters and missing evidence explicitly instead of failing the whole workflow.
- Do not add a scheduler, controller, remote execution loop, or automatic repair in this phase.

## Design

Inspection policies describe what should be checked for a cluster or service scope:

- Kubernetes health such as abnormal Pods and restarts.
- Node and filesystem pressure.
- Optional business-flow health from configured `Flow` entries.
- Optional middleware checks such as Kafka lag when matching datasources exist.

First implementation is configuration-first:

- `inspections catalog` lists configured policies.
- `inspection run --name <name>` evaluates the configured policy shape and returns check status plus gaps.
- `inspection generate --cluster <cluster>` produces a reviewable YAML draft and does not write files.

## Risk Boundary

- All new commands are read-only.
- No Kubernetes, GitLab, GitOps, datasource, or middleware mutation is performed.
- Disabled checks are returned as `disabled`.
- Enabled checks without a runtime adapter are returned as `not_executed` with `inspection_check_adapter_not_configured:<check>`.

## Validation Plan

- `go test ./...`
- `go vet ./...`
- `go run ./cli --output human config validate --dir ./config/opspilot-config`
- CLI smoke after deploy:
  - `opspilot inspections catalog`
  - `opspilot inspection generate --cluster node200-test`
