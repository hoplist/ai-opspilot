# Skills Review Pipeline

## Goal

Complete the server-side skills import review workflow:

1. Automatically discover mirrored candidates.
2. Generate dry-run import plans.
3. Score whether a candidate fits OpsPilot backend runtime boundaries.
4. Require explicit confirmation and GitLab skills repository changes before a candidate becomes a runtime skill.

## Candidate Cleanup

The following OpsPilot-owned platform gaps are now tracked as skills mirror candidates, not runtime skills:

- `api-quality-check`: API response time and basic security quality checks.
- `middleware-provisioning`: MySQL, Redis, Elasticsearch, Kafka, S3, and RabbitMQ planning.
- `release-healer`: GitLab, BuildKit, Registry, GitOps, Argo CD, and rollout failure attribution.
- `storage-guardrail`: hostPath, PV, filesystem pressure, cleanup risk, and retention boundaries.
- `frontend-smoke`: frontend blank page, asset loading, static build output, and framework precheck evidence.

## OpsPilot Changes

- Added `/api/skills/discover` for all candidate scoring.
- Added `/api/skills/review?name=<candidate>` for one candidate scoring.
- Added `opspilot skills discover`.
- Added `opspilot skills review --name <candidate>`.
- Review output includes decision, score, grade, import-plan readiness, blockers, missing mappings, and next steps.
- Stabilized the OpsPilot `update:gitops` CI job by removing runtime `apk add git yq`; the job now uses the Go CI image and GitLab Commit API, so it no longer depends on git/yq being installed inside the runner image.

## Safety Boundary

- `discover`, `review`, `import-plan`, and `promote --dry-run` are read-only.
- Promotion is still a GitLab skills repository commit under `skills/`.
- Unsupported candidates remain blocked when they require client-local browser, mobile device, desktop, or external session runtime.
- Runtime validation remains `opspilot skills validate`.

## Intended Flow

```text
skills discover
-> skills review --name <candidate>
-> skills import-plan --name <candidate>
-> explicit confirmation
-> commit reviewed files under GitLab skills repo skills/<candidate>/
-> git-sync refreshes OpsPilot backend mount
-> skills validate
```

## Validation

- Unit tests cover scoring, unsupported blocking, API endpoints, CLI output, and CLI schema registration.
- Release pipeline validation covers the GitOps job without external Alpine package installation.
- Standard release flow remains required for OpsPilot backend changes:
  node206 GitLab Runner -> BuildKit -> Registry -> GitOps -> node200 Argo CD.
