# 2026-05-21 Release Status CLI And Core

## Change

Implemented the first read-only release evidence command:

```powershell
opspilot release status --service opspilot-core --output human
```

Core endpoint:

```text
GET /api/release/status?service=opspilot-core
```

## Evidence Included

- Configured service mapping.
- Kubernetes Deployment rollout status.
- Matching Pods via Deployment selector labels.
- Pod metrics from Prometheus when configured.
- Short-window Kubernetes log presence.
- ELK log search presence.
- Explicit evidence gaps for GitLab, Registry, GitOps, and Argo CD until their
  read-only datasources are configured.

## Configuration

```text
OPSPILOT_RELEASE_SERVICES="opspilot-core=namespace:opspilot,deployment:opspilot-core,source:node200-k8s,image:192.168.48.206:5050/platform/opspilot/opspilot-core"
```

## Verification

Local Core validation returned:

```text
Release: opspilot-core
Status: healthy stage=rollout namespace=opspilot deployment=opspilot-core
Kubernetes: ready=1 desired=1 updated=1 available=1
Gaps: gitlab_datasource_missing, registry_token_or_api_missing, gitops_datasource_missing, argocd_datasource_missing
```

## Boundary

This is still read-only. It does not trigger GitLab pipelines, push images,
mutate GitOps, sync Argo CD, or roll back releases.

