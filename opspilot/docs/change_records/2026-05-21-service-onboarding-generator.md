# 2026-05-21 Service Onboarding Generator

## Change

Added a first service onboarding generator to OpsPilot CLI:

```powershell
opspilot onboard service --config opspilot.service.yaml --write
```

The generator creates missing release files:

- `.gitlab-ci.yml`
- `deploy/k8s/deployment.yaml`
- `deploy/k8s/service.yaml`
- `deploy/k8s/kustomization.yaml`
- optional `Dockerfile`
- `opspilot.release-service.txt`

## Design

The generator does not force a single Dockerfile style. Existing Dockerfiles
are preserved by default. CI is generated as a thin GitLab include that points
to the platform template in `ci/templates/buildkit-gitops.go.yml`.

## Impact

New services can be onboarded with fewer hand-written files while keeping room
for project-specific Dockerfiles, proxy settings, and future custom templates.
