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

## Deferred

- Deep Parca query API integration.
- Flamegraph data parsing inside OpsPilot.
- Production deployment.
- Automatic profile regression detection after release.
