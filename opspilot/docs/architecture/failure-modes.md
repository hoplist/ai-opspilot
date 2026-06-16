# OpsPilot failure modes

## GitLab Unavailable

Impact:
- release jobs/history/rollback may be unavailable;
- `repo upload --confirm` cannot create or reuse the target GitLab project;
- GitOps updates cannot be submitted.

Minimum response:
- return an explicit `gitlab_datasource_missing` or GitLab API error;
- do not report release success;
- keep Kubernetes and Prometheus evidence usable.

## GitOps Update Failed

Impact:
- image may be built but not deployed;
- rollback may not submit the desired-state change.

Minimum response:
- include GitLab job evidence when available;
- identify GitOps project/path/ref;
- provide the next command to inspect release jobs/logs.

## Argo CD Out Of Sync Or Unhealthy

Impact:
- GitOps desired state may not match cluster state.

Minimum response:
- show Argo CD sync and health state;
- hard refresh before assuming stale evidence is current;
- verify rollout status in Kubernetes after Argo CD sync.

## Elasticsearch/OpenSearch Unavailable

Impact:
- APISIX/application log evidence is missing.

Minimum response:
- return `logs.datasource_unavailable` or query error;
- continue Kubernetes logs, events, metrics, and release evidence;
- do not fallback to unbounded global search.

## Kibana Unavailable

Impact:
- human UI links may be unavailable.

Minimum response:
- do not fail log querying solely because Kibana is unavailable;
- query ES/OpenSearch datasource when configured.

## Prometheus Unavailable

Impact:
- CPU, memory, restart, and filesystem metrics may be missing.

Minimum response:
- return `prometheus_datasource_missing` or query error;
- continue Kubernetes status, events, logs, GitLab, GitOps, and Argo CD evidence.

## Kubeconfig Or Cluster Registration Missing

Impact:
- remote cluster inspection cannot run.

Minimum response:
- return a clear cluster registration error;
- do not silently fallback to another cluster.

## Skills Git Sync Unavailable

Impact:
- latest GitLab-managed skills may not load.

Minimum response:
- use image-bundled core skills fallback;
- report the runtime skill source and warning.

## Local Evidence Store Full

Impact:
- new audit/evidence writes may fail.

Minimum response:
- retention cleanup runs before reaching the `emptyDir` size limit;
- if writes fail, report a warning/error instead of pretending persistence
  succeeded.
