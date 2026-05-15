# opspilot-core deployment

MVP deployment for the Python standard-library `opspilot-core`.

Build the image from repository root:

```bash
docker build -f opspilot/Dockerfile -t opspilot-core:0.1.0-mvp .
```

Deploy:

```bash
kubectl apply -k deploy/opspilot/core
```
