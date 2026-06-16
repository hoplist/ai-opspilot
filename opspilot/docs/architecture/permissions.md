# OpsPilot permissions model

## Current Internal Test Mode

OpsPilot currently runs in an internal test environment where network isolation
is the main boundary. Full user authentication and approval workflow are
planned extension points, not blockers for test usage.

## Risk Levels

| Risk | Automation | Examples |
| --- | --- | --- |
| `read_only` | auto execute | inspect, metrics, logs, catalogs, audit recent |
| `controlled_mutate` | confirm or plan-first | release trigger, rollback, optional quality job |
| `high_risk` | plan only | namespace deletion, data deletion, hostPath cleanup, credential rotation |
| `forbidden` | blocked | secret value dump, unaudited destructive action, arbitrary shell |

## Thin Client Boundary

Clients should not own:

- kubeconfig contents;
- GitLab project creation policy;
- datasource credentials;
- skills registry decisions;
- high-risk action policy.

Clients may still perform local-only work:

- repository detection;
- local code precheck;
- local git status;
- local git push until server-side archive upload exists.

## Audit Requirements

Every API request should emit:

- actor when available;
- method and path;
- target type and target;
- risk level;
- outcome;
- sanitized query metadata.

Audit must not store passwords, tokens, kubeconfig contents, or raw Secret
values.

## Future Auth Extension

The future auth layer can be added in front of `opspilot-core` without changing
the core command model:

```text
Ingress / gateway auth
-> identity headers
-> OpsPilot policy check
-> audit
-> read-only action or plan-first mutation
```

Approval workflow can use the same risk levels: only `controlled_mutate` and
selected `high_risk` plans need extra workflow.
