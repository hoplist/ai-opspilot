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
