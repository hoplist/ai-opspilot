# 2026-06-09 gstack security follow-up

## Goal

Record the gstack review findings and apply the test-environment hardening items
that are safe to land now.

## Decisions

- Core API authentication for non-read-only endpoints is deferred for the test
  environment. OpsPilot is currently deployed only on the isolated internal
  network and is not yet a production service. This must be revisited before any
  production or wider-network exposure.
- Auto-generated lightweight middleware credentials are accepted for the test
  environment. They are intended for internal test services only. Production
  onboarding must switch to platform-managed random credentials or external
  Secret references before going live.
- Repo preflight monorepo path handling is in scope now because OpsPilot itself
  has code under `opspilot/`, CI at repository root, and deployment manifests
  under `deploy/opspilot/core`.
- Runtime hardening is in scope now for `opspilot-core`. The Docker node agent
  keeps its Docker socket access model, so the immediate control for that
  surface is mandatory bearer-token configuration rather than forcing a non-root
  image user that could break socket access on node206.
- CLI HTTP timeout hardening is in scope now because a stuck backend should not
  make beginner-facing commands hang forever.
- gstack is integrated as a server-side OpsPilot runtime workflow, not as a
  client-local dependency. Clients should only call OpsPilot; OpsPilot loads
  GitLab-backed skills and routes review, security, devex, release, and
  investigation guidance from the server-side skills repository.
- Every OpsPilot code submission should pass the gstack-inspired sequence before
  standard release: review, security/CSO, developer-experience review, tests,
  preflight, standard pipeline, GitOps update, Argo CD sync, and rollout
  verification.

## Changed

- Added `repo preflight --ci-path`, `--deploy-path`, `--namespace`, and
  per-resource manifest path overrides so platform repositories and monorepos
  can point preflight at their real CI, namespace, guardrail, service account,
  and workload manifest locations without changing the default business-service
  layout.
- Hardened the `opspilot-core` image with a non-root runtime user.
- Hardened the `opspilot-core` Deployment with pod/container security context:
  `runAsNonRoot`, `RuntimeDefault` seccomp, no privilege escalation, dropped
  Linux capabilities, read-only root filesystem, and explicit user/group IDs.
- Added `LimitRange` and `ResourceQuota` for the `opspilot` namespace and added
  an `emptyDir.sizeLimit` for the runtime skills volume.
- Kept `skills-sync` on a read-only root filesystem and added a bounded
  `/tmp` `emptyDir` because git-sync creates a temporary gitconfig file at
  startup.
- Added node-agent startup validation: when listening on a non-local host,
  `OPSPILOT_AGENT_TOKEN` must be configured. Localhost-only development remains
  token-optional.
- Added `OPSPILOT_NODE_AGENT_TOKENS` support in `opspilot-core` so calls to
  token-protected node agents send a Bearer token. The value should come from
  Kubernetes Secret env, not ConfigMap/GitOps plaintext.
- Documented the node-agent token requirement in `agent/README.md`.
- Replaced CLI `http.Get` / `http.DefaultClient.Do` usage with a shared
  `http.Client{Timeout: 30s}` and per-request context so backend stalls return
  errors instead of hanging indefinitely.
- Added GitLab-backed runtime skills for the gstack workflow in the
  `platform/opspilot-skills` repository: `gstack-review`, `gstack-cso`,
  `gstack-devex-review`, `gstack-ship`, and `gstack-investigate`. These are
  deterministic OpsPilot-callable skills that map gstack concepts to approved
  OpsPilot commands and evidence rather than requiring client-local gstack
  slash commands.

## Deferred To Production Readiness

- Add Core API auth/RBAC for `release trigger`, `release rollback`,
  `quality run`, and future mutating endpoints.
- Replace deterministic test middleware credentials with random
  platform-managed credentials or external Secret references.
- Add a deployment-specific node-agent hardening plan that preserves Docker
  socket access while reducing runtime privilege.

## Validation

Executed:

```text
go test ./opspilot/...
go vet ./opspilot/...
go run ./opspilot/cli --output human repo preflight --repo opspilot --project platform/opspilot --ci-path ../.gitlab-ci.yml --deploy-path ../deploy/opspilot/core --namespace opspilot --namespace-path ../deploy/opspilot/rbac/namespace.yaml --limitrange-path ../deploy/opspilot/rbac/limitrange.yaml --resourcequota-path ../deploy/opspilot/rbac/resourcequota.yaml --serviceaccount-path ../deploy/opspilot/rbac/serviceaccount.yaml
kubectl kustomize deploy/opspilot/rbac
kubectl kustomize deploy/opspilot/core
```

The OpsPilot self-preflight is now `ready=true`. It still reports non-blocking
warnings for direct BuildKit CI and missing optional quality config, which are
accepted for the current platform repository shape.
