# ADR-001: Keep clients thin and move policy to opspilot-core

## Status
Accepted

## Context

OpsPilot is used by CLI, AI assistants, and future web entrypoints. If clients
own GitLab project creation policy, datasource routing, credential handling, or
skills routing, every client becomes a policy surface and long-term maintenance
gets harder.

## Decision

Keep clients thin. `opspilot-core` owns policy, catalogs, datasource routing,
credentials, skills routing, audit, and controlled mutation APIs.

The CLI may still perform local-only work such as repository detection, code
precheck, git status, and git push until a server-side source archive upload
path exists.

## Consequences

### Positive
- One policy implementation for CLI, AI, and web.
- Tokens and kubeconfigs stay server-side when possible.
- Audit is centralized.

### Negative
- Some CLI commands require backend availability.
- Fully thin repository upload needs a later archive upload design.

### Neutral
- Local developer convenience remains available for read-only planning and
  source detection.

## Alternatives Considered

- Keep GitLab project creation in CLI: rejected for long-term policy drift.
- Upload all source archives to core now: deferred because it increases API and
  storage surface before the current test workflow needs it.
