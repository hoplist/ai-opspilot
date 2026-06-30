# OpsPilot core architecture

## Purpose

OpsPilot should be the single operations entrypoint for users who do not want
to learn Kubernetes, GitLab CI, Argo CD, Prometheus, ELK, or cluster
credentials. Thin clients, CLI binaries, and AI assistants should call OpsPilot
APIs. They should not carry local skills registries, kubeconfigs, or release
logic.

## Target Request Flow

```text
User natural language
-> CLI/Web/API thin client
-> OpsPilot intent parser
-> GitLab-backed skills registry and capability catalog
-> policy/risk classification
-> read-only evidence collection or plan-first mutation
-> AI-readable evidence pack
```

## Current Code Boundaries

| Area | Current package | Responsibility |
| --- | --- | --- |
| HTTP API | `core` | Builds datasources and exposes OpsPilot APIs. |
| CLI | `cli` | Thin command wrapper, human/table output, compatibility helpers. |
| Natural language intent | `internal/intent` | Deterministic parse from user text to action, command, risk, and automation mode. |
| Skills registry | `internal/skillregistry` | Loads GitLab-backed skill routing metadata for backend use. |
| Kubernetes evidence | `internal/k8s` | Pod, event, log, workload, and Argo CD CR evidence. |
| Metrics evidence | `internal/prometheus` | Prometheus datasource registry and resource queries. |
| Logs evidence | `internal/logsearch` | ELK/OpenSearch search and APISIX/service correlation. |
| Release evidence | `internal/release` | GitLab, Registry, GitOps, Argo CD, quality, release, and rollback evidence. |
| Datasource routing | `internal/datasource` | Resolves bounded ES/OpenSearch query routes from service, host, cluster, and region metadata. |
| Repository upload target | `internal/repoupload` | Server-owned GitLab project create/reuse for sandbox repository upload. |

## Natural Language Policy

Natural-language parsing is deterministic first. The first version intentionally
maps a small set of intents to stable commands:

| Intent | Example | Risk | Automation |
| --- | --- | --- | --- |
| `inspect_service` | `check opspilot-core status` | `read_only` | `auto_execute` |
| `release_history` | `show opspilot-core release history` | `read_only` | `auto_execute` |
| `release_service` | `deploy opspilot-core` | `controlled_mutate` | `plan_first` |
| `rollback_service` | `rollback opspilot-core to <tag>` | `controlled_mutate` | `plan_first` |

High-risk operations such as namespace deletion, data deletion, GitLab project
deletion, or hostPath cleanup remain plan-only unless a future policy explicitly
allows them.

## CLI Direction

`cli/main.go` is still too large. The safe migration path is incremental:

1. Keep command behavior stable.
2. Move shared parsing and policy into `internal/*` packages.
3. Split CLI commands by domain after behavior is covered by tests:
   - `inspect`
   - `release`
   - `metrics`
   - `logs`
   - `skills`
   - `quality`
   - `ask`
   - `output`
   - `httpclient`
4. Keep CLI as a thin client. Server-side APIs should own skills, cluster
   catalogs, credential catalogs, and policy.

`repo upload --confirm` now follows the first thin-client migration phase:
`opspilot-core` creates or reuses the GitLab project and enforces allowed
target bases; the CLI still performs local precheck, local git status, and
local `git push` because the server cannot read the user's local repository
unless a future archive-upload mode is added.

## Multi-Cluster Model

OpsPilot can manage multiple clusters when cluster state is represented as a
server-side datasource catalog:

```text
client request with optional --cluster
-> OpsPilot API
-> cluster registry
-> Kubernetes datasource
-> Prometheus/log/release datasource
-> evidence pack with active cluster name
```

Clients should not receive kubeconfigs. Remote cluster credentials should live
in Kubernetes Secrets or an external secret manager and be referenced by
OpsPilot configuration.

The first implementation supports:

- `OPSPILOT_CLUSTER_CATALOG` entries with `kubernetes:in-cluster`,
  `kubernetes:remote`, or `kubernetes:kubeconfig`.
- Remote kubeconfig paths such as
  `/var/run/opspilot/clusters/<secret-name>/kubeconfig`.
- Optional kubeconfig context selection through `context:<name>`.
- Cluster-aware Kubernetes inventory, Pod logs, Pod context, Pod diagnosis,
  release status, quality jobs, lifecycle janitor/healer evidence, and
  capability checks.

Thin clients pass `--cluster <name>` only. If the requested cluster is not in
the server-side catalog, OpsPilot returns an explicit missing registration error
instead of falling back silently.

## Read-Only Catalog APIs

OpsPilot exposes catalog metadata so users and AI can understand what the
platform can see without exposing secret values:

```text
GET /api/credentials/catalog
GET /api/clusters/catalog
GET /api/services/catalog
```

CLI wrappers:

```text
opspilot credentials catalog --output human
opspilot clusters catalog --output human
opspilot services catalog --output human
```

Initial configuration can come from environment variables:

```text
OPSPILOT_CREDENTIAL_CATALOG="name=opspilot-release-secrets,class=platform-runtime,scope=node200/opspilot,storage=kubernetes-secret,namespace=opspilot,used_by=opspilot-core|argocd,permissions=read_gitlab|write_gitops_confirmed"

OPSPILOT_CLUSTER_CATALOG="node200-test=environment:test,kubernetes:in-cluster,prometheus:node200-k8s,gitops_project:tpo/deploy/gitops-manifests,path:clusters/test,argocd_ns:argocd,registry:192.168.48.206:5050"
```

Remote example:

```text
OPSPILOT_CLUSTER_CATALOG="node200-test=environment:test,kubernetes:in-cluster,prometheus:node200-k8s,path:clusters/test;prod-a=environment:prod,kubernetes:remote,secret:opspilot-cluster-prod-a,kubeconfig:/var/run/opspilot/clusters/opspilot-cluster-prod-a/kubeconfig,context:prod-a,prometheus:prod-a,logs:prod-elk,path:clusters/prod-a"
```

The catalog stores metadata only. It must not include token values, passwords,
kubeconfig contents, or database passwords.

## Productization Spine

The current platform spine is:

```text
service catalog
-> evidence pack
-> audit trail
-> risk boundary
-> plan-first or read-only action
```

The first version intentionally avoids new middleware:

- audit records are JSONL files;
- recent Evidence Packs are JSON files;
- service catalog metadata comes from runtime configuration/GitOps;
- event-driven packs use a lightweight in-process scanner;
- observability integrations stay as adapters.

New API surfaces:

```text
GET /api/audit/recent
GET /api/audit/policy
GET /api/evidence/pack
GET /api/evidence/packs/recent
GET /api/logs/route
POST /api/repo/upload-target
```

CLI wrappers:

```text
opspilot audit recent --output human
opspilot audit policy --output human
opspilot evidence pack --target-type service --service opspilot-core --output human
opspilot evidence packs --output human
opspilot logs route --host <domain> --output pretty
```

High-risk operations remain plan-only until the platform has explicit
authorization, audit retention, and before/after validation evidence.

## Next Refactor Steps

1. Move capability construction out of `core/main.go` into an internal
   capability catalog package.
2. Move more AI/skill recommendations from CLI evidence builders to backend
   responses.
3. Split CLI command implementations once backend ownership is clear.
4. Switch live Argo CD from the compatibility `clusters/test/argocd-core`
   source path to `platform/argocd/overlays/node200-test` only after running
   `tools/argocd-render-diff` and completing sync/health checks.
