# OpsPilot platform productization spine

## Background

OpsPilot already has Kubernetes inspection, Prometheus metrics, node206 Docker
agent evidence, release evidence, skills registry, onboarding, quality checks,
and plan-first janitor/healer flows. The next step is not to keep adding
scattered commands. The platform needs a small product spine:

- audit records;
- event-driven Evidence Packs;
- service catalog;
- observability adapter boundaries;
- safe action risk levels.

## Decision

Do not add new middleware in this stage.

Audit, Evidence Pack persistence, and service catalog metadata use local files
and GitOps/runtime configuration first. PostgreSQL, Kafka/RabbitMQ, Backstage,
Devtron, KubeVela, or a custom log/APM store are explicitly deferred.

## Added

| Area | Change |
| --- | --- |
| Audit | Added file-backed audit records for API requests. |
| Audit API | Added `GET /api/audit/recent` and `GET /api/audit/policy`. |
| Service catalog | Added `GET /api/services/catalog` backed by `OPSPILOT_SERVICE_CATALOG` plus release registry seeds. |
| Evidence Pack | Added `GET /api/evidence/pack` for service, Pod, and cluster targets. |
| Evidence Pack store | Added `GET /api/evidence/packs/recent` for persisted packs. |
| Event-driven packs | Added a lightweight in-process scanner that periodically converts recent Kubernetes/Argo/release/file events into persisted Evidence Packs. |
| Event target naming | Pod-driven event packs use the concrete Pod name as the target while keeping service metadata in the event evidence. |
| CLI | Added `audit recent`, `audit policy`, `services catalog`, `evidence pack`, and `evidence packs`. |

## Modified

| Area | Change |
| --- | --- |
| Release mapping | `release.Registry` now exposes release service items and can fall back to service catalog entries when `OPSPILOT_RELEASE_SERVICES` is empty. |
| CLI schema | Added audit, service catalog, and Evidence Pack commands. |
| API request handling | API requests now emit audit records with actor, action, target, risk, outcome, and sanitized query metadata. |
| Deployment config | The source deploy template now mounts writable local paths for audit, Evidence Pack, and file-event data because the core container uses a read-only root filesystem. |
| Release mapping hygiene | Removed stale demo release mappings from the OpsPilot core deploy template so GitOps updates do not reintroduce old demo services. |

## Deleted Or Deferred

| Item | Decision |
| --- | --- |
| New DB for catalog/audit | Deferred until query volume or retention requires it. |
| MQ for event handling | Deferred; the first version uses a lightweight scanner. |
| Log/APM storage | Not built into OpsPilot. ELK/OpenObserve/Loki/APISIX remain adapters. |
| High-risk auto execution | Still plan-only. Destructive operations require explicit validation and are not auto-executed. |
| More scattered CLI commands | Deferred; new work should aggregate into `inspect`, `release`, `service(s)`, `audit`, and `evidence`. |

## Runtime Configuration

New environment variables:

```text
OPSPILOT_AUDIT_LOG_PATH=/var/lib/opspilot/audit/audit.jsonl
OPSPILOT_EVIDENCE_PACK_DIR=/var/lib/opspilot/evidence-packs
OPSPILOT_EVENT_PACK_ENABLED=true
OPSPILOT_EVENT_PACK_INTERVAL_SECONDS=300
OPSPILOT_SERVICE_CATALOG="opspilot-core=environment:test,group:platform,project:opspilot,owner:platform,repo:platform/opspilot,namespace:opspilot,deployment:opspilot-core,container:core,source:node200-k8s,image:192.168.48.206:5050/platform/opspilot/opspilot-core,gitlab:platform/opspilot,gitops:clusters/test/apps/opspilot-core/deployment.yaml,argocd:opspilot-core"
```

`OPSPILOT_RELEASE_SERVICES` remains supported for compatibility. The target
direction is to keep richer metadata in `OPSPILOT_SERVICE_CATALOG`, then use
release mapping only as a compatibility surface.

## Risk Boundary

| Risk | Automation | Examples |
| --- | --- | --- |
| `read_only` | auto execute | inspect, metrics, logs, release status, audit recent |
| `controlled_mutate` | plan first or explicit confirm | release trigger, rollback, quality run |
| `high_risk` | plan only | namespace deletion, data deletion, hostPath cleanup, credential rotation |
| `forbidden` | blocked | secret value dump, unaudited destructive action, arbitrary shell |

## Minimum Validation

Local validation:

```powershell
go test ./...
go vet ./...
go run ./cli --output human audit policy
go run ./cli --output human services catalog
go run ./cli --output human evidence pack --target-type service --service opspilot-core
```

Release validation:

```powershell
.\scripts\opspilot.ps1 --output human release jobs --service opspilot-core
.\scripts\opspilot.ps1 --output human release status --service opspilot-core
kubectl -n argocd annotate application opspilot-core argocd.argoproj.io/refresh=hard --overwrite
kubectl -n opspilot rollout status deploy/opspilot-core --timeout=180s
.\scripts\opspilot.ps1 --output human audit recent --limit 5
.\scripts\opspilot.ps1 --output human services catalog
.\scripts\opspilot.ps1 --output human evidence pack --target-type service --service opspilot-core --persist
.\scripts\opspilot.ps1 --output human evidence packs --limit 5
```

## Expected Result

OpsPilot becomes easier for non-ops users and AI agents to reason about:

```text
service catalog -> evidence pack -> audit trail -> safe action boundary
```

Missing ELK/APISIX/service-log evidence remains an explicit adapter gap, not a
failure of Kubernetes or release inspection.
