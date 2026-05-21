# Service Onboarding

OpsPilot service onboarding generates the repeatable release files for a new
service without forcing every project to use the same Dockerfile style.

The first milestone is local and conservative:

```powershell
opspilot onboard service --config opspilot.service.yaml --write
```

It creates missing files and skips existing files unless `--force` is passed.

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
