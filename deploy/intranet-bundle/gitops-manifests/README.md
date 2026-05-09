# GitOps Manifests

This repository is managed by Codex bootstrap on 2026-04-24.

## Layout

- `source/deploy`: copy of the local `deploy/` directory.
- `source/yaml`: copy of the local `yaml/` directory.
- `clusters/test/observability`: curated Argo CD sync path for the test observability stack.
- `apps`: Argo CD AppProject and Application manifests.
- `apps/aigateway-auto-inspection-mcp-registration.json`: AI Gateway registration payload template for `auto-inspection-mcp` with a readonly tool allowlist. Apply it through the AI Gateway API after the gateway endpoint exists.

Secrets are currently present where they already existed in local deployment manifests. For production, replace them with SealedSecret, ExternalSecret, or SOPS.

## Argo CD Bootstrap

- `apps/argocd-bootstrap-project.yaml` and `apps/argocd-bootstrap-application.yaml` manage cluster-scoped Argo CD bootstrap resources.
- `clusters/test/argocd-bootstrap` currently pins the Argo CD CRDs from `v3.3.8`.
- `ServerSideApply=true` is enabled to avoid large-CRD annotation size issues during sync.
- `apps/argocd-core-project.yaml` and `apps/argocd-core-application.yaml` manage the live-exported Argo CD control plane resources in `clusters/test/argocd-core`.
- `clusters/test/argocd-bootstrap` manages the three Argo CD CRDs (`Application`, `AppProject`, `ApplicationSet`).

## AI Gateway MCP Registration

- `auto-inspection-mcp` should be registered as a streamable HTTP MCP server at `http://auto-inspection-rca.observability.svc.cluster.local:18081/mcp`.
- Only the readonly tools in `apps/aigateway-auto-inspection-mcp-registration.json` should be exposed.
- Mutation tools such as Kubernetes apply/delete, Argo CD sync/rollback, GitLab push/merge, and pipeline trigger remain denied.

