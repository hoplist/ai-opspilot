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
