# 2026-05-22 Service Onboarding Preflight

## Change

Added a repository preflight check for OpsPilot service onboarding:

```powershell
opspilot onboard check --config opspilot.service.yaml
```

Also added CI preflight checks to:

- Root `.gitlab-ci.yml` for OpsPilot itself.
- Shared `ci/templates/buildkit-gitops.go.yml` for onboarded Go services.

## Checks

The command verifies:

- Dockerfile exists at `dockerfile.path`.
- `.gitlab-ci.yml` includes the BuildKit platform template or direct BuildKit
  usage.
- `deploy/k8s/deployment.yaml` exists.
- `deploy/k8s/service.yaml` exists.
- `deploy/k8s/kustomization.yaml` exists.
- optional `opspilot.release-service.txt` exists.

## Usage

Use `onboard check` before release or as a GitLab CI preflight job. Use the
existing generator to fill missing files:

```powershell
opspilot onboard service --config opspilot.service.yaml --write
```

The generator already creates deploy YAML files and skips existing files unless
`--force` is passed.
