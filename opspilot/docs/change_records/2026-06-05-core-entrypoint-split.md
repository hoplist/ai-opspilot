# 2026-06-05 Core entrypoint split

## Goal

Continue reducing `opspilot-core` backend entrypoint size without changing API
behavior.

## Changed

- Split HTTP route registration from `core/main.go` into `core/routes.go`.
- Split HTTP wrapper, request parsing helpers, error conversion, and env helpers
  into `core/http.go`.
- Split capability aggregation and Pod metric enrichment into
  `core/capabilities.go`.
- Split route registration by domain:
  - `core/routes_system.go`
  - `core/routes_catalog.go`
  - `core/routes_kubernetes.go`
  - `core/routes_metrics.go`
  - `core/routes_logs.go`
  - `core/routes_release.go`

## Current Shape

```text
core/main.go          startup and datasource construction
core/routes.go        route-domain dispatcher
core/routes_*.go      domain route registration
core/http.go          HTTP wrappers and request helpers
core/capabilities.go  capability and evidence-source summary
```

`core/main.go` is now only the service entrypoint and dependency assembly file.

## Validation

```text
go test ./...
go vet ./...
```

## Next

- Move capability aggregation into `internal/capability` once its response
  contract is stable enough to be reused by CLI tests and docs.
- Add focused route tests by domain when the next endpoint behavior change
  happens.
