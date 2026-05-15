# opspilot-core deployment

MVP deployment for the Go `opspilot-core`.

Build the image from repository root:

```bash
docker build -f opspilot/Dockerfile -t opspilot-core:0.1.0-mvp-go .
```

Deploy:

```bash
kubectl apply -k deploy/opspilot/core
```
