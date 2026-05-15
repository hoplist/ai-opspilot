# OpsPilot

OpsPilot is the clean-slate Go implementation of the previous auto-inspection
RCA platform.

The repository now keeps only the new implementation:

- `opspilot/` - Go core API, Go CLI, contracts, skill draft, and component docs.
- `deploy/opspilot/` - Kubernetes deployment manifests for the Go core service.
- `docs/opspilot/` - architecture, product notes, migration plan, and change records.

## Run Locally

Start the read-only core API:

```powershell
go run ./opspilot/core --host 127.0.0.1 --port 18080
```

Use the CLI:

```powershell
go run ./opspilot/cli schema
go run ./opspilot/cli inventory overview
go run ./opspilot/cli k8s pods --status abnormal
go run ./opspilot/cli k8s logs pod -n default --pod example --tail 100
go run ./opspilot/cli context pod -n default --pod example
go run ./opspilot/cli diagnose pod -n default --pod example
```

## Build

```powershell
$env:CGO_ENABLED="0"
$env:GOOS="linux"
$env:GOARCH="amd64"
go build -trimpath -ldflags="-s -w" -o build/linux-amd64/opspilot-core ./opspilot/core
go build -trimpath -ldflags="-s -w" -o build/linux-amd64/opspilot ./opspilot/cli
docker build -f opspilot/Dockerfile -t opspilot-core:0.1.0-mvp-go .
```

## Deploy

```powershell
kubectl apply -k deploy/opspilot/core
```

## Principles

- Go core and Go CLI are the default online path.
- Kubernetes access is read-only by default.
- Pod logs are read on demand through `pods/log`; no full log pipeline is required.
- Prometheus, ELK, OpenSearch, MinIO, MySQL, and eBPF are optional integrations, not
  core dependencies.
