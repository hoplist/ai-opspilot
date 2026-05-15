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
