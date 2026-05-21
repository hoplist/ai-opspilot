# Release Evidence Chain

OpsPilot should treat the CI/GitOps flow as a read-only evidence chain. It
does not replace GitLab CI, BuildKit, the image registry, GitOps, or Argo CD.
It queries them, correlates their state, and explains where a release is
currently blocked.

## Target Flow

```text
developer git push
-> node206 GitLab pipeline
-> node206 GitLab Runner job
-> BuildKit rootless image build
-> push image to node206 GitLab Registry
-> update GitOps repository
-> node200 Argo CD sync
-> Kubernetes rollout
-> Pod metrics and logs
```

## User Experience

Developer workflow stays simple:

```bash
git push
```

Then the operator can ask AI through OpsPilot:

```text
show me where the opspilot-core release is now
```

OpsPilot should answer with:

```text
service: opspilot-core
stage: rollout
status: degraded
evidence:
  pipeline: success
  image: exists
  gitops: updated
  argocd: synced
  deployment: progressing
  pods: not ready
gap:
  service logs not found in ELK
next:
  inspect new Pod readiness and recent Kubernetes events
```

## CLI Shape

First read-only commands:

```powershell
opspilot release status --service opspilot-core
opspilot release evidence --service opspilot-core --commit <sha>
opspilot release diagnose --service opspilot-core
```

Initial service mapping is configured through:

```text
OPSPILOT_RELEASE_SERVICES="opspilot-core=namespace:opspilot,deployment:opspilot-core,container:core,source:node200-k8s,image:192.168.48.206:5050/platform/opspilot/opspilot-core,gitlab:platform/opspilot,gitops:clusters/test/apps/opspilot-core/deployment.yaml,argocd:opspilot-core"
OPSPILOT_GITLAB_URL="http://192.168.48.206:8929"
OPSPILOT_GITOPS_PROJECT="platform/gitops-manifests"
OPSPILOT_GITOPS_REF="main"
OPSPILOT_GITLAB_TOKEN="<read-only token>"
```

The first implementation can return `unknown` for unavailable datasources, but
must explicitly report the missing evidence instead of failing silently.

## Data Model

```json
{
  "service": "opspilot-core",
  "environment": "test",
  "commit": "abc123",
  "image": "192.168.48.206:5050/platform/opspilot/opspilot-core:abc123",
  "stage": "rollout",
  "status": "healthy|progressing|degraded|failed|unknown",
  "evidence": {
    "gitlab_pipeline": {
      "status": "success|failed|running|unknown",
      "url": "",
      "job_count": 0
    },
    "buildkit": {
      "status": "success|failed|unknown",
      "image_digest": ""
    },
    "registry": {
      "status": "exists|missing|unknown",
      "tag": ""
    },
    "gitops": {
      "status": "updated|missing|unknown",
      "commit": "",
      "image_tag": ""
    },
    "argocd": {
      "sync_status": "Synced|OutOfSync|Unknown",
      "health_status": "Healthy|Progressing|Degraded|Unknown"
    },
    "kubernetes": {
      "deployment": "",
      "namespace": "",
      "ready_replicas": 0,
      "desired_replicas": 0
    },
    "logs": {
      "kubernetes_log_bytes": 0,
      "elk_hits": 0
    }
  },
  "gaps": [
    "gitlab_token_missing",
    "argocd_datasource_missing",
    "elk_logs_missing"
  ],
  "next_checks": []
}
```

## Datasources

Required eventually:

- GitLab API: pipeline, job, commit, artifacts.
- GitLab Registry API: image tag and digest existence.
- GitOps repository: image tag diff and desired state.
- Argo CD API or Kubernetes read-only App CR: sync and health.
- Kubernetes API: Deployment, ReplicaSet, Pod, events, short-window logs.
- Prometheus: Pod and node resource evidence.
- ELK: service logs when available.

## Boundaries

- OpsPilot release commands are read-only by default.
- OpsPilot should not push images.
- OpsPilot should not mutate GitOps repositories unless a future explicit
  write-capable workflow is approved.
- OpsPilot should not call `argocd app sync` automatically.
- Rollback should start as `rollback-plan`, not an automatic rollback.

## First Milestone

Implement `release status` as a read-only aggregator:

1. Read service mapping from config.
2. Query Kubernetes Deployment desired image and rollout state.
3. Query Argo CD sync/health from the Application CR when `argocd:<app>` is configured.
4. Query GitLab latest pipeline when `gitlab:<project>` and `OPSPILOT_GITLAB_TOKEN` are configured.
5. Query GitLab Registry tag evidence from the Deployment image tag.
6. Query GitOps desired image from the configured manifest path.
7. Query matching Pods.
8. Query Pod logs and metrics.
9. Report unavailable GitLab/Registry/GitOps evidence as explicit gaps.

Create the token as an optional Kubernetes Secret in the `opspilot` namespace:

```bash
kubectl -n opspilot create secret generic opspilot-release-secrets \
  --from-literal=OPSPILOT_GITLAB_TOKEN='<read-only token>'
```
