# 2026-05-22 Remove Namespace Bootstrap

## Change

Removed the `opspilot-namespace-bootstrap` CronJob and the in-cluster
`opspilot bootstrap namespace-secrets` command.

The test environment will use an internal private registry with image pull
authentication configured in `containerd`, so service namespaces do not need a
copied `gitlab-registry-pull` Secret.

## Generator Behavior

Generated service Deployments no longer include:

```yaml
imagePullSecrets:
  - name: gitlab-registry-pull
```

This avoids noisy `FailedToRetrieveImagePullSecret` warnings in namespaces that
use node-level registry authentication.

## GitOps Cleanup

The OpsPilot release pipeline now removes the previously generated GitOps path:

```text
clusters/test/apps/opspilot-bootstrap
```

Argo CD will prune the bootstrap CronJob, ServiceAccount, ClusterRole, and
ClusterRoleBinding after the next OpsPilot release.
