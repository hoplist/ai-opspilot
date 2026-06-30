# 2026-06-30 Inner-Network GitLab And Private Image Baseline

## Goal

Prepare a small, clean baseline for synchronizing OpsPilot into an inner-network
GitLab and private registry.

## Current Required GitLab Repositories

After repository cleanup and compatibility removal, node206 GitLab contains only:

- `tpo/deploy/gitops-manifests`
- `tpo/ops/backups/node200-etcd-snapshots`
- `tpo/platform/opspilot/opspilot-config`
- `tpo/platform/opspilot/opspilot-core`
- `tpo/platform/opspilot/opspilot-skills`

For a new inner-network install, the minimum recommended sync set is:

- `tpo/platform/opspilot/opspilot-core`
- `tpo/platform/opspilot/opspilot-config`
- `tpo/platform/opspilot/opspilot-skills`
- `tpo/deploy/gitops-manifests`

## Private Image Push

Current live OpsPilot core image:

```text
192.168.48.206:5050/tpo/platform/opspilot/opspilot-core/opspilot-core:8553f0ba
```

Retagged and pushed on node206:

```text
docker-hub.tpo.xzoa.com/opspilot/opspilot-core:8553f0ba
```

Verification:

```text
docker pull docker-hub.tpo.xzoa.com/opspilot/opspilot-core:8553f0ba
digest: sha256:3a6b75f18b820034c9b667cf0a7cfd4117537de2897c5a7b8947c6a58bf2d554
```

## Documentation

Added:

```text
docs/inner-network-gitlab-migration.md
```

The document covers:

- which GitLab projects to create and sync;
- which backup repository is optional for a new inner-network cluster;
- which GitLab URLs must be changed in config and GitOps;
- how to replace the OpsPilot runtime image with the private-registry tag;
- which credentials must be regenerated in the inner network;
- first-start validation and rollback.

## Boundary

No runtime Kubernetes deployment was changed in node200. The private-registry
image is prepared for inner-network use; the live test cluster still runs the
standard GitLab Registry image until its GitOps desired state is intentionally
changed.
