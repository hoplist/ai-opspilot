# OpsPilot Config Repository

OpsPilot supports a GitLab-managed configuration directory as the human-editable
source for services, log datasources, credentials, topology, and correlation
rules. The runtime still supports legacy environment variables, so existing
deployments can migrate gradually.

## Repository Shape

Recommended GitLab project:

```text
platform/opspilot-config
```

Repository root layout:

```text
settings/
credentials/
clusters/
datasources/
agents/
services/
topology/
assets/
correlation/
schemas/
.gitlab-ci.yml
```

The sample structure in this repo is under `config/opspilot-config/`.

## Runtime Loading

Set:

```bash
OPSPILOT_CONFIG_DIR=/etc/opspilot/config
```

OpsPilot loads all `.yaml` and `.yml` files recursively from that directory.
In Kubernetes, the recommended mount path is:

```bash
OPSPILOT_CONFIG_DIR=/etc/opspilot/config/current
```

`opspilot-config-init` pulls the repository once before startup and
`config-sync` keeps it updated. OpsPilot can hot reload the directory with:

```bash
OPSPILOT_CONFIG_RELOAD_SECONDS=60
```

The config file values are merged with legacy env values:

1. legacy env remains valid for compatibility;
2. YAML values are appended and can override same-name catalog entries;
3. missing YAML sections keep using existing env/default behavior.

The current deployment keeps only bootstrap values in the Kubernetes ConfigMap:

- listener port;
- config Git sync URL/ref/period;
- retention paths and size limits;
- skills Git sync URL/ref/period;
- secret-backed execution tokens.

Service topology, datasources, cluster catalog, node agents, release mappings,
credential catalog, and quality runner metadata live in the GitLab config repo.

## Asset Network Zones

Server asset correlation starts with advisory-only network zones. The first
stage does not call JumpServer, does not write Prometheus, and does not remove
targets. It only classifies IPs and reports missing asset evidence.

Example:

```yaml
network_zones:
  - name: chengdu-inner
    region: chengdu
    zone: inner
    cidrs:
      - 10.65.0.0/16
    coverage: partial
    action_policy: advisory_only

asset_sources:
  - name: jumpserver-chengdu-inner
    kind: jumpserver
    region: chengdu
    network_zone: chengdu-inner
    enabled: false
    coverage: partial
```

Commands:

```powershell
opspilot assets zones --output human
opspilot assets inspect --ip 10.236.12.19 --output human
opspilot assets diff --output human
```

Use `coverage: partial` when JumpServer does not fully cover a network zone.
`action_policy: advisory_only` means OpsPilot can say "missing" or "candidate
for removal", but it must not delete JumpServer assets or Prometheus targets.

## Credential Policy

For the current internal test environment, credentials can be stored in the
private GitLab config repository as plaintext so humans can maintain them
without hidden state.

Runtime API behavior:

- OpsPilot uses the password to connect to datasources when configured.
- `credentials catalog` and `/api/config/status` do not return the password.
- They only show `username` and `password_set=true`.

This keeps the maintenance model simple now while leaving a future migration
path to Kubernetes Secret or Vault.

## Validation

Local validation:

```powershell
go run ./opspilot/cli --output human config validate --dir ./opspilot/config/opspilot-config
```

Runtime status after deployment should show:

```text
Config: source=file valid=true dir=/etc/opspilot/config/current
```

Runtime status:

```powershell
go run ./opspilot/cli --output human config status
```

Optional hot reload:

```bash
OPSPILOT_CONFIG_RELOAD_SECONDS=60
```

When enabled, OpsPilot periodically reloads the config directory. Invalid YAML
is reported and ignored; the previous valid runtime snapshot keeps serving
requests.

The first implementation validates YAML parseability, required fields, duplicate
names, credential references, and generated catalog compatibility. JSON schema
files are provided under `config/schemas/` for GitLab CI or manual validation.

## Log Correlation

Service configs can set:

```yaml
correlation:
  require_uri: false
```

This means a user can ask for domain/status/time-window RCA without providing a
full URI. URI or path prefixes improve confidence, but are not mandatory.

Evidence strength rules remain explicit:

- `strong`: shared request id or trace id exists across gateway and app logs.
- `medium`: same domain/service/time window with gateway and service evidence.
- `weak`: only one side has useful evidence.
- `missing`: required log source or mapping is absent.

## Log Datasource Routing

Before querying logs, OpsPilot can explain the shortest bounded route:

```powershell
opspilot logs route --host todo.tpo.xzoa.com --output human
opspilot logs route --service todo-server --output pretty
opspilot logs route --region chengdu-inner --global --output pretty
```

Routing order:

1. service/domain exact match;
2. cluster default log datasource;
3. same-region ES/OpenSearch datasource;
4. neighbor-region datasource from `topology/`;
5. global datasource search only when `--global` is explicit.

Kibana datasources are kept as UI metadata and are not used as query targets.
Elasticsearch/OpenSearch datasources are the only log query targets.
