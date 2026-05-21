# 2026-05-21 Release GitLab GitOps Evidence

## Change

Extended `opspilot release status` with optional read-only GitLab evidence:

- Latest GitLab pipeline for the mapped project.
- GitLab Container Registry tag existence for the active image tag.
- GitOps desired image from the configured deployment manifest.

## Configuration

`OPSPILOT_RELEASE_SERVICES` now supports:

- `container:<container-name>`
- `gitlab:<project-path-or-id>`
- `gitops:<manifest-file-path>`

Global release datasource settings:

- `OPSPILOT_GITLAB_URL`
- `OPSPILOT_GITLAB_TOKEN`
- `OPSPILOT_GITOPS_PROJECT`
- `OPSPILOT_GITOPS_REF`

The deployment mounts optional Secret `opspilot-release-secrets`, so the token
can be added without putting it in the ConfigMap.

## Impact

When the token or mappings are missing, release status keeps returning
Kubernetes, Argo CD, metric, and log evidence and reports the GitLab/Registry/
GitOps portions as explicit evidence gaps.
