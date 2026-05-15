# opspilot-core

Online read-only API for OpsPilot.

Initial responsibilities:

- Inventory API.
- Kubernetes resource API.
- Kubernetes Pod logs on demand.
- Prometheus query proxy and resource TopN.
- ELK query proxy for gateway/business logs.
- Evidence Pack assembly.
- Auth, audit, rate limit, timeout, cache, and redaction boundaries.

The implementation language is Go. Python is kept for future async worker work
only when it is the better fit.

## Run MVP

```bash
go run ./opspilot/core --host 127.0.0.1 --port 18080
```

Implemented MVP endpoints:

- `GET /api/health`
- `GET /api/inventory/overview`
- `GET /api/k8s/pods`
- `GET /api/k8s/logs/pod`
- `GET /api/context/pod`
- `GET /api/diagnose/pod`

Runtime modes:

- in-cluster Kubernetes API through service account token.
- local `kubectl` fallback.
