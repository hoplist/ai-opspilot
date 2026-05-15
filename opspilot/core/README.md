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

The preferred implementation language is Go once API contracts are frozen.

## Run MVP

```bash
python -m opspilot.core --host 127.0.0.1 --port 18080
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
