# Optional Parca Profiling

Parca is an optional profile evidence source for OpsPilot. It is not part of
the default deployment and must not block Kubernetes, logs, metrics, release, or
host troubleshooting.

## Position

```text
Parca Agent DaemonSet
-> Parca Server
-> OpsPilot parca datasource
-> profiles status/link and capabilities profile_evidence
```

Do not merge Parca Agent into `opspilot-agent`. Parca Agent needs node-level
profiling permissions for eBPF/perf sampling, while `opspilot-agent` should
remain a lightweight read-only host evidence helper.

## Test Install

Use the Parca upstream Kubernetes install method or Helm chart in the test
cluster only. The official Helm chart is:

```powershell
helm repo add parca https://parca-dev.github.io/helm-charts
helm repo update parca
helm upgrade --install parca parca/parca --namespace parca --create-namespace
```

If the environment cannot download the chart or images, leave Parca uninstalled.
OpsPilot will report `profile_evidence_not_ready` and continue with the rest of
the evidence chain.

After Parca Server is reachable in-cluster, add this datasource to
`platform/opspilot-config`:

```yaml
datasources:
  - name: parca-node200
    kind: parca
    environment: test
    region: chengdu-inner
    url: http://parca.parca.svc.cluster.local:7070
```

## Verify

```powershell
.\opspilot\scripts\opspilot.ps1 --output human profiles status
.\opspilot\scripts\opspilot.ps1 --output human profiles link --namespace opspilot --service opspilot-core --since 10m
.\opspilot\scripts\opspilot.ps1 --output human capabilities
```

Expected behavior when Parca is not installed:

```text
profile_evidence: missing or not_ready
```

Existing inspect/release/log/metric commands must continue to work.

## Boundaries

- Test cluster first.
- No production install in this phase.
- No release blocking.
- No deep flamegraph parsing inside OpsPilot yet.
- No app code changes required.
