# 2026-05-28 Skills Registry

## Goal

Fold the most useful local Codex skills into OpsPilot without turning the
platform into an unconstrained skill runner. OpsPilot remains the deterministic
source of operational evidence, while skills provide AI routing metadata and
safe follow-up guidance.

## Integrated first

- `opspilot-ops`: default CLI-first investigation entry.
- `auto-inspection-rca`: RCA evidence grouping and AI-readable fix planning.
- `kubernetes-specialist`: Pod, workload, event, readiness, restart, and probe
  troubleshooting rules.
- `monitoring-expert`: Prometheus, filesystem, log-source, and capability-gap
  reasoning.
- `devops-engineer`: GitLab Runner, BuildKit, Registry, GitOps, Argo CD,
  rollback, and repository governance.
- `debugging-wizard`: hypothesis-driven debugging from logs, events, metrics,
  and release evidence.

## Changes

- Added a static OpsPilot skills registry in Core.
- Added `GET /api/skills/registry`.
- Added `opspilot skills registry`.
- Added `skills_registry` to capability detection.
- Added skill recommendations to `inspect`, `check`, `fix --dry-run`, and
  `--output evidence` results so AI can route follow-up work to the right
  domain rules.

## Boundary

The registry does not execute arbitrary skill code. It maps approved skills to
existing OpsPilot commands, evidence sources, and safe boundaries. Cluster and
release mutations still go through the existing OpsPilot command contracts and
standard GitLab Runner -> BuildKit -> Registry -> GitOps -> Argo CD flow.
