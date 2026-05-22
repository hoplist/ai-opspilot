# 2026-05-22 DevEx Namespace Automation

## Change

Moved the service onboarding convention to the `tpo` GitLab root with current
services grouped under `devex`:

```text
tpo/devex/<project>/<service>
```

OpsPilot now derives ownership and namespace from that path. The namespace is
project-level by default:

```text
tpo/devex/demo/demo-api     -> cicd-devex-demo
tpo/devex/demo/demo-web     -> cicd-devex-demo
tpo/devex/skillshub/api     -> cicd-devex-skillshub
```

The platform can still override a namespace with `opspilot.namespaces.yaml`:

```yaml
namespaceMappings:
  tpo/devex/opspilot/*: opspilot
```

## Generated Files

`opspilot onboard generate --write` now writes:

- `opspilot.service.yaml` with `ownership` and `namespaceSource`.
- `.gitlab-ci.yml` including the shared BuildKit/GitOps template.
- `deploy/k8s/namespace.yaml`.
- `deploy/k8s/deployment.yaml`.
- `deploy/k8s/service.yaml`.
- `deploy/k8s/kustomization.yaml` including `namespace.yaml`.
- `opspilot.release-service.txt`.

## CI And GitOps

The shared Go BuildKit template now checks `deploy/k8s/namespace.yaml` during
preflight. During `update:gitops`, it copies all `deploy/k8s` files and adds
the generated Argo Application file to `apps/kustomization.yaml`, so a new
service can be discovered by the existing app-of-apps flow.

## Validation

Local validation target:

```powershell
go test ./opspilot/...
.\opspilot\scripts\build-cli.ps1
```

Real validation target:

```powershell
opspilot onboard generate --project tpo/devex/demo/demo-api --write
git push node206 main
opspilot release status --service opspilot-core
```

## Real Demo Result

Created and pushed a real demo service:

```text
GitLab project: tpo/devex/demo/demo-api
Local repo: D:\code\auto_inspection\real-demo-api-devex
Namespace: cicd-devex-demo
Argo Application: devex-demo-demo-api
Image: 192.168.48.206:5050/tpo/devex/demo/demo-api/demo-api:8d5df845
```

End-to-end result:

- `opspilot onboard detect` inferred `devex/demo/demo-api`.
- `opspilot onboard generate --write` generated Dockerfile, GitLab CI,
  namespace, Deployment, Service, Kustomize, and release mapping.
- GitLab pipeline completed successfully:
  preflight, Go test, binary build, BuildKit image build, and GitOps update.
- GitOps registered `apps/devex-demo-demo-api-application.yaml` through
  `apps/kustomization.yaml`.
- Argo CD reported `Synced` and `Healthy`.
- Kubernetes reported `deployment/demo-api` as `1/1` available.
- OpsPilot `k8s pods --namespace cicd-devex-demo` saw the demo Pod as Ready.

## Follow-up Gap

The first demo rollout exposed one environment bootstrap gap: a newly generated
namespace does not automatically receive `gitlab-registry-pull`. The Pod
entered `ImagePullBackOff` until a project-scoped `read_registry` deploy token
was created and stored as a Kubernetes pull secret in `cicd-devex-demo`.

This should become a platform-owned namespace bootstrap step, not a developer
task. The service generator should continue to avoid committing registry
credentials into GitOps.
