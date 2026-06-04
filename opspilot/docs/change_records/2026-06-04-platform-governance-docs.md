# 2026-06-04 Platform governance docs

## Goal

Document the next OpsPilot governance work before changing Argo CD or cluster
state.

## Added

- `docs/platform-governance-roadmap.md`
  - credential ledger and repository boundary;
  - one-page developer push flow;
  - business-service GitOps templating;
  - later Argo CD core portability;
  - multi-cluster OpsPilot model.
- `docs/developer-standard-flow.md`
  - what developers do;
  - what happens after push;
  - what developers should not touch;
  - how middleware and release checks are requested through OpsPilot.
- `docs/credential-ledger.md`
  - credential classes;
  - current credential inventory;
  - ledger record template;
  - future plan-first credential add flows;
  - temporary database debugging credential model.

## Decision

Do not refactor the large `argocd-core` YAML first. Stabilize governance,
credentials, repository permissions, and service GitOps templates before making
Argo CD itself portable.
