# 2026-05-22 Namespace Bootstrap Registry Pull Secret

## Change

Added a platform-owned namespace bootstrap package:

```text
deploy/opspilot/bootstrap
```

It deploys `opspilot-namespace-bootstrap`, a CronJob that runs the OpsPilot CLI
command `opspilot bootstrap namespace-secrets`. The command copies the
platform GitLab Registry pull secret from `opspilot/gitlab-registry-pull` into
every namespace labelled:

```text
opspilot.io/managed=true
```

Generated service namespaces already carry this label through
`deploy/k8s/namespace.yaml`, so new services can pull private GitLab Registry
images without developer action.

## Security Boundary

Registry credentials stay out of service repositories and GitOps application
directories. The bootstrap job reads the source Secret in-cluster and applies a
copy into managed namespaces.

The source Secret was updated to use a GitLab `read_registry` credential only,
stored as `opspilot/gitlab-registry-pull`. It has no GitLab API scope and no
registry write permission.

The bootstrap ServiceAccount is separate from `opspilot-core` and has only the
Kubernetes permissions needed to list namespaces and create/update the pull
Secret. The CronJob reuses the released OpsPilot image, avoiding a dependency
on an external kubectl utility image.

## GitOps

The OpsPilot release pipeline now copies:

```text
deploy/opspilot/bootstrap -> clusters/test/apps/opspilot-bootstrap
```

and includes it from the `opspilot-core` Argo Application kustomization.

## Validation Target

After release:

```powershell
kubectl -n opspilot get cronjob opspilot-namespace-bootstrap
kubectl -n cicd-devex-demo delete secret gitlab-registry-pull
kubectl -n opspilot create job --from=cronjob/opspilot-namespace-bootstrap opspilot-namespace-bootstrap-manual
kubectl -n cicd-devex-demo get secret gitlab-registry-pull
```

Expected result: the Secret is recreated automatically in the managed namespace
and `demo-api` remains `1/1` Ready.
