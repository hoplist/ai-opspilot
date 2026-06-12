# 2026-06-12 GitLab Managed OpsPilot Config

## Goal

Move multi-region service, datasource, credential, and correlation configuration
out of long environment variables and into a GitLab-maintained YAML config
model. The model must stay human-maintainable, avoid code changes for ordinary
application onboarding, and keep legacy env configuration working during
migration.

## Decisions

- Add `OPSPILOT_CONFIG_DIR` as the runtime config directory.
- Load YAML recursively from that directory.
- Keep legacy env configuration as compatibility input.
- Allow internal test credentials to be stored in the private config repository
  as plaintext for easier manual maintenance.
- Do not return plaintext passwords from OpsPilot APIs or CLI catalog/status
  output.
- Do not require full URI for APISIX/application-log correlation; domain,
  status, time window, and service mapping are allowed as a first RCA path.

## Implemented

- Added `internal/configloader`.
  - Reads single-resource YAML docs such as `kind: Service`.
  - Reads bulk files such as `credentials: [...]`.
  - Produces legacy-compatible catalog strings for existing registries.
  - Attaches datasource credentials at runtime.
  - Redacts password values from config status output.
- Added runtime config integration in `opspilot-core`.
  - Kubernetes cluster catalog can come from YAML.
  - Prometheus datasource list can come from YAML.
  - Logsearch URL, APISIX index, service index, route rules, and basic auth can
    come from YAML.
  - Service and credential catalogs can come from YAML.
- Added `/api/config/status`.
- Added optional config hot reload:
  - `OPSPILOT_CONFIG_RELOAD_SECONDS=<seconds>`
  - routes and scheduled Evidence Pack scans read the latest valid runtime
    snapshot;
  - invalid config reloads are ignored and the previous valid snapshot remains
    active.
- Added CLI commands:
  - `opspilot config validate --dir <config-dir>`
  - `opspilot config status`
- Added example config repository tree under `config/opspilot-config/`.
- Added JSON schema files under `config/schemas/` for manual or GitLab CI use.
- Updated evidence request correlation so `--uri` is optional and `--status` is
  supported.

## Not Implemented In This Step

- No remote GitLab project creation yet.
- No GitLab CI pipeline for `platform/opspilot-config` yet.
- No Secret/Vault migration. Internal test credentials can remain plaintext in
  the private config repository.

## Phase 2: Runtime Env Migration To GitLab Config

Goal:

- Move mutable OpsPilot runtime mappings out of the `opspilot-core` ConfigMap
  and into the GitLab-managed `platform/opspilot-config` repository.
- Keep only bootstrap/runtime-local values in the Kubernetes ConfigMap:
  listener port, config Git sync URL/ref/period, retention paths/limits, skills
  Git sync URL/ref/period, and other storage-loop settings that must exist
  before config loading.
- Keep sensitive execution tokens in Kubernetes Secret or GitLab CI variables;
  OpsPilot APIs must continue to return only redacted credential metadata.

Implemented in this phase:

- Added config model support for:
  - `settings`: default cluster, kubeconfig directory, GitLab URL, GitOps
    project/ref, optional quality runner settings.
  - `agents`: read-only node agent endpoints such as node206.
- Updated runtime wiring so these values are read from config files first and
  legacy env remains a fallback:
  - Kubernetes cluster catalog/default.
  - Prometheus datasources/default.
  - Elasticsearch/OpenSearch/APISIX/service-log defaults.
  - Release service mappings through the service catalog.
  - GitLab/GitOps release datasource metadata.
  - Node agent endpoints and optional token references.
- Replaced the local example config with the current node200/node206 test
  platform configuration:
  - `settings/platform.yaml`
  - `credentials/platform.yaml`
  - `clusters/node200.yaml`
  - `datasources/prometheus.yaml`
  - `datasources/elasticsearch.yaml`
  - `agents/node206.yaml`
  - `services/platform/opspilot-core.yaml`
- Slimmed `deploy/opspilot/core/configmap.yaml` by removing long catalog envs:
  `OPSPILOT_CLUSTER_CATALOG`, `OPSPILOT_PROMETHEUS_DATASOURCES`,
  `OPSPILOT_NODE_AGENTS`, `OPSPILOT_LOGSEARCH_URL`,
  `OPSPILOT_APISIX_INDEX`, `OPSPILOT_SERVICE_CATALOG`,
  `OPSPILOT_RELEASE_SERVICES`, `OPSPILOT_CREDENTIAL_CATALOG`, and quality
  runner envs.
- Added `opspilot-config-init` and `config-sync` git-sync containers:
  - init container pulls config once before `opspilot-core` starts;
  - sidecar keeps `/etc/opspilot/config/current` updated every 60 seconds;
  - `opspilot-core` reads `OPSPILOT_CONFIG_DIR=/etc/opspilot/config/current`
    and hot reloads every 60 seconds.
- Added schema files for `Agent`, `Cluster`, and `Settings`.

Boundary:

- `OPSPILOT_GITLAB_TOKEN` remains secret-provided env because it is an execution
  credential for GitLab API calls, not human-maintained topology config.
- Git sync credentials remain in Kubernetes Secret and are consumed only by
  git-sync sidecars.
- Retention paths/limits remain ConfigMap bootstrap settings for now because
  they are needed before stores are constructed and are local process behavior,
  not service topology.

## Minimum Validation

```powershell
go test ./opspilot/internal/configloader ./opspilot/internal/catalog ./opspilot/internal/logsearch ./opspilot/core ./opspilot/cli
go vet ./opspilot/...
go run ./opspilot/cli --output human config validate --dir ./opspilot/config/opspilot-config
```

## Validation Result

- `go test ./opspilot/...` passed.
- `go vet ./opspilot/...` passed.
- `go run ./opspilot/cli --output human config validate --dir ./opspilot/config/opspilot-config` passed.
- Runtime smoke passed:
  - `/api/config/status` returned `valid=true`.
  - `/api/services/catalog` loaded `todo-server`.
  - `password_returned=false`.
- Hot reload smoke passed:
  - service catalog changed from `todo-server` to `workflow-server` after
    `OPSPILOT_CONFIG_RELOAD_SECONDS=1`.

## Risk Boundary

- Invalid YAML is reported as invalid config and does not expose fake success.
- Passwords are only used internally for datasource auth and are not returned by
  catalog/status APIs.
- Missing APISIX trace/request id remains an evidence gap, not a strong request
  correlation.
