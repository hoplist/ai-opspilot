# 2026-05-27 CLI AI Evidence Loop

## Goal

Make OpsPilot usable by non-technical users while keeping the skill layer thin.
The CLI should execute deterministic inspections, return AI-readable evidence,
and let AI use that evidence to suggest code or configuration fixes.

## Scope

- Add a `doctor` command for local CLI/backend/capability self-checks.
- Keep `check` as the beginner-friendly alias for `inspect`.
- Add `check release <service>` as a natural release-status inspection alias.
- Add `--output evidence` for fixed AI-readable evidence packs.
- Add `fix ... --dry-run` as a safe planning command that gathers evidence and
  recommends code/config/release next actions without mutating repositories or
  clusters.

## Intended User Flow

```powershell
opspilot doctor --output human
opspilot check service skillshub --output evidence
opspilot fix service skillshub --dry-run --output evidence
```

The skill should only call CLI commands and summarize the result. The CLI/Core
remain the source of operational evidence.
