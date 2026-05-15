# opspilot-core deployment

MVP deployment for the Go `opspilot-core`.

Build the image from repository root:

```bash
$env:CGO_ENABLED="0"
$env:GOOS="linux"
$env:GOARCH="amd64"
go build -trimpath -ldflags="-s -w" -o build/linux-amd64/opspilot-core ./opspilot/core
go build -trimpath -ldflags="-s -w" -o build/linux-amd64/opspilot ./opspilot/cli
docker build -f opspilot/Dockerfile -t opspilot-core:0.1.0-mvp-go .
```

Deploy:

```bash
kubectl apply -k deploy/opspilot/core
```

The service exposes `18080` inside the cluster and NodePort `32180` for direct
access from the node200 network.
