# 2026-06-04 GitLab-only runtime skills

## Goal

Make `platform/opspilot-skills` the runtime source of truth for OpsPilot
skills. Thin clients and CLI users should only ask the OpsPilot backend for the
active skills registry. They should not rely on local Codex skills or a client
embedded registry.

## Changes

- OpsPilot backend now returns GitLab-backed dynamic skills as source
  `gitlab`.
- Dynamic skills are no longer merged with the embedded registry when
  `OPSPILOT_SKILLS_DYNAMIC_ENABLED=true`.
- If the GitLab-backed skills directory is missing or empty, the backend reports
  `gitlab-unavailable` with warnings instead of silently pretending the
  embedded registry is active.
- `opspilot skills registry` no longer falls back to a CLI embedded registry
  when the backend is unavailable.
- The seed `opspilot/skills-repo` now includes all 10 currently integrated
  skills:
  - `opspilot-ops`
  - `auto-inspection-rca`
  - `kubernetes-specialist`
  - `monitoring-expert`
  - `devops-engineer`
  - `debugging-wizard`
  - `code-reviewer`
  - `security-reviewer`
  - `secure-code-guardian`
  - `database-optimizer`

## Runtime Model

```text
GitLab platform/opspilot-skills
-> git-sync sidecar
-> /opt/opspilot/skills/current/skills
-> OpsPilot backend /api/skills/registry
-> CLI/thin clients
```

The client may display skills returned by the backend, but it must not claim a
local registry as active platform state.

## Remaining Follow-up

Some CLI evidence commands still attach skill recommendations locally. The next
hardening step is to move those recommendation calculations into backend
responses so all AI routing and skill guidance is produced from the GitLab
runtime registry.
