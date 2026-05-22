# 2026-05-22 Service Onboarding Detect Generate

## Change

Added repository detection and namespace-catalog driven generation so
developers do not need to hand-write `opspilot.service.yaml`.

New commands:

```powershell
opspilot onboard detect --project platform/skillshub-api
opspilot onboard generate --project platform/skillshub-api --write
```

## Namespace Management

Namespaces are resolved from a platform-owned catalog:

```yaml
namespaceMappings:
  platform/skillshub-*: skillshub
  platform/deer-flow*: ai-dev
  platform/opspilot: opspilot
```

If no mapping matches, generation fails with `namespace_mapping_missing`.
OpsPilot does not guess a namespace for new repositories.

## Detection

Detection infers:

- service name from GitLab project path or repository directory.
- language from common project files.
- Dockerfile path from common locations.
- port from Dockerfile `EXPOSE`, defaulting to `8080`.
- Go build entry from `cmd/<service>/main.go` or root `main.go`.

## Generation

`onboard generate --write` writes:

- `opspilot.service.yaml`
- `.gitlab-ci.yml`
- `deploy/k8s/deployment.yaml`
- `deploy/k8s/service.yaml`
- `deploy/k8s/kustomization.yaml`
- `opspilot.release-service.txt`
- `Dockerfile` only when one was not detected

Existing files are skipped unless `--force` is passed.
