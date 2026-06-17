# Parca Optional Profile Evidence

## Background

OpsPilot already correlates Kubernetes state, logs, metrics, release evidence,
host disk/network evidence, and asset network zones. CPU-heavy Go services need
function-level evidence when metrics only show "CPU is high". Parca fits this
gap as an optional continuous profiling source.

The primary runtime is Go, with some Python services. Go profiling evidence is
expected to be higher confidence than Python because symbol and stack quality
depends on runtime and build details.

## Goal

Add Parca as an optional, non-blocking profile evidence source:

- keep Parca Agent separate from `opspilot-agent`;
- add `parca` datasource metadata to OpsPilot config;
- expose read-only profile datasource/status/link evidence;
- include profile capability as available or missing evidence;
- do not make Parca required for inspect, release, logs, or metrics.

## Safety Boundary

Parca Agent needs node-level profiling permissions such as eBPF/perf access.
For this phase:

- do not merge Parca into `opspilot-agent`;
- do not deploy to production clusters;
- do not block existing troubleshooting when Parca is missing;
- do not require app code changes;
- do not collect or expose secrets through profile APIs.

OpsPilot only reasons from evidence that exists. If Parca is missing, the
result must say profile evidence is unavailable and continue with Kubernetes,
logs, metrics, release, and host evidence.

## Design

Configuration:

```yaml
datasources:
  - name: parca-node200
    kind: parca
    environment: test
    cluster: node200-test
    region: chengdu-inner
    url: http://parca.parca.svc.cluster.local:7070
```

CLI/API:

- `profiles datasources`
- `profiles status`
- `profiles link --service <service> --namespace <ns> --pod <pod> --since 10m`

Evidence behavior:

- `profile_evidence` is `ready` only when a Parca datasource is configured and
  reachable.
- `profile_evidence_missing` is reported when Parca is not configured or not
  reachable.
- Generated links are hints for flamegraph/UI inspection; they do not replace
  deterministic metrics/logs evidence.

## Deployment Plan

Use Parca Server + Parca Agent as a separate optional module in the test
cluster. Keep it out of the default OpsPilot deployment.

Minimum validation:

1. Parca pods are Running in the test namespace.
2. `profiles datasources --output human` shows a configured datasource.
3. `profiles status --output human` reports ready or a clear unreachable
   reason.
4. `capabilities --output human` shows profile evidence as ready or missing.
5. Existing `inspect service`, `inspect pod`, and `release status` still work
   when Parca is missing.

## Follow-up: Test Cluster Install

Parca is installed as an independent optional GitOps application in the node200
test cluster:

- namespace: `parca`
- Argo CD application: `parca`
- GitOps path: `clusters/test/apps/parca`
- datasource cluster: `node200-test`

The Parca Agent remains separate from `opspilot-agent` because it needs
node-level profiling privileges. OpsPilot consumes only the configured Parca
service URL as optional profile evidence.

Minimum validation:

1. `kubectl -n parca get pods` shows Parca server and agent Pods.
2. `opspilot profiles status` shows `cluster=node200-test`.
3. If Parca is unavailable, OpsPilot reports `profile_evidence_not_ready`
   without blocking other evidence.

## Deferred

- Deep Parca query API integration.
- Flamegraph data parsing inside OpsPilot.
- Production deployment.
- Automatic profile regression detection after release.
