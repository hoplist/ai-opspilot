# 2026-06-11 HTTP Timeout Hardening

## Background

Understand-Anything and code review found that OpsPilot already has route-level
timeouts, but two network boundaries still relied on default Go HTTP behavior:

- GitLab release evidence client used an `http.Client` without a default
  timeout.
- `opspilot-core` and `opspilot-agent` used `http.ListenAndServe` directly,
  without server read/write/idle timeout settings.

## Change

- Added a 30 second default timeout to the internal GitLab API client.
- Replaced direct `http.ListenAndServe` calls with explicit `http.Server`
  instances in `opspilot-core` and `opspilot-agent`.
- Server timeout policy:
  - `ReadHeaderTimeout`: 5 seconds
  - `ReadTimeout`: 30 seconds
  - `WriteTimeout`: 35 seconds
  - `IdleTimeout`: 60 seconds

## Risk And Boundaries

- No API route, request payload, response shape, or CLI command behavior was
  changed.
- `WriteTimeout` is slightly larger than the core route context timeout so
  normal 30 second bounded operations can still return their error response.
- This is a runtime hardening change only; it does not change authentication or
  permission policy.

## Validation

- Run `go test ./...`.
- Run `go vet ./...`.
