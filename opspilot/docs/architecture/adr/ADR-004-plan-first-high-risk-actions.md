# ADR-004: Keep high-risk actions plan-first

## Status
Accepted

## Context

OpsPilot is allowed to be more active in internal test environments, but some
actions can still delete data, remove applications, or damage hostPath storage.

## Decision

Classify actions by risk. Read-only actions can execute automatically.
Controlled mutations require explicit confirmation or plan-first behavior.
High-risk actions return plans with minimum validation steps. Forbidden actions
are blocked.

## Consequences

### Positive
- AI can help diagnose and propose fixes without silently deleting state.
- Operators get a clear minimum verification path before risky work.
- Future approval workflow can attach to the same risk model.

### Negative
- Some operations remain slower until approval/auth is implemented.

### Neutral
- Internal test mode can keep auth light while preserving future policy shape.

## Alternatives Considered

- Allow all actions inside the internal network: rejected because mistakes in
  test can still damage shared nodes and storage.
- Disable all mutations: rejected because release trigger, rollback, and
  quality jobs are useful controlled operations.
