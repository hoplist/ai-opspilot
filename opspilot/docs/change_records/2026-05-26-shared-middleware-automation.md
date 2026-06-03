# 2026-05-26 shared middleware automation

## Background

OpsPilot service onboarding already covers frontend/backend image build and
Kubernetes deployment generation. The next gap is middleware dependencies such
as databases, cache, message queues, object storage, and search services.

The platform should not default to one middleware instance per service in the
test environment. That would create too many Pods and waste resources,
especially for MySQL, PostgreSQL, Elasticsearch, Kafka, and RabbitMQ.

## Decision

For the test environment, use shared middleware instances by default and
allocate isolated logical resources per service:

| Middleware | Default mode | Isolation unit |
| --- | --- | --- |
| MySQL | shared | database + user |
| PostgreSQL | shared | database + user/schema |
| Redis | shared | key prefix or DB index |
| RabbitMQ | shared | vhost + user |
| MinIO/S3 | shared | bucket + access key |
| Elasticsearch/OpenSearch | shared | index prefix or namespace |
| Kafka | shared | topic prefix + ACL user |

Dedicated middleware instances are only for explicit special cases such as load
testing, version compatibility testing, strong isolation, or middleware-specific
configuration differences.

## User Experience

Developers should not need to understand middleware deployment details. OpsPilot
should explain the intent in plain language:

```text
Detected dependency: MySQL
Reason: go.mod imports a MySQL driver and config references DATABASE_URL.
Plan: use shared MySQL and create database/user for this service.
Injected secret: <service>-mysql-conn
```

## Automation Scope

The first implementation adds platform-side detection and generated intent:

- Detect middleware dependencies from common repository files.
- Classify each dependency with default shared mode.
- Generate stable resource names for logical allocations.
- Include middleware intent in `opspilot.service.yaml`.
- Include middleware checks in repository preflight output.

Runtime provisioning of shared databases/users/vhosts/buckets is a later step.
It should be handled by OpsPilot release automation or a dedicated controller,
using platform-owned admin credentials and writing only service-scoped
Kubernetes Secrets.

## Implemented In This Change

- Added `middleware` intent to generated `opspilot.service.yaml`.
- Added dependency detection for:
  - MySQL
  - PostgreSQL
  - Redis
  - RabbitMQ
  - S3-compatible object storage
  - OpenSearch/Elasticsearch
  - Kafka
- Added shared-mode allocation metadata:
  - `mode`
  - `allocation`
  - `resource`
  - `secret`
  - `env`
  - `reason`
- Added `repo preflight` middleware evidence output.
- Kept generated Kubernetes Deployments unchanged so missing runtime
  provisioning cannot break Pods.
- Skipped platform-generated files such as `opspilot.service.yaml` during
  dependency scanning to avoid self-referential evidence.
- Bumped CLI schema/version to `0.1.15-shared-middleware-intent`.

## Guardrails

- Do not auto-create heavy dedicated instances by default.
- Do not store admin credentials in business repositories.
- Do not expose generated passwords in CLI output.
- Production can remain fully automatic, but must enforce platform policies:
  resource limits, approved middleware types, backup requirements, and
  forbidden external/root credentials.

## Follow-up

Later work should add:

- shared middleware pool configuration per environment and organization group;
- provisioning commands for MySQL/PostgreSQL/Redis/RabbitMQ/MinIO;
- release evidence that checks application Pods, middleware allocation,
  generated Secrets, and connection checks together;
- `inspect service` output that shows missing dependency evidence clearly.

## Validation

Ran:

```text
go test ./...
```

Also ran a local demo repository containing MySQL and Redis client dependencies.
`repo autofix --write` generated `opspilot.service.yaml` with shared MySQL and
Redis intent, and `repo preflight` passed after generated files existed.

## 2026-06-03 automatic lightweight middleware

After the `demo-test` full-flow retest, middleware intent alone was not enough
for the desired small-team workflow. OpsPilot now generates lightweight
middleware manifests for common development dependencies:

- Redis: Secret, Deployment, Service, resource limits, TCP probes.
- MySQL: Secret, Deployment, Service, resource limits, TCP probes.
- PostgreSQL: Secret, Deployment, Service, resource limits, TCP probes.
- S3-compatible storage: MinIO Secret, Deployment, Service, resource limits,
  HTTP health probes.

The generated application Deployment now:

- uses a service-specific ServiceAccount;
- references `gitlab-registry-pull` for node206 GitLab Registry image pulls;
- injects middleware connection Secrets through `envFrom`;
- keeps CPU/memory requests and limits on both application and middleware
  containers.

Heavy middleware such as Kafka, RabbitMQ, and OpenSearch remains detected as
`provision: external` by default. They should be backed by platform/shared
services unless a future policy explicitly enables dedicated instances, because
auto-starting them for every small service can exhaust a single-node or small
test cluster.

Validation:

- `go test ./cli ./core/...`
- Generated a temporary FastAPI demo with Redis signals through
  `opspilot onboard generate --write --force`.
- Verified generated files include `serviceaccount.yaml`,
  `middleware-redis.yaml`, application `envFrom`, and `gitlab-registry-pull`.
- Verified `kubectl kustomize deploy/k8s` renders Namespace, ServiceAccount,
  application Deployment, Redis Secret, Redis Deployment, and Redis Service.

Release inspection defect fix:

- `inspect service` / release status no longer fails immediately when a new
  service is missing from `OPSPILOT_RELEASE_SERVICES`.
- The release registry falls back to a read-only Kubernetes Deployment lookup
  by service name and returns current Pod/log/metric evidence.
- The result records `release_mapping_missing`, so AI can explain that
  GitLab/GitOps evidence may be incomplete while Kubernetes evidence is still
  usable.
