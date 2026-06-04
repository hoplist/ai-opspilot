# 2026-06-04 Core intent architecture

## Goal

Move OpsPilot toward a backend-owned natural-language and skills execution
model. Users and thin clients should ask OpsPilot what to do; the backend should
own intent parsing, risk classification, skills metadata, and evidence routing.

## Implemented

- Added `internal/intent`.
  - Converts natural language into a deterministic action.
  - Returns the target service, stable CLI command, risk class, automation mode,
    confidence, warnings, and next steps.
  - Keeps mutating release and rollback requests as `plan_first`.
- Added backend `/api/intent/parse`.
  - Uses configured release services from the backend registry.
  - Keeps parsing side-effect free.
- Updated CLI `ask` / `nl`.
  - Calls backend `/api/intent/parse` first.
  - Falls back to the shared local parser only for version compatibility when
    the backend endpoint is unavailable.
- Added read-only platform catalogs.
  - `/api/credentials/catalog` exposes credential metadata without secret
    values.
  - `/api/clusters/catalog` exposes cluster datasource metadata.
  - `opspilot credentials catalog` and `opspilot clusters catalog` query the
    backend only.
- Added `docs/opspilot-core-architecture.md`.
  - Documents the thin-client model, backend-owned skills, risk policy,
    multi-cluster target model, and CLI refactor path.

## Design Decision

Do not make the CLI the long-term owner of natural language or skills routing.
The CLI can display results and provide compatibility, but OpsPilot core should
own:

- runtime skills registry;
- intent parsing;
- policy/risk classification;
- evidence collection;
- plan-first mutation decisions;
- multi-cluster datasource routing.

## Risk Boundary

- `inspect_service` and `release_history` are read-only.
- `release_service` and `rollback_service` are controlled mutations and should
  be plan-first from natural language.
- High-risk lifecycle operations remain plan-only.

## Follow-Up

- Move capability construction out of `core/main.go`.
- Split `cli/main.go` by command domain after this backend boundary is stable.
- Move more AI-readable recommendations from CLI evidence formatting into
  backend responses.
