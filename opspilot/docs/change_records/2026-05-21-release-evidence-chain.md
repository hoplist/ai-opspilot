# 2026-05-21 Release Evidence Chain

## Change

Added the first release evidence chain design for integrating the CI/GitOps
flow into OpsPilot as a read-only troubleshooting capability.

## Scope

The design covers:

- GitLab pipeline evidence.
- BuildKit image build evidence.
- GitLab Registry image evidence.
- GitOps desired-state evidence.
- Argo CD sync and health evidence.
- Kubernetes rollout, Pod, metrics, and logs evidence.
- Explicit evidence gaps when a datasource is missing.

## Decision

OpsPilot should not replace CI/CD tools. It should query and correlate them so
AI can answer where a release is blocked after a developer pushes code.

## First Milestone

Start with `release status --service <name>` as a read-only aggregator. Keep
write operations and rollback automation out of scope until the evidence model
is stable.

