# 2026-05-21 Release Pipeline Standard

## Change

Documented the required release path for future OpsPilot image builds and
deployments:

```text
node206 GitLab
-> node206 GitLab Runner
-> BuildKit rootless image build
-> Push image registry
-> Update GitOps repository
-> node200 Argo CD automatic deployment
```

## Reason

The project should avoid one-off local image builds and direct cluster mutation
for normal releases. CI builds and GitOps reconciliation provide a clearer
audit trail and safer rollback path.

## Impact

- Local builds remain acceptable for CLI and unit-test validation.
- Release images should be built by node206 GitLab Runner with rootless
  BuildKit.
- Cluster deployment should happen through GitOps updates reconciled by Argo CD
  on node200.

