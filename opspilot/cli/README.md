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
go run ./opspilot/cli metrics health
go run ./opspilot/cli metrics datasources
go run ./opspilot/cli metrics nodes --source all --limit 10
go run ./opspilot/cli metrics pods --source node200-k8s -n opspilot --sort memory --limit 10
go run ./opspilot/cli metrics containers --source node206-host --sort cpu --limit 10
go run ./opspilot/cli metrics filesystems --source all --output table
go run ./opspilot/cli k8s pods --status abnormal
go run ./opspilot/cli docker agents
go run ./opspilot/cli docker containers --host node206
go run ./opspilot/cli docker logs --host node206 --container gitlab --tail 300
go run ./opspilot/cli diagnose docker --host node206 --container gitlab
go run ./opspilot/cli logs search -n ai-dev --pod deer-flow-provisioner-8b47f95bf-t8rbt --query error --limit 10
go run ./opspilot/cli evidence request --host workflow.tpo.xzoa.com --uri /api/hr/queryUserScheduleList --service-index workflow-server* --service-uri-field msg --since 900
go run ./opspilot/cli evidence request --uri /api/hr/queryUserScheduleList --service-index workflow-server* --service-uri-field msg --service-only
go run ./opspilot/cli inspect pod -n ai-dev --pod sandbox-errno36-test --source node200-k8s --output human
go run ./opspilot/cli inspect cluster --source all --output human
go run ./opspilot/cli release status --service opspilot-core --output human
```

Build a local binary:

```powershell
.\opspilot\scripts\build-cli.ps1
.\build\opspilot.exe --backend-url http://192.168.48.200:32180 metrics health
```

The wrapper prefers the built binary when `build\opspilot.exe` exists, and
falls back to `go run` when it does not:

```powershell
.\opspilot\scripts\opspilot.ps1 metrics health
```

Cross-build examples:

```powershell
.\opspilot\scripts\build-cli.ps1 -TargetOS linux -TargetArch amd64
.\opspilot\scripts\build-cli.ps1 -TargetOS windows -TargetArch amd64
```
