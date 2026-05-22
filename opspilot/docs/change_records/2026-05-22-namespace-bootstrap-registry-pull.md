# 2026-05-22 Namespace Bootstrap Registry Pull Secret

## Superseded

This experiment added a platform CronJob to copy
`opspilot/gitlab-registry-pull` into OpsPilot-managed namespaces.

It was removed after confirming that the test environment uses an internal
private registry with image pull authentication configured at the `containerd`
layer. Per-namespace registry Secret bootstrap is not needed for the default
service onboarding flow.

See:

```text
2026-05-22-remove-namespace-bootstrap.md
```
