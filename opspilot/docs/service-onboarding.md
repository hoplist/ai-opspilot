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
- middleware intent from dependency and config signals such as MySQL, Redis,
  RabbitMQ, MinIO/S3, OpenSearch/Elasticsearch, and Kafka clients.

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

## Automated Governance

For fully automated repository normalization, use the higher-level `repo`
commands. They do not require a platform-owner approval step; they either pass,
auto-generate controlled files, or fail with a policy reason that can be fixed
by the next automated run.

```powershell
opspilot repo preflight --repo . --project tpo/devex/skillshub/skillshub-api
opspilot repo autofix --repo . --project tpo/devex/skillshub/skillshub-api --write
opspilot repo autofix --repo . --project tpo/devex/skillshub/skillshub-api --write --force
```

`repo preflight` checks:

- Dockerfile presence and obvious local-only or unsafe patterns.
- `.gitlab-ci.yml` usage of the platform BuildKit/GitOps template.
- `deploy/k8s/namespace.yaml`, Deployment, Service, and Kustomize entrypoint.
- inferred namespace ownership from the GitLab project path.
- Deployment namespace, probes, and disallowed fields such as `hostPath`,
  `hostNetwork`, and `privileged`.
- health path defaults.
- middleware intent, using shared test-environment instances by default.

`repo autofix --write` writes missing platform-managed files. Add `--force`
when the repository already contains a local Dockerfile, local CI, or manifests
that must be replaced by the platform standard.

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

middleware:
  mysql:
    kind: mysql
    display: MySQL database
    mode: shared-database
    allocation: database-user
    resource: devex_skillshub_skillshub_api_mysql
    secret: skillshub-api-mysql-conn
    env: DATABASE_URL
    reason: detected MySQL database dependency; use shared-database and allocate database-user

release:
  prometheusSource: node200-k8s
```

## Middleware

OpsPilot does not default to one middleware Pod per service in the test
environment. It detects dependencies and records a platform-owned allocation
intent:

```text
MySQL/PostgreSQL  -> shared instance, database + user
Redis             -> shared instance, key prefix or DB index
RabbitMQ          -> shared instance, vhost + user
MinIO/S3          -> shared instance, bucket + access key
OpenSearch        -> shared instance, index prefix
Kafka             -> shared instance, topic prefix + ACL user
```

`repo preflight` reports detected middleware evidence, and `repo autofix` or
`onboard generate` persists the intent in `opspilot.service.yaml`. This first
step does not inject Secret references into the Deployment, because runtime
provisioning of databases/users/vhosts/buckets must happen before the
application can safely consume those Secrets.

When provisioning is added, failures should be written as structured error
events so OpsPilot and AI assistants can read them directly:

```json
{
  "source": "middleware",
  "stage": "provision",
  "service": "skillshub-api",
  "namespace": "cicd-devex-skillshub",
  "resource": "mysql/devex_skillshub_skillshub_api_mysql",
  "message": "failed to create database user"
}
```

The runtime location is controlled by `OPSPILOT_ERROR_EVENT_DIR`, defaulting to
`/var/lib/opspilot/error-events`, and can be queried with:

```powershell
opspilot errors recent --source middleware --service skillshub-api
```

Dedicated middleware instances should be explicit exceptions for load testing,
version compatibility testing, strong isolation, or middleware-specific
configuration differences.

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

Initial platform templates are available for:

```text
ci/templates/buildkit-gitops.go.yml
ci/templates/buildkit-gitops.node.yml
ci/templates/buildkit-gitops.python.yml
```

If the repository is missing release files, generate them:

```powershell
opspilot repo autofix --project tpo/devex/skillshub/skillshub-api --write
```

## Registry Auth

Generated Deployments do not create or reference a per-namespace
`imagePullSecret`.

In the test environment, image pull authentication is owned by the node
runtime. Configure the internal private registry and credentials in
`containerd`, then service manifests stay clean and new namespaces do not need
registry Secret bootstrap.

Do not commit registry credentials into a service repository or GitOps app
directory.
