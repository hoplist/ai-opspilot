# 2026-05-21 Release Argo CD Evidence

## Change

Added Kubernetes-side Argo CD Application evidence to the OpsPilot release
status path.

`opspilot release status --service opspilot-core` can now include:

- Deployment container image from Kubernetes desired state.
- Argo CD Application sync status.
- Argo CD Application health status.
- Argo CD revision and operation phase when present.

## Reason

The first release evidence milestone should tell operators whether a release
has reached GitOps reconciliation before they inspect Pod-level failures.
Reading the Argo CD Application CR keeps the integration read-only and avoids
requiring the `argocd` CLI.

## Impact

- `OPSPILOT_RELEASE_SERVICES` now supports `argocd:<app-name>`.
- Missing Argo CD access is reported as an explicit evidence gap.
- GitLab, registry, and GitOps repository checks remain explicit future gaps
  until read-only credentials are configured.
