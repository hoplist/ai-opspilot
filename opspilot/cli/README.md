# opspilot CLI

Deterministic command entrypoint for humans and AI agents.

The CLI should:

- Expose stable commands.
- Expose `schema`.
- Return JSON by default.
- Avoid direct cluster mutation.
- Route all data access through `opspilot-core`.

Example future commands:

```bash
opspilot schema
opspilot inventory overview
opspilot k8s pods --status abnormal
opspilot k8s logs pod --namespace prod --pod xxx --tail 300
opspilot context pod --namespace prod --pod xxx
opspilot diagnose pod --namespace prod --pod xxx
```

MVP invocation from source:

```bash
go run ./opspilot/cli schema
go run ./opspilot/cli inventory overview
go run ./opspilot/cli k8s pods --status abnormal
```
