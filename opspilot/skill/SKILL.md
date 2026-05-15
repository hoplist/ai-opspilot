# OpsPilot Skill

Use this skill when the user asks AI to inspect infrastructure, Kubernetes Pods,
server resources, short-window Pod logs, or RCA evidence through OpsPilot.

## Core Rule

Use deterministic OpsPilot commands first. Do not guess cluster state from memory.
Do not run raw `kubectl`, shell, SQL, or destructive commands when an OpsPilot
command exists.

## Command Entry

Prefer:

```bash
opspilot schema
opspilot inventory overview
opspilot k8s pods --status abnormal
opspilot k8s logs pod -n <namespace> --pod <pod> --tail 300
opspilot context pod -n <namespace> --pod <pod>
opspilot diagnose pod -n <namespace> --pod <pod>
```

When running from repository source before installing the binary, replace
`opspilot` with `go run ./opspilot/cli`.

Set backend URL when needed:

```bash
set OPSPILOT_BACKEND_URL=http://<opspilot-core>:18080
```

## Workflow

For cluster overview:

1. Run `opspilot inventory overview`.
2. Summarize counts, abnormal resources, and warnings.

For abnormal Pods:

1. Run `opspilot k8s pods --status abnormal`.
2. Identify namespace, pod, phase, readiness, restarts, and waiting reasons.

For a Pod RCA:

1. Run `opspilot context pod -n <namespace> --pod <pod>`.
2. If needed, run `opspilot diagnose pod -n <namespace> --pod <pod>`.
3. Use current and previous logs only as short-window evidence.
4. State confidence and missing evidence clearly.

## Boundaries

Read-only by default:

- inventory
- k8s pods
- k8s logs pod
- context pod
- diagnose pod

Do not perform:

- delete
- patch
- rollout restart
- scale
- exec
- attach
- port-forward
- secret value reads
