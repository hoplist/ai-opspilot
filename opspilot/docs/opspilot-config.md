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

Directory layout:

```text
opspilot-config/
  credentials/
  datasources/
  services/
  topology/
  correlation/
```

The sample structure in this repo is under `config/opspilot-config/`.

## Runtime Loading

Set:

```bash
OPSPILOT_CONFIG_DIR=/etc/opspilot/config
```

OpsPilot loads all `.yaml` and `.yml` files recursively from that directory.
The config file values are merged with legacy env values:

1. legacy env remains valid for compatibility;
2. YAML values are appended and can override same-name catalog entries;
3. missing YAML sections keep using existing env/default behavior.

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
