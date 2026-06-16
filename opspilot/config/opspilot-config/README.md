# OpsPilot Config

This repository is the human-maintained runtime configuration source for the
test OpsPilot deployment.

OpsPilot loads YAML files recursively from the repository root. Keep these
directories small and explicit:

- `settings/`: platform defaults such as GitLab URL, GitOps project, quality
  runner metadata, and default cluster.
- `credentials/`: credential catalog metadata. Passwords are allowed only for
  internal test datasource access and are redacted by OpsPilot APIs.
- `clusters/`: cluster and GitOps placement metadata.
- `datasources/`: Prometheus, Elasticsearch/OpenSearch, APISIX, and app log
  datasource definitions.
- `agents/`: read-only node agent endpoints.
- `assets/`: advisory-only network zones, planned JumpServer/Prometheus asset
  sources, and optional static assets.
- `services/`: service catalog, runtime mapping, release mapping, and log
  correlation hints.
- `topology/`: region and network path hints.

Validate before pushing:

```powershell
go run ./opspilot/cli --output human config validate --dir ./opspilot/config/opspilot-config
```

Runtime should report:

```text
Config: source=file valid=true
```
