# 2026-05-29 Server-side runtime skills

## Goal

Move OpsPilot skills from client-local files toward a server-side runtime
registry. Users and thin clients should only talk to OpsPilot. OpsPilot loads
approved skills from a GitLab-backed repository, maps them to safe OpsPilot
commands, and keeps the embedded registry as fallback.

## Decisions

- GitLab is the source of truth for runtime skills.
- OpsPilot reads skills from `OPSPILOT_SKILLS_DIR`, defaulting to
  `/opt/opspilot/skills/current`.
- The cluster deployment uses an `emptyDir` volume and a `skills-sync` sidecar.
- The sidecar uses the same OpsPilot image, so node200 pulls from the node206
  GitLab Registry after the standard release flow updates GitOps.
- The sidecar switches `/opt/opspilot/skills/current` through a relative
  symlink so both containers can read the same release directory even though
  they mount the shared volume at different paths.
- `hostPath` is not used for skills because it binds runtime state to one node.
- PVC can be added later if the skills repository becomes large or needs a
  persistent cache.
- Skills remain metadata and guidance. They cannot define arbitrary shell
  execution; they map to approved OpsPilot commands and evidence sources.

## Runtime behavior

- `GET /api/skills/registry` now loads dynamic skills when available.
- If dynamic skills are missing or invalid, OpsPilot returns the embedded
  registry and includes warnings.
- `GET /api/capabilities` reports the active skills source, source path,
  source version, dynamic count, and any sync/load warnings.
- The CLI `opspilot skills registry` displays the active source and dynamic
  count when talking to the backend.

## Seeded skills

Only OpsPilot-used skills are seeded:

- `opspilot-ops`
- `auto-inspection-rca`
- `kubernetes-specialist`
- `monitoring-expert`
- `devops-engineer`
- `debugging-wizard`

Seed path in this repo:

```text
opspilot/skills-repo/skills
```

Recommended GitLab repository:

```text
platform/opspilot-skills
```

## Mount and update model

Recommended first version:

```text
GitLab skills repo -> skills-sync sidecar -> emptyDir -> /opt/opspilot/skills/current
```

`emptyDir` is enough because GitLab is the source of truth and the mounted
directory is only a local cache. If the Pod restarts, the sidecar pulls the
skills again.

For larger or slower environments, use a PVC as the same mount target. The
OpsPilot code does not care whether the directory comes from `emptyDir`, PVC,
or hostPath. The operational recommendation is:

- `emptyDir`: default and simplest.
- `PVC`: use for cache persistence or large repos.
- `hostPath`: only for single-node manual testing.

## Secret requirement

If `platform/opspilot-skills` is private, create this Secret out of band:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: opspilot-skills-secrets
  namespace: opspilot
type: Opaque
stringData:
  OPSPILOT_SKILLS_GIT_USERNAME: <deploy token username or oauth2>
  OPSPILOT_SKILLS_GIT_PASSWORD: <read-only GitLab token>
```

The Secret is optional. Without it, the sidecar can only pull public/internal
repositories that allow unauthenticated HTTP clone. OpsPilot still falls back
to embedded skills if sync fails.

## Validation

Run:

```powershell
go test ./opspilot/...
go run ./opspilot/cli skills registry --local --output human
```
