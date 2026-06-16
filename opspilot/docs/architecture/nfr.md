# OpsPilot non-functional requirements

## Scope

OpsPilot is an internal operations entrypoint for test and controlled internal
environments. The first expected user group is about 30 to 50 people.

## Capacity

| Area | Current target |
| --- | --- |
| Human users | 30 to 50 internal users |
| CLI/API requests | Low concurrency, bursty troubleshooting |
| Evidence pack storage | Short retention, local JSON files |
| Audit storage | Short retention, local JSONL file |
| Log queries | Bounded by time window, result limit, and datasource route |

## Performance

- Health and catalog APIs should return within 1 second when local files are
  available.
- Kubernetes, Prometheus, GitLab, and ES calls should use explicit timeouts.
- ES/OpenSearch queries must stay bounded:
  - default short time window;
  - hard maximum lookback;
  - hard maximum result limit;
  - `track_total_hits=false` unless a future explicit count mode is added.

## Availability

- Target availability is internal-tool level, not production control-plane
  criticality.
- Missing Prometheus, ES, Kibana, APISIX, GitLab, or Argo CD evidence must be
  reported as an evidence gap instead of making unrelated Kubernetes inspection
  unusable.
- Invalid GitLab-managed config must not replace the last valid runtime
  snapshot during hot reload.

## Security And Permissions

- Current test environment may use network isolation instead of full user auth.
- The architecture must keep clean extension points for future auth, RBAC, and
  approval workflow.
- Secret values must not be returned by API responses, CLI output, audit logs,
  evidence packs, or error messages.
- High-risk operations remain plan-only until explicit authorization and
  before/after validation are implemented.

## Reliability

- GitOps is the desired-state recovery source for Kubernetes workloads.
- Daily etcd backup remains the cluster recovery baseline.
- GitLab-backed config and skills repositories are the recovery source for
  platform metadata.
- Local audit/evidence stores are useful for recent troubleshooting, not the
  system of record for long-term compliance.

## Maintainability

- Keep clients thin.
- Put policy, datasource routing, skills routing, and GitLab/GitOps actions in
  `opspilot-core`.
- Prefer small internal packages over new platform dependencies.
- Every high-risk capability must document the minimum verification method.
