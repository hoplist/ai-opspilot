# 2026-06-04 Multi-cluster execution and Argo CD portability

## Goal

Continue the previously deferred platform governance items:

- cluster-aware execution beyond metadata-only catalogs;
- portable Argo CD core packaging.

## Implemented

- Added server-side Kubernetes cluster registry in `internal/k8s`.
  - Default cluster keeps the existing in-cluster behavior.
  - Remote clusters can be selected from `OPSPILOT_CLUSTER_CATALOG`.
  - Remote kubeconfig paths are server-side only, for example
    `/var/run/opspilot/clusters/<secret-name>/kubeconfig`.
  - Optional kubeconfig context is supported.
- Extended cluster catalog metadata.
  - `kubeconfig`
  - `kubeconfig_path`
  - `context`
  - `kube_context`
- Updated OpsPilot core routes to resolve the active cluster per request.
  - capabilities
  - Kubernetes Pods
  - Kubernetes Pod logs
  - Pod context and diagnosis
  - release status evidence
  - quality status and quality run jobs
  - recent error evidence
  - inventory overview
- Updated CLI to pass `--cluster` without handling kubeconfig contents.
  - global `--cluster`
  - inspect/check service, pod, cluster
  - fix service/pod dry-run
  - release service/status/jobs/logs/history/rollback
  - quality run/status
  - janitor/healer/decommission evidence paths
- Added a portable Argo CD Kustomize package in the GitOps repository:

```text
platform/argocd/
  base/
  overlays/node200-test/
```

The portable package is a non-destructive copy of the current
`clusters/test/argocd-core` resources. The live Argo CD Application still points
at the compatibility path until a separate planned GitOps migration changes it.
- Registered `node200-test` in the deployed OpsPilot core ConfigMap through
  `OPSPILOT_CLUSTER_CATALOG`, with the server-side kubeconfig directory
  reserved for future remote clusters.
- Updated the source deployment template
  `deploy/opspilot/core/configmap.yaml` with `OPSPILOT_CLUSTER` and
  `OPSPILOT_CLUSTER_KUBECONFIG_DIR` so the standard GitLab/BuildKit/GitOps
  release path preserves the runtime cluster registry settings.
- Added a safety check for catalog entries such as `node206-host` that are
  datasource bundles but not Kubernetes clusters. Kubernetes inspection now
  returns an explicit "no Kubernetes datasource" error instead of silently using
  the default kubectl context.
- Fixed a rollout-discovered bug where catalog-defined `kubernetes:in-cluster`
  clients did not inherit ServiceAccount connection settings. The registry now
  passes `KUBERNETES_SERVICE_HOST`, `KUBERNETES_SERVICE_PORT`, token path, and
  CA path into named in-cluster clients.
- Tightened CLI cluster inspection so non-Kubernetes datasource bundles fail
  fast instead of mixing a capability error with unrelated metric evidence.
- Documented read-only user configuration:
  - normal readonly DB account;
  - read replica/export copy for heavy reads;
  - separate writable metadata store when sync clients need to write local
    state;
  - temporary audited readonly access for production debugging.

## Configuration Example

```text
OPSPILOT_CLUSTER_CATALOG="node200-test=environment:test,kubernetes:in-cluster,prometheus:node200-k8s,path:clusters/test;prod-a=environment:prod,kubernetes:remote,secret:opspilot-cluster-prod-a,kubeconfig:/var/run/opspilot/clusters/opspilot-cluster-prod-a/kubeconfig,context:prod-a,prometheus:prod-a,logs:prod-elk,path:clusters/prod-a"
```

Remote kubeconfig values must be stored in Kubernetes Secrets or an external
secret manager. They must not be committed to Git or distributed to CLI users.

The CLI/API selection model is intentionally thin:

```text
opspilot clusters catalog --output human
opspilot check cluster --cluster node200-test
opspilot check service <service> --cluster prod-a
```

Clients select a registered cluster name only. OpsPilot core resolves the
server-side kubeconfig and datasource bundle.

## Validation

```text
go test ./opspilot/internal/catalog ./opspilot/internal/k8s ./opspilot/core ./opspilot/cli
kubectl kustomize D:\code\auto_inspection\gitops-manifests-work\platform\argocd\base
kubectl kustomize D:\code\auto_inspection\gitops-manifests-work\platform\argocd\overlays\node200-test
```

Both Kustomize paths render successfully.

## Boundary

- This change does not expose kubeconfig content through the CLI or catalog.
- This change does not switch the live `argocd-core` Application path yet.
- Production clusters should remain read-only evidence by default until their
  credential, datasource, GitOps, and rollback policy records are reviewed.

## Next

1. Add the required kubeconfig Secret and mount for a real remote cluster.
2. Register the remote cluster in `OPSPILOT_CLUSTER_CATALOG`.
3. Use `opspilot check cluster --cluster <name>` to verify read-only evidence.
4. In a separate GitOps change, switch `apps/argocd-core-application.yaml` from
   `clusters/test/argocd-core` to `platform/argocd/overlays/node200-test`.
