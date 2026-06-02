# 2026-06-02 Code precheck GitLab skills

## Goal

Add a lightweight code precheck gate before BuildKit packaging. The goal is to
catch obviously dangerous code before it reaches image build or Kubernetes,
while keeping the normal release flow automatic and friendly for non-technical
users.

## Flow

```text
developer push
-> GitLab preflight:onboarding
-> GitLab code-precheck
-> language tests
-> BuildKit image build
-> GitOps update
-> Argo CD rollout
-> OpsPilot inspect/release evidence
-> AI skill explanation and fix suggestions
```

## Decisions

- The first version is deterministic and local to the repository.
- OpsPilot CLI owns the richer `repo precheck` command.
- GitLab CI templates also run a lightweight inline scanner and publish
  `.opspilot/evidence/code-precheck.json`.
- Normal warnings do not block the release.
- Only high-confidence blockers stop the pipeline.
- Skills do not execute arbitrary code. They explain the evidence and route the
  next safe action.

## Initial Blockers

- Hardcoded access tokens, private keys, or password-like assignments.
- Destructive SQL such as `DROP`, `TRUNCATE`, or unguarded `DELETE/UPDATE`.
- Query-style handlers/routes that also perform write operations.
- Obvious full-table reads without pagination or filtering.
- Dangerous shell execution such as `rm -rf /` or piping remote downloads into
  `sh`/`bash`.
- Unbounded file writes to logs/uploads/runtime paths.

## Warning-Only Findings

- Possible N+1 query patterns.
- `SELECT *` with weak context.
- Missing timeout hints on outbound clients.
- Suspicious raw SQL string construction that needs human review.

## Skill Routing

- `code-reviewer`: general code review and dangerous code smells.
- `security-reviewer`: secrets, injection, unsafe execution, dependency risks.
- `secure-code-guardian`: remediation patterns for auth, validation, SQL
  parameterization, and secure input handling.
- `database-optimizer` / `sql-pro`: full-table scan, N+1, pagination, and
  index/query risk explanation.
- `devops-engineer`: GitLab CI, BuildKit, GitOps, and release workflow.
- `debugging-wizard`: turns precheck evidence into a bounded fix plan.

## Evidence Schema

```json
{
  "service": "demo-api",
  "project": "tpo/devex/demo/demo-api",
  "status": "blocker",
  "summary": {
    "blockers": 1,
    "warnings": 2
  },
  "items": [
    {
      "id": "db_destructive_sql",
      "severity": "blocker",
      "category": "database",
      "path": "src/user.go",
      "line": 42,
      "message": "destructive SQL detected",
      "skill": "database-optimizer",
      "recommendation": "Add a guarded WHERE condition or move the operation behind an explicit migration/admin workflow."
    }
  ]
}
```

## Validation

- Add `repo precheck` tests for warning-only, blocker, and `--write` evidence.
- Ensure generated CI includes the `code-precheck` stage.
- Run `go test ./opspilot/...`.
- Publish through the standard node206 GitLab Runner -> BuildKit -> Registry
  -> GitOps -> Argo CD flow.

## Implemented

- Added `opspilot repo precheck`.
- Added `--write` evidence output:

```text
.opspilot/evidence/code-precheck.json
```

- Added code-precheck tests for:
  - warning-only findings;
  - blocker findings;
  - evidence file writing;
  - CI template coverage.
- Added `code-precheck` stage to all platform GitLab templates:
  - `ci/templates/buildkit-gitops.go.yml`
  - `ci/templates/buildkit-gitops.node.yml`
  - `ci/templates/buildkit-gitops.python.yml`
  - `ci/templates/buildkit-gitops.frontend.yml`
  - `ci/templates/buildkit-gitops.java.yml`
- Added `code-precheck` to the OpsPilot platform `.gitlab-ci.yml` itself so
  `opspilot-core` releases also exercise the gate.
- Added integrated skill registry entries for:
  - `code-reviewer`
  - `security-reviewer`
  - `secure-code-guardian`
  - `database-optimizer`

## Verified

2026-06-02:

```powershell
go test ./opspilot/...
git diff --check
go run ./opspilot/cli --output human repo precheck --repo . --project platform/opspilot
```

The current OpsPilot repository returns warning-only precheck evidence and does
not block its own release.

## Standard Flow Retest

2026-06-02:

- Retest the full standard release flow with a real documentation commit:
  node206 GitLab -> node206 GitLab Runner -> BuildKit rootless -> GitLab
  Registry -> GitOps -> node200 Argo CD.
- Expected result:
  - `preflight:onboarding` succeeds.
  - `code-precheck` succeeds and writes precheck evidence.
  - `test:go` succeeds.
  - `build:binaries` succeeds.
  - `build:image` succeeds and pushes a commit-tagged image.
  - `update:gitops` succeeds.
  - node200 Argo CD syncs the new image and the `opspilot-core` Deployment
    becomes Healthy.
