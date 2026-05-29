# 2026-05-29 git-sync runtime skills sidecar

## Goal

Replace the hand-written runtime skills sync script with the Kubernetes
`git-sync` sidecar pattern. OpsPilot still loads skills from a mounted
directory, but cloning, fetching, and symlink switching are delegated to
git-sync.

## Decision

- Use git-sync v4 style arguments for the runtime skills sidecar.
- Mirror the official image into the node206 GitLab Registry for node200
  deployments:

```text
registry.k8s.io/git-sync/git-sync:v4.6.0
-> 192.168.48.206:5050/platform/opspilot/git-sync:v4.6.0
```

- Keep the sidecar name `skills-sync`, but change the image to the mirrored
  git-sync image.
- Keep the shared volume as `emptyDir`.
- Set Pod `fsGroup: 65533` because the official git-sync image runs as
  `65533:65533` and must write the shared `emptyDir`.
- Set `OPSPILOT_SKILLS_DIR` to:

```text
/opt/opspilot/skills/current/skills
```

git-sync points `/skills/current` to the latest synced worktree; the skills repo
keeps actual skill definitions under the `skills/` subdirectory.

## Behavior

- git-sync retries forever with `--max-failures=-1`.
- OpsPilot keeps the embedded skills registry as fallback if the dynamic skills
  directory is empty or unavailable.
- The OpsPilot image no longer installs `git` or `openssh-client` just to sync
  skills.
- The GitOps update step only rewrites the `core` container image. The
  `skills-sync` image remains pinned to git-sync.

## Loader compatibility

The dynamic skills loader now resolves symlinked roots and can derive the source
version from a git-sync style path such as:

```text
/opt/opspilot/skills/current/skills -> /opt/opspilot/skills/root/<hash>/skills
```

## Validation

Run:

```powershell
go test ./opspilot/...
kubectl kustomize deploy/opspilot/core
opspilot skills registry --integrated-only --output human
```
