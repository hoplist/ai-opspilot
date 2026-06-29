# node206 curated manifests

This directory keeps only the first offline bootstrap manifests:

- compose/: GitLab, GitLab Runner, node-exporter, Prometheus, OpsPilot agent.
- gitops/apps/: Argo CD Application/AppProject entrypoints for Argo CD, OpsPilot, and monitoring.
- gitops/clusters/test/argocd-bootstrap: Argo CD CRDs.
- gitops/clusters/test/argocd-core: Argo CD control plane.
- gitops/clusters/test/apps/opspilot-core: OpsPilot core workload.
- gitops/clusters/test/apps/opspilot-rbac: OpsPilot namespace/RBAC baseline.
- gitops/clusters/test/apps/opspilot-prometheus: Prometheus service alias.

Excluded from the offline kit on purpose:

- observability experiments: OpenSearch, Fluent Bit, OTEL, Pyroscope, Falco, MinIO.
- temporary demos: devex demos, cicd-demo, obsidian-sync.
- mysql backup/recovery lab manifests.
- raw .git directories and source history snapshots.
- secret.yaml files.

Before starting `compose/opspilot-agent-docker-compose.yml`, copy
`compose/.env.example` to `compose/.env` and replace `OPSPILOT_AGENT_TOKEN`.
The compose file intentionally fails without this value so the agent is not
started with an empty token.
