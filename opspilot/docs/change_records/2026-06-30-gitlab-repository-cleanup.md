# 2026-06-30 GitLab Repository Cleanup

## Goal

Reduce the node206 GitLab project list to only the repositories needed for
inner-network synchronization and current OpsPilot operation.

## Backup

Before deleting projects, mirror backups were created under:

```text
D:\code\auto_inspection\backups\gitlab-cleanup-20260630-164742
```

The backup covers:

- `tpo/sandbox/devex/ai-loop-demo`
- `tpo/sandbox/devex/demo-api`
- `tpo/sandbox/devex/frontend-vite-demo`
- `tpo/sandbox/devex/fullstack-go-api`
- `tpo/sandbox/devex/fullstack-vue-web`
- `tpo/sandbox/devex/java-spring-demo`
- `tpo/sandbox/devex/python-fastapi-demo`
- `tpo/sandbox/devex/resource-guardrail-demo`
- `tpo/ops/backups/test-cluster-backup`
- `tpo/ops/yaml`

## GitOps And Cluster Cleanup

`ai-loop-demo` was the only sandbox demo still referenced by Argo CD.

Actions:

- Removed `apps/devex-demo-ai-loop-demo-application.yaml` from
  `tpo/deploy/gitops-manifests`.
- Removed `clusters/test/apps/devex/demo/ai-loop-demo`.
- Removed the `apps/kustomization.yaml` reference.
- Pushed GitOps commit `f1ab02b`.
- Deleted the Argo CD Application `devex-demo-ai-loop-demo`.
- Deleted namespace `cicd-devex-demo`, which only contained the demo workload.

## GitLab Cleanup

Deleted:

- all `tpo/sandbox/devex/*` demo repositories listed above;
- `tpo/ops/backups/test-cluster-backup`;
- `tpo/ops/yaml`.

For demo repositories with Container Registry images, the cleanup deleted
registry tags first, including `buildcache`, then deleted the registry
repositories, then deleted the GitLab projects.

GitLab delayed deletion renamed projects to `*-deletion_scheduled-*`. The
scheduled projects were then removed from the GitLab Rails side through
`Projects::DestroyService` on node206.

## Remaining Required Repositories

The verified remaining GitLab project list after the follow-up compatibility
cleanup is:

| Path | Purpose |
| --- | --- |
| `tpo/deploy/gitops-manifests` | Argo CD desired state. |
| `tpo/ops/backups/node200-etcd-snapshots` | Active node200 etcd backups. |
| `tpo/platform/opspilot/opspilot-config` | Runtime config source. |
| `tpo/platform/opspilot/opspilot-core` | Current OpsPilot core source and CI. |
| `tpo/platform/opspilot/opspilot-skills` | Runtime skills source. |

## Validation

Verified after cleanup:

- GitLab project API returns only the five required projects above.
- Argo CD Applications are `Synced` and `Healthy`:
  - `argocd-bootstrap`
  - `argocd-core`
  - `obsidian-sync`
  - `opspilot-core`
  - `opspilot-prometheus`
  - `parca`
- No `demo`, `sandbox`, or `cicd` namespace remains in node200.

## Rollback

Source repositories can be restored from the local mirror backups if needed.
Registry images for deleted sandbox demos were intentionally not retained,
because they were temporary validation artifacts and are not part of the
inner-network baseline.
