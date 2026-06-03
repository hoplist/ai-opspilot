# 2026-06-03 Vibecoding auto quality gate

## Goal

Clarify that pre-release checks are not an operations approval workflow. They
are an automatic OpsPilot quality gate for vibecoding users: catch
high-confidence mistakes early and return concrete AI-readable repair options.

## Decisions

- Human approval is not required.
- `blocker` is reserved for high-confidence failures that are likely to:
  - break runtime behavior, such as a frontend blank page;
  - expose secrets;
  - corrupt data;
  - endanger nodes or storage.
- `warning` does not block release. It should explain uncertainty and offer a
  safe follow-up plan.
- `suggestion` is reserved for future optimization-only advice.

## Evidence Additions

`repo precheck` and the frontend GitLab `code-precheck` template now include:

```json
{
  "policy": {
    "owner": "opspilot",
    "audience": "vibecoding",
    "mode": "automatic_quality_gate",
    "human_approval_required": false
  },
  "items": [
    {
      "gate": "auto_quality",
      "decision": "block_release",
      "audience": "vibecoding",
      "recommendation": "Use a .vue SFC with @vitejs/plugin-vue...",
      "fix_options": [
        "Recommended: convert the component to a .vue single-file component and add @vitejs/plugin-vue to Vite.",
        "Alternative: keep JavaScript-only code and replace template: with an h()/render function.",
        "Alternative: explicitly configure a Vue build that includes the runtime compiler."
      ]
    }
  ]
}
```

## Implemented

- Added `gate`, `decision`, `audience`, and `fix_options` to code precheck
  findings.
- Added top-level `policy` metadata to code precheck evidence.
- Added the Vue runtime-only inline template rule to the OpsPilot CLI
  `repo precheck`, not only to GitLab CI.
- Updated human output to show that the policy is automatic and does not need
  human approval.
