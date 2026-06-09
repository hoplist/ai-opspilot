# Skills Import Plan

## Goal

OpsPilot skills are maintained server-side through the GitLab skills repository. External skill sources such as `garrytan/gstack` can be mirrored and inventoried, but candidates must not become runtime skills automatically.

## Changes

- Added `skills import-plan --name <candidate>` to generate a dry-run review plan from mirrored candidate metadata.
- Added `skills promote --name <candidate> --dry-run` as a safe alias for the same plan.
- Added backend endpoint `/api/skills/import-plan?name=<candidate>`.
- Import plans generate reviewable drafts for:
  - `skills/<name>/skill.yaml`
  - `skills/<name>/SKILL.md`
  - `skills/<name>/examples/<name>-example.md`
- Unsupported candidates stay blocked and return a reason instead of generated runtime files.

## Boundaries

- No automatic enablement.
- No server-side write in this phase.
- Runtime skills are still only the reviewed files under `skills/` in the GitLab skills repository.
- Client binaries do not carry or update the skills registry.

## Validation

- Unit tests cover candidate draft generation, unsupported candidate blocking, backend import-plan API, and CLI dry-run promote behavior.
- Standard release flow remains required for backend changes:
  node206 GitLab Runner -> BuildKit -> Registry -> GitOps -> node200 Argo CD.
