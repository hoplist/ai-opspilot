# Service Onboarding

OpsPilot service onboarding detects repository shape, maps the service to a
platform-managed namespace, and generates repeatable release files without
forcing every project to use the same Dockerfile style.

Developers should not have to hand-write `opspilot.service.yaml`. Treat it as
a generated intermediate file that operators can review when needed.

## Namespace Catalog

Namespaces are owned by the platform. Keep project-to-namespace rules in a
catalog file such as `opspilot.namespaces.yaml`:

```yaml
namespaceMappings:
  platform/skillshub-*: skillshub
  platform/skills-*: skillshub
  platform/deer-flow*: ai-dev
  platform/opspilot: opspilot
```

Patterns support exact match and trailing `*` prefix match. If a repository
does not match the catalog, `onboard generate` fails with
`namespace_mapping_missing` instead of guessing.

## Auto Detect

Run detection from a service repository:

```powershell
opspilot onboard detect --project platform/skillshub-api
```

Detection infers:

- service name from GitLab project path or repository directory.
- language from `go.mod`, `package.json`, `pyproject.toml`, or
  `requirements.txt`.
- Dockerfile path from common locations.
- runtime port from Dockerfile `EXPOSE`, defaulting to `8080`.
- Go build entry from `cmd/<service>/main.go` or root `main.go`.
- namespace from the platform namespace catalog.

## Auto Generate

Generate onboarding files from repository detection:

```powershell
opspilot onboard generate --project platform/skillshub-api --write
```

This writes `opspilot.service.yaml` plus the same release files as
`onboard service --write`. Existing files are skipped unless `--force` is
passed.

The first milestone is local and conservative:

```powershell
opspilot onboard service --config opspilot.service.yaml --write
```

This lower-level command still works when operators want to provide an explicit
config.

Before a repository is released, run the preflight check:

```powershell
opspilot onboard check --config opspilot.service.yaml
```

The check verifies:

- Dockerfile path from `dockerfile.path`.
- `.gitlab-ci.yml` contains the BuildKit platform template or direct BuildKit
  usage.
- `deploy/k8s/deployment.yaml`.
- `deploy/k8s/service.yaml`.
- `deploy/k8s/kustomization.yaml`.
- optional `opspilot.release-service.txt`.

This command is intended for local onboarding and as a future GitLab CI
preflight job before BuildKit runs.

## Config

```yaml
name: skillshub-api
gitlabProject: platform/skillshub-api
language: go

build:
  entry: ./cmd/skillshub-api
  output: build/skillshub-api

runtime:
  port: 8080
  healthPath: /health

deploy:
  namespace: skillshub
  replicas: 1
  container: skillshub-api

dockerfile:
  mode: existing
  path: Dockerfile

ci:
  mode: include

release:
  prometheusSource: node200-k8s
```

## Modes

- `dockerfile.mode: existing`: keep an existing Dockerfile and do not overwrite it.
- `dockerfile.mode: generate`: create a simple Dockerfile when one does not exist.
- `ci.mode: include`: create a thin `.gitlab-ci.yml` that includes the platform CI template.
- `ci.mode: generate`: create the same include-based CI file for now.

## Proxy

Proxy values should be GitLab variables, not committed files:

```text
HTTP_PROXY
HTTPS_PROXY
NO_PROXY
```

The platform CI template passes these as BuildKit build args. Project
Dockerfiles can choose whether to consume them:

```dockerfile
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY
```

## Generated Files

```text
Dockerfile                    # only when requested and missing
.gitlab-ci.yml
deploy/k8s/deployment.yaml
deploy/k8s/service.yaml
deploy/k8s/kustomization.yaml
opspilot.release-service.txt
```

`opspilot.release-service.txt` contains the OpsPilot release mapping that can
be added to `OPSPILOT_RELEASE_SERVICES` or a future structured service catalog.

## CI Preflight

A service repository can fail fast before building images:

```yaml
preflight:opspilot:
  stage: test
  script:
    - opspilot onboard check --config opspilot.service.yaml
```

The shared `ci/templates/buildkit-gitops.go.yml` also includes a lightweight
shell preflight job that checks `DOCKERFILE_PATH`, `deploy/k8s/*.yaml`, and
whether `.gitlab-ci.yml` references BuildKit.

If the repository is missing release files, generate them:

```powershell
opspilot onboard service --config opspilot.service.yaml --write
```
