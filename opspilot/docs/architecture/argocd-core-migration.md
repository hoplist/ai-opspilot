# Argo CD core live path migration

## Goal

Switch the live `argocd-core` Application source path from the compatibility
layout to the portable Kustomize overlay only after render diff and health
validation.

## Current And Target Paths

Current compatibility path:

```text
clusters/test/argocd-core
```

Target portable path:

```text
platform/argocd/overlays/node200-test
```

## Render Diff Procedure

Render old and new paths from the GitOps repository:

```powershell
go run ./opspilot/tools/argocd-render-diff `
  --old <gitops-root>/clusters/test/argocd-core `
  --new <gitops-root>/platform/argocd/overlays/node200-test
```

The diff must compare:

- resource identity: kind, namespace, name;
- Deployment images, args, env, resources, and volumes;
- Service exposure including NodePort;
- RBAC subjects and verbs;
- important ConfigMap keys.

## Live Migration Procedure

1. Back up the GitOps repository state.
2. Run render diff and review the output.
3. Change only the `argocd-core` Application source path.
4. Keep prune disabled for the first sync.
5. Hard refresh Argo CD.
6. Verify:
   - `argocd-core` is Synced and Healthy;
   - pods in namespace `argocd` are Running;
   - `opspilot-core` can still sync and report release status.
7. Retire the compatibility path only in a later cleanup change.

## Stop Conditions

Do not switch the live path when:

- render output is missing resources that exist in the current path;
- Service exposure changes unexpectedly;
- RBAC changes are not understood;
- the target overlay cannot render locally.
