# 2026-06-09 skills review follow-up

## Goal

Apply the first four findings from the skill-assisted OpsPilot code review.
The fifth finding, missing ELK/APISIX evidence, is intentionally unchanged
because service-only troubleshooting with explicit missing-evidence reporting is
the current design.

## Changed

- Reduced `repo precheck` false positives by excluding HTTP query/helper loops
  from the possible N+1 database-access heuristic.
- Added backend endpoint `/api/skills/validate` so clients can ask OpsPilot to
  validate the server-side runtime skills without knowing the Pod filesystem
  path.
- Changed `opspilot skills validate` to use the backend endpoint by default.
  `--dir` remains available for local/offline validation.
- Split skills-related CLI code from `cli/main.go` into `cli/skills.go`.
- Treated direct BuildKit CI as valid for the `platform/opspilot` repository
  because this repository owns the platform CI flow.
- Added `.opspilot/quality.yaml` with an optional `/api/live` smoke check.

## Not Changed

- ELK/OpenSearch and APISIX missing evidence remains a known capability gap.
  OpsPilot should continue to report it explicitly and continue with
  Kubernetes, Prometheus, release, Docker agent, and skills evidence.

## Validation Plan

- `go test ./opspilot/...`
- `go vet ./opspilot/...`
- `opspilot repo precheck --repo opspilot --project platform/opspilot`
- `opspilot repo preflight` for `platform/opspilot`
- `opspilot skills validate` against the backend runtime registry
