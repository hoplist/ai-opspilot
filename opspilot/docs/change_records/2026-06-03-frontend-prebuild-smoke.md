# 2026-06-03 Frontend prebuild smoke

## Goal

Catch frontend runtime blank-page failures before GitOps deployment. The
fullstack Vue demo exposed a real gap: `npm run build` succeeded, but the
browser rendered a blank page because the app used an inline Vue `template`
with a runtime-only Vue bundle.

## Changes

- Added a frontend-specific blocker to the GitLab `code-precheck` template:
  Vue projects without `@vitejs/plugin-vue` are blocked when source files use
  inline `template:` component definitions.
- Added `prebuild:image-smoke` to the frontend CI template before image push and
  GitOps update.
- The smoke step uses BuildKit to build the final image filesystem without
  pushing it, then verifies:
  - `index.html` exists in the final image filesystem;
  - `index.html` references built JavaScript assets;
  - referenced JavaScript assets exist and are non-empty.
- The step writes AI-readable evidence:

```text
.opspilot/evidence/frontend-image-smoke.json
```

## Flow

```text
developer push
-> preflight:onboarding
-> code-precheck
-> test:frontend
-> prebuild:image-smoke
-> build:image
-> update:gitops
-> Argo CD rollout
```

## Notes

- This keeps the flow automatic. Only high-confidence failures block release.
- The prebuild smoke intentionally does not require Kubernetes or browser
  services. It is a fast image-structure gate before the real image is pushed.
- A later optimization should bake Python into the CI Alpine image because
  `apk add python3` makes `code-precheck` slow on node206.
