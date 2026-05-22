# Service Onboarding

OpsPilot service onboarding detects repository shape, maps the service to a
platform-managed namespace, and generates repeatable release files without
forcing every project to use the same Dockerfile style.

Developers should not have to hand-write `opspilot.service.yaml`. Treat it as
a generated intermediate file that operators can review when needed.

## GitLab And Namespace Model

Use `tpo` as the GitLab root group. Current services are grouped under
`devex` first:

```text
tpo/devex/<project>/<service>
```

OpsPilot treats the GitLab path as ownership metadata:

```text
tpo/devex/skillshub/skillshub-api
        |      |        |
        group  project  service
```

The default namespace is project-level, not service-level:

```text
tpo/devex/skillshub/skillshub-api    -> cicd-devex-skillshub
tpo/devex/skillshub/skillshub-web    -> cicd-devex-skillshub
tpo/devex/deer-flow/deer-flow-api    -> cicd-devex-deer-flow
```

Developers normally do not choose the namespace. OpsPilot derives it from
`group + project` and writes `deploy/k8s/namespace.yaml`.

Namespaces are still owned by the platform. Use an optional catalog file such
as `opspilot.namespaces.yaml` only when a project needs an explicit override:

```yaml
namespaceMappings:
  tpo/devex/skillshub/*: cicd-devex-skillshub
  tpo/devex/deer-flow/*: cicd-devex-deer-flow
  tpo/devex/opspilot/*: opspilot
```

Patterns support exact match and trailing `*` prefix match. If a repository
does not match the catalog, OpsPilot uses the project namespace convention:
`cicd-<group>-<project>`.

## Auto Detect

Run detection from a service repository:

```powershell
opspilot onboard detect --project tpo/devex/skillshub/skillshub-api
```

Detection infers:

- service name from GitLab project path or repository directory.
- language from `go.mod`, `package.json`, `pyproject.toml`, or
  `requirements.txt`.
- Dockerfile path from common locations.
- runtime port from Dockerfile `EXPOSE`, defaulting to `8080`.
- Go build entry from `cmd/<service>/main.go` or root `main.go`.
- ownership from the GitLab path.
- namespace from the platform catalog or the project namespace convention.

## Auto Generate

Generate onboarding files from repository detection:

```powershell
opspilot onboard generate --project tpo/devex/skillshub/skillshub-api --write
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
opspilot onboard check --repo . --config opspilot.service.yaml
```

The check verifies:

- Dockerfile path from `dockerfile.path`.
- `.gitlab-ci.yml` contains the BuildKit platform template or direct BuildKit
  usage.
- `deploy/k8s/namespace.yaml`.
- `deploy/k8s/deployment.yaml`.
- `deploy/k8s/service.yaml`.
- `deploy/k8s/kustomization.yaml`.
- optional `opspilot.release-service.txt`.

This command is intended for local onboarding and as a future GitLab CI
preflight job before BuildKit runs.

## Config

```yaml
name: skillshub-api
gitlabProject: tpo/devex/skillshub/skillshub-api
ownership:
  organization: tpo
  group: devex
  project: skillshub

language: go

build:
  entry: ./cmd/skillshub-api
  output: build/skillshub-api

runtime:
  port: 8080
  healthPath: /health

deploy:
  namespace: cicd-devex-skillshub
  namespaceSource: auto_project
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
deploy/k8s/namespace.yaml
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
shell preflight job that checks `DOCKERFILE_PATH`, `deploy/k8s/namespace.yaml`,
the workload manifests, and whether `.gitlab-ci.yml` references BuildKit. The
same template writes the service manifests into GitOps and registers the Argo
Application in the app-of-apps `apps/kustomization.yaml`.

If the repository is missing release files, generate them:

```powershell
opspilot onboard service --config opspilot.service.yaml --write
```

## Registry Pull Secret

Generated Deployments reference `gitlab-registry-pull` because normal service
images are stored in the GitLab Registry. A freshly generated namespace must
receive that Secret from platform bootstrap before Pods can pull private
images.

Do not commit registry credentials into a service repository or GitOps app
directory. Use a platform-owned initializer, external secret flow, or an
operator-managed namespace bootstrap step.
