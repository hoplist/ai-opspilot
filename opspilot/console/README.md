# opspilot-console

Web UI for OpsPilot.

The console should call `opspilot-core` only. It should not talk directly to
Kubernetes, Prometheus, or ELK.

Primary views:

- Inventory overview.
- Abnormal resources.
- Pod / workload context.
- Metrics TopN.
- Pod logs on demand.
- Release context.
- Investigation summary.
