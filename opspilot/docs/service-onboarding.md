# Service Onboarding

OpsPilot service onboarding detects repository shape, maps the service to a
platform-managed namespace, and generates repeatable release files without
forcing every project to use the same Dockerfile style.

Developers should not have to hand-write `opspilot.service.yaml`. Treat it as
a generated intermediate file that operators can review when needed.

## GitLab And Namespace Model

Repository purpose and target GitLab group layout are governed by
[gitlab-repository-governance.md](gitlab-repository-governance.md). Service
onboarding only applies to application source repositories, not GitOps,
backups, runtime skills, or sandbox repositories.

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
- storage intent from logs, runtime/upload, and cache/temp path signals.
- config source intent from Apollo-style flags or config keys such as
  `--cfg=`, `--env=`, `APOLLO_META`, `apollo.meta`, and `apolloconfig`.

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
- `deploy/k8s/serviceaccount.yaml`.
- `deploy/k8s/deployment.yaml`.
- `deploy/k8s/service.yaml`.
- `deploy/k8s/kustomization.yaml`.
- optional `.opspilot/quality.yaml`.
- optional `opspilot.release-service.txt`.
- `deploy/k8s/configmap.yaml` when `configSources` such as Apollo are
  configured.

This command is intended for local onboarding and as a future GitLab CI
preflight job before BuildKit runs.

## Automated Governance

For fully automated repository normalization, use the higher-level `repo`
commands. They do not require a platform-owner approval step; they either pass,
auto-generate controlled files, or fail with a policy reason that can be fixed
by the next automated run.

```powershell
opspilot repo preflight --repo . --project tpo/devex/skillshub/skillshub-api
opspilot repo precheck --repo . --project tpo/devex/skillshub/skillshub-api
opspilot repo upload-plan --repo . --name skillshub-api
opspilot repo autofix --repo . --project tpo/devex/skillshub/skillshub-api --write
opspilot repo autofix --repo . --project tpo/devex/skillshub/skillshub-api --write --force
```

`repo preflight` checks:

- Dockerfile presence and obvious local-only or unsafe patterns.
- `.gitlab-ci.yml` usage of the platform BuildKit/GitOps template.
- `deploy/k8s/namespace.yaml`, Deployment, Service, and Kustomize entrypoint.
- optional `.opspilot/quality.yaml` API smoke checks. Missing quality config is
  a warning only, not a release blocker.
- inferred namespace ownership from the GitLab project path.
- Deployment namespace, probes, and disallowed fields such as `hostPath`,
  `hostNetwork`, and `privileged`.
- Deployment storage policy: non-platform hostPath is blocked, platform-managed
  hostPath is allowed only under `/data/opspilot/hostpath/`, and `emptyDir`
  must include `sizeLimit`.
- health path defaults.
- middleware detection and generated lightweight middleware manifests where
  policy allows automatic provisioning.
- Apollo/config source detection and generated ConfigMap plus Deployment
  references when the application already supports that config source.

`repo precheck` scans repository source code for high-confidence dangerous
patterns before image packaging:

- hardcoded secret/token/password-like values.
- destructive SQL and unguarded `UPDATE`/`DELETE`.
- query-style handlers that write data.
- full-table reads, `SELECT *`, and possible N+1 query patterns.
- dangerous shell execution.
- unbounded writes to logs/uploads/runtime paths.

Warnings do not block the flow. Blockers fail `repo precheck` and the GitLab
`code-precheck` job. Use `--write` to persist the AI-readable evidence file:

```powershell
opspilot repo precheck --repo . --project tpo/devex/skillshub/skillshub-api --write
```

This writes:

```text
.opspilot/evidence/code-precheck.json
```

This is an automatic OpsPilot quality gate for vibecoding-style repositories,
not a manual operations approval step. Evidence includes a `policy` block with
`human_approval_required=false`. High-confidence `blocker` findings stop the
release and include concrete `fix_options`; uncertain findings remain
`warning` and should be explained by AI without blocking the release.

`repo upload-plan` is for the early test-only stage where the user identity is
not mapped yet. It returns a read-only target plan:

```text
GitLab project: tpo/sandbox/devex/<repo>
Namespace: sandbox
GitOps path: clusters/test/apps/sandbox/<repo>
Release scope: test-only
```

It does not create GitLab projects, push code, edit GitOps, or mutate the
cluster. Real GitLab creation remains an explicit platform action until the
identity and permission model is added.

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
    provision: auto
    resource: devex_skillshub_skillshub_api_mysql
    secret: skillshub-api-mysql-conn
    env: DATABASE_URL
    reason: detected MySQL database dependency; use shared-database and allocate database-user

storage:
  logs:
    purpose: logs
    mode: hostPath
    mountPath: /app/logs
    hostPath: /data/opspilot/hostpath/cicd-devex-skillshub/skillshub-api/logs
    sizeHint: 10Gi
    retentionDays: 7
  cache:
    purpose: cache
    mode: emptyDir
    mountPath: /tmp/cache
    sizeLimit: 1Gi

configSources:
  apollo:
    type: apollo
    required: true
    appId: skillshub-api
    env: prod
    cluster: default
    namespaces: application
    meta: http://apolloconfig-server-inner.tpo.xzoa.com
    tokenSecret: skillshub-api-apollo-token
    inject: env

release:
  prometheusSource: node200-k8s
```

## Middleware

OpsPilot detects dependencies and, for common lightweight development
dependencies, generates middleware manifests together with the application:

```text
MySQL/PostgreSQL  -> provision: auto, dedicated lightweight Deployment/Service/Secret
Redis             -> provision: auto, dedicated lightweight Deployment/Service/Secret
MinIO/S3          -> provision: auto, dedicated lightweight Deployment/Service/Secret
RabbitMQ          -> provision: external, platform/shared service by default
OpenSearch        -> provision: external, platform/shared service by default
Kafka             -> provision: external, platform/shared service by default
```

`repo preflight` reports detected middleware evidence, and `repo autofix` or
`onboard generate` persists the plan in `opspilot.service.yaml`.

For auto-provisioned middleware, generated files include:

- `deploy/k8s/middleware-<kind>.yaml`.
- A Kubernetes Secret containing service-scoped connection variables.
- A middleware Deployment and Service with CPU/memory limits and probes.
- Application `envFrom` references so the service can consume the Secret.

Heavy middleware remains external by default because auto-starting Kafka,
OpenSearch, or RabbitMQ for every service can exhaust small test clusters. Those
dependencies should be backed by platform/shared instances until an explicit
policy enables dedicated instances.

Provisioning failures should be written as structured error events so OpsPilot
and AI assistants can read them directly:

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

## Config Sources

OpsPilot treats external configuration systems as config sources, not
middleware. The first supported source type is Apollo.

When a repository already has Apollo support, OpsPilot can generate:

- `deploy/k8s/configmap.yaml` with non-secret Apollo metadata such as AppID,
  cluster, namespaces, environment, and config service URL.
- Deployment `env` entries from that ConfigMap.
- Optional Deployment `args` for applications that read flags such as
  `--env=prod --cfg=http://apollo...`.
- Optional read-only `apollo.yaml` ConfigMap mount for applications that read a
  local config file.
- Optional `Secret` reference for Apollo tokens. OpsPilot references the Secret
  but does not generate or commit the token value.

Example for command-argument style applications:

```yaml
configSources:
  apollo:
    type: apollo
    required: true
    appId: task-server
    env: prod
    cluster: default
    namespaces: application,gms
    meta: http://apolloconfig-server-inner.tpo.xzoa.com
    tokenSecret: task-server-apollo-token
    inject: args
    envFlag: --env
    metaFlag: --cfg
```

This renders Deployment args similar to:

```yaml
args:
  - "--env=$(APOLLO_ENV)"
  - "--cfg=$(APOLLO_META)"
```

For Spring Boot or other file-based apps, use `inject: file` and optionally set
`mountPath`; OpsPilot mounts an `apollo.yaml` key from the generated ConfigMap.

If the application code does not support Apollo yet, OpsPilot should report the
config source as detected/planned evidence and let AI generate a code/config fix
plan. YAML alone cannot make an application consume Apollo.

## Storage

OpsPilot supports a first version of storage governance for clusters that still
use hostPath heavily.

Generated services may declare storage intent in `opspilot.service.yaml`.
OpsPilot then writes matching Deployment annotations, `volumeMounts`, and
volumes:

```text
logs/runtime/uploads -> hostPath under /data/opspilot/hostpath/<namespace>/<service>/<volume>
cache/temp           -> emptyDir with sizeLimit
```

Non-platform hostPath remains a release blocker because it can bypass capacity
planning and cleanup. Platform-managed hostPath is allowed, but it is soft
governance only: Kubernetes does not enforce a per-directory hard limit for a
plain hostPath mount. `sizeHint` and `retentionDays` are metadata for OpsPilot
inspection, cleanup planning, and a future node-side quota/retention agent.

For cache and temporary files, OpsPilot generates `emptyDir.sizeLimit` so a
single Pod has a bounded local scratch area.

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

## Front Gateway

The optional test front gateway is documented but not generated by OpsPilot in
this phase. If the network team or gateway owner configures the formal-side
entry to forward:

```text
*.test.tpo.xzoa.com -> one test ingress/APISIX/Nginx entry
```

then services can later be given stable test domains such as
`skillshub-api.test.tpo.xzoa.com`. This must stay optional: missing gateway
configuration should only produce a missing evidence note and must not fail
`repo preflight`, `repo autofix`, `onboard generate`, or release.

## Generated Files

```text
Dockerfile                    # only when requested and missing
.gitlab-ci.yml
deploy/k8s/namespace.yaml
deploy/k8s/limitrange.yaml
deploy/k8s/resourcequota.yaml
deploy/k8s/deployment.yaml
deploy/k8s/service.yaml
deploy/k8s/kustomization.yaml
.opspilot/quality.yaml
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

The shared `ci/templates/buildkit-gitops.go.yml` also includes:

- `preflight:onboarding`: checks `DOCKERFILE_PATH`, namespace guardrails,
  workload manifests, probes, resources, and BuildKit template usage.
- `code-precheck`: scans source code, writes
  `.opspilot/evidence/code-precheck.json`, and blocks only high-confidence
  dangerous findings.
- Frontend templates also run `prebuild:image-smoke` before image push and
  GitOps update. It uses BuildKit to build the final image filesystem and
  writes `.opspilot/evidence/frontend-image-smoke.json`, catching missing
  `index.html`, missing JavaScript assets, and common blank-page risks such as
  Vue runtime-only inline templates without compiler support.
- BuildKit packaging, GitOps update, and Argo Application registration in the
  app-of-apps `apps/kustomization.yaml`.

Initial platform templates are available for:

```text
ci/templates/buildkit-gitops.go.yml
ci/templates/buildkit-gitops.node.yml
ci/templates/buildkit-gitops.python.yml
ci/templates/buildkit-gitops.frontend.yml
ci/templates/buildkit-gitops.java.yml
```

If the repository is missing release files, generate them:

```powershell
opspilot repo autofix --project tpo/devex/skillshub/skillshub-api --write
```

## Registry Auth

Generated Deployments and ServiceAccounts reference the standard
`gitlab-registry-pull` image pull Secret. This fixes the observed node200
failure mode where Pods reached `ErrImagePull` / `ImagePullBackOff` because the
node206 GitLab Registry returned `403 Forbidden` for anonymous pulls.

The Secret value is still platform-owned. Do not commit registry credentials
into a service repository or GitOps app directory. The platform bootstrap layer
must make the read-only `gitlab-registry-pull` Secret available in managed
namespaces before or during release.

If a future cluster uses containerd-level registry credentials and does not need
per-namespace pull Secrets, keep the Secret name as a harmless compatibility
contract or disable it through a platform template option after review.

`docker-hub.tpo.xzoa.com` is an explicit exception path, not the default release
registry. Do not push generated service images there unless the platform owner
confirms that exception.
