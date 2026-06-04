# 2026-06-04 Janitor, healer, and decommission v1

## Goal

Add the first OpsPilot-native version of Janitor Agents and Self-healing
Pipelines without creating a separate agent platform. The initial version is
aggressive in planning and diagnosis, but conservative in mutation: high-risk
actions produce plans only.

## Permission Model

Actions are classified into four risk levels:

- `read_only`: inspection, evidence gathering, diagnosis, and planning.
- `safe_mutate`: low-risk actions such as rerunning checks or preparing a fix
  commit after evidence classification.
- `controlled_mutate`: actions that may be automated later with `--confirm`,
  such as GitOps rollback, demo namespace cleanup, or GitOps workload removal.
- `high_risk`: plan only. No automatic execution.

High-risk examples:

- delete production applications or namespaces;
- delete PVC/PV/hostPath/database data;
- delete GitLab projects;
- modify cluster-level RBAC, CNI, StorageClass, or ingress controller.

## Application Deletion

OpsPilot did not previously have a first-class application deletion lifecycle.
Runtime deletion currently happens indirectly through GitOps and Argo CD
`prune`.

This version adds:

```text
opspilot app decommission plan --service <service>
```

The plan separates:

- GitOps Application/workload manifest removal;
- Argo CD prune behavior;
- namespace deletion risk;
- data-bearing resources such as PVC, PV, hostPath, databases, and middleware.

Data is kept by default. Data deletion and GitLab project deletion remain
blocked/high-risk actions.

## Demo Safety Test

Demo verification found and fixed one safety edge case: an unmapped service
could show GitOps removal actions even when the GitOps path or Argo CD
Application mapping was missing. Execution was still disabled in v1, so no
cluster mutation could happen, but the plan was too optimistic for a future
`--confirm` flow.

The decommission planner now requires complete namespace, deployment, GitOps
path, Argo CD Application, and `keep-data=true` evidence before any
`controlled_mutate` action is shown. If those mappings are missing, OpsPilot
keeps only read-only verification in allowed actions and moves GitOps,
namespace, data, and GitLab project removal to `high_risk`/`plan_only` blocked
actions.

## Implemented Commands

```text
opspilot janitor plan
opspilot healer diagnose --service <service>
opspilot app decommission plan --service <service>
```

`run`/`fix` execution subcommands intentionally return a disabled message in
v1. They will be enabled only after allow-list rules and confirmation handling
are implemented.

## Evidence

All three commands output AI-readable action objects with:

- `risk`
- `automation`
- `requires`
- `blocked_by`
- `rollback_hint`

This lets OpsPilot answer "what can be done automatically" without hiding
dangerous operations behind a black box.
