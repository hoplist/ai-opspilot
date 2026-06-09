# 2026-06-09 runtime skills validation and examples

## Goal

Start implementing the runtime skills source-adaptation design by adding a
local validation command and first-batch examples for high-value OpsPilot
skills.

## Changed

- Added `skills validate --dir <path>` to the OpsPilot CLI.
- Added `skillregistry.ValidateDirectory` for static validation of
  GitLab-backed runtime skills.
- Validation checks:
  - skills directory exists
  - `skill.yaml` files exist and parse
  - required routing fields are present
  - duplicate skill names are rejected
  - commands must map to OpsPilot capabilities, not arbitrary shell
  - high-priority skills should include `examples/`
  - `SKILL.md` should be present
- Added tests for a ready skill repository and a blocked arbitrary shell
  command.
- Added first-batch GitLab skills examples in `platform/opspilot-skills` for
  release success, GitOps drift, CrashLoop sidecar diagnosis, partial evidence
  RCA, beginner cluster checks, precheck blockers, secret redaction, deferred
  internal auth, and missing-evidence wording.

## Decision

The validator is intentionally lightweight. It does not execute skills and does
not need internet access. Its job is to prevent unsafe or incomplete runtime
skill packages from entering the server-side registry unnoticed.

## Next Work

- Publish the updated GitLab skills repository.
- Add validation to the standard gstack-inspired release gate before OpsPilot
  release.
- Add more examples for database, Kubernetes, and monitoring skills.
