# 2026-06-09 runtime skills source adaptation design

## Goal

Design how OpsPilot should use public Coding Agent skill recommendations, such
as the Linux.do skill-pack discussion, without making clients install local
skills or importing arbitrary external workflows directly.

Reference: https://linux.do/t/topic/1802808

## Added

- Added `docs/runtime-skills-source-adaptation.md`.
- Documented the server-side runtime skill model:
  GitLab skills repository -> git-sync -> OpsPilot local runtime directory ->
  approved OpsPilot command/API execution.
- Documented how external sources such as `garrytan/gstack`, SuperClaude,
  Anthropic skills, Vercel agent skills, MiniMax skills, Composio skills, and
  awesome-skill lists should be treated as idea sources rather than direct
  runtime dependencies.
- Documented the recommended skill package shape:
  `skill.yaml`, `SKILL.md`, optional `references/`, optional `examples/`, and
  optional `tests/`.
- Documented a phased plan for examples, import tooling, validation, and
  fallback bundled skills.

## Decision

OpsPilot should keep `skill.yaml` small and deterministic. Detailed behavior
belongs in `SKILL.md`, `references/`, and `examples/`. The backend should call
approved OpsPilot capabilities, not arbitrary commands defined by external
skills.

## Next Work

- Add examples and decision trees for the highest-value skills.
- Add validation tooling for `skill.yaml`.
- Add image-bundled fallback skills so Pod recreation does not depend only on
  GitLab availability.
