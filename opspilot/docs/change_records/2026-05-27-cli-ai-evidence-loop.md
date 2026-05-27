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

## Demo Validation

- Created a small Go service `platform/devex/demo/ai-loop-demo` to validate the
  loop with a real application failure.
- First revision intentionally crashed at startup with
  `AI_LOOP_DEMO_BOOT_ERROR: config file /app/config.yaml not found`.
- OpsPilot `check pod --output evidence` identified the unhealthy Pod,
  Kubernetes log bytes, missing ELK evidence, and runtime/configuration likely
  cause.
- OpsPilot `fix pod --dry-run --output evidence` produced a safe repair plan
  without mutating code or cluster state.
- The code was fixed, pushed through GitLab Runner -> BuildKit -> Registry ->
  GitOps -> Argo CD, and the service recovered to `Running` with zero restarts.
- Registered `ai-loop-demo` in the OpsPilot release service mapping so
  beginner commands such as `check service`, `check release`, and
  `release status` can inspect this demo by service name.
- During validation, Argo reported the demo release as degraded even though the
  Pod was healthy because multiple service Applications tried to own the same
  project namespace. The demo service now leaves shared namespace ownership to
  the platform/project layer instead of including `namespace.yaml` in its
  service kustomization.
