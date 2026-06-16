# ADR-002: Use GitOps as the Kubernetes desired-state source of truth

## Status
Accepted

## Context

OpsPilot releases update Kubernetes workloads through GitLab CI, image
registry, GitOps, and Argo CD. Direct cluster mutation would be faster but
harder to audit and roll back.

## Decision

Use GitOps manifests as the desired-state source of truth for workload changes.
Controlled release and rollback actions submit GitOps changes, then Argo CD
reconciles the cluster.

## Consequences

### Positive
- Release state is reviewable in Git history.
- Rollback can be represented as a GitOps change.
- Cluster recovery can replay desired state from Git.

### Negative
- Release completion depends on both GitLab and Argo CD.
- Stale Argo CD cache can temporarily disagree with GitOps state.

### Neutral
- Argo CD is required for the current test release chain, but should remain an
  adapter rather than a mandatory dependency for all read-only inspection.

## Alternatives Considered

- Direct `kubectl set image`: rejected because it bypasses GitOps history.
- Full deployment platform replacement: rejected as too heavy for current
  internal test usage.
