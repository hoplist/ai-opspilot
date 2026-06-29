# OpsPilot Offline Kit

This directory is the first offline installation kit layout for an internal
single-node K3s test platform.

The split is intentional:

- K3s and monitoring images are packaged as compressed tar files in this kit.
- OpsPilot and CI images are mirrored to the existing private registry namespace:
  `docker-hub.tpo.xzoa.com/opspilot/<image>:<tag>`.
- GitLab remains a separate host and is not installed by this kit.

## Target Topology

```text
GitLab host
  -> GitLab / Registry / Runner / BuildKit

K3s test host
  -> K3s
  -> Argo CD
  -> Prometheus / node-exporter
  -> OpsPilot
  -> demo services
```

## Directory Layout

```text
offline-kit/
  README.md
  VERSION
  k3s/
    config.yaml
    registries.yaml
    artifacts/.gitkeep
  monitoring/
    artifacts/.gitkeep
  argocd/
    artifacts/.gitkeep
  repos/
    .gitkeep
  packages/
    .gitkeep
  manifests/
    .gitkeep
    gitlab/docker-compose.yml
    node206/
      compose/
      gitops/
  image-lists/
    required.txt
    argocd.txt
    monitoring.txt
    node206-host.txt
    ci.txt
    optional.txt
  scripts/
    prepare-host.sh
    install-node206-services.sh
    install-k3s-airgap.sh
    save-k3s-monitoring-images.sh
    load-monitoring-images.sh
    load-argocd-images.sh
    install-platform-manifests.sh
    retag-push-opspilot-images-206.sh
    export-source-bundles.sh
    package-kit.sh
    verify.sh
```

## Included Artifacts

This first kit already includes these K3s airgap artifacts under
`offline-kit/k3s/artifacts/`:

```text
k3s
install.sh
k3s-airgap-images-amd64.tar.zst
```

It also includes the monitoring image archive under
`offline-kit/monitoring/artifacts/`:

```text
monitoring-images.tar.gz
```

The first version keeps monitoring as one tar package so the internal K3s host
can start Prometheus/node-exporter without pulling from the internet.

Argo CD runtime images are also stored locally and are not pushed to the
private registry:

```text
offline-kit/argocd/artifacts/argocd-images.tar.gz
```

Included Argo CD images:

```text
quay.io/argoproj/argocd:v3.3.8
ghcr.io/dexidp/dex:v2.43.0
public.ecr.aws/docker/library/redis:8.2.3-alpine
```

Source repositories are exported under `offline-kit/repos/` as Git bundle files:

```text
platform-opspilot.bundle
platform-gitops-manifests.bundle
platform-opspilot-config.bundle
platform-opspilot-skills.bundle
```

Known gap in this first kit:

```text
platform-ci-templates.bundle
```

`platform/ci-templates.git` was not exported because the repository was not
found or the current GitLab credential cannot read it. This does not block the
first internal K3s bring-up, but it should be fixed before testing the full
new-service onboarding flow offline.

Node206 bootstrap manifests are copied as a curated set into:

```text
offline-kit/manifests/node206/compose
offline-kit/manifests/node206/host-config
offline-kit/manifests/node206/gitops
```

This is intentionally not a raw `/opt` snapshot. The first kit keeps only the
GitLab/GitLab Runner compose files, node-exporter/Prometheus compose files,
cadvisor compose, OpsPilot agent compose, Prometheus config, Argo CD
bootstrap/core manifests, OpsPilot core manifests, OpsPilot RBAC, and the
Prometheus service alias. Temporary demos, observability experiments, database
backup labs, `.git` directories, and `secret.yaml` files are excluded.

On the node206 GitLab/Runner host, install the Docker Compose services with:

```bash
cd offline-kit
sudo bash scripts/install-node206-services.sh
```

`opspilot-agent` requires `OPSPILOT_AGENT_TOKEN` in
`/opt/opspilot-agent/.env`. GitLab Runner still needs a real runner token before
it can pick up CI jobs.

## Registry Policy

OpsPilot and CI images should be pushed to:

```text
docker-hub.tpo.xzoa.com/opspilot/
```

Examples:

```text
docker-hub.tpo.xzoa.com/opspilot/opspilot-core:d5e84c50
docker-hub.tpo.xzoa.com/opspilot/opspilot-agent:main
docker-hub.tpo.xzoa.com/opspilot/ci-alpine:3.20
docker-hub.tpo.xzoa.com/opspilot/ci-buildkit:rootless
docker-hub.tpo.xzoa.com/opspilot/ci-golang:1.23-alpine
docker-hub.tpo.xzoa.com/opspilot/ci-node:20-alpine
docker-hub.tpo.xzoa.com/opspilot/ci-python:3.12-alpine
docker-hub.tpo.xzoa.com/opspilot/ci-maven:3.9.9-jdk21-alpine
docker-hub.tpo.xzoa.com/opspilot/gitlab-ce:18.11.3
```

Run on node206:

```bash
cd offline-kit
bash scripts/retag-push-opspilot-images-206.sh
```

Create the final copyable package:

```bash
cd offline-kit
bash scripts/package-kit.sh
```

## Internal Install Order

On the K3s test host:

```bash
cd offline-kit
sudo bash scripts/prepare-host.sh
sudo bash scripts/install-k3s-airgap.sh
sudo bash scripts/load-monitoring-images.sh
sudo bash scripts/load-argocd-images.sh
sudo bash scripts/install-platform-manifests.sh
bash scripts/verify.sh
```

`install-platform-manifests.sh` installs the curated bootstrap manifests in this
order:

1. Argo CD CRDs;
2. Argo CD core control plane;
3. OpsPilot namespace and RBAC;
4. OpsPilot Prometheus service alias;
5. OpsPilot core.

Before running it, make sure any required GitLab config and skills repositories
are reachable from the K3s host. If those repositories are private, create the
corresponding `opspilot-config-secrets` and `opspilot-skills-secrets` in the
`opspilot` namespace first. The script intentionally fails visibly if
git-sync, image pulls, or rollout health fail.

## Notes

- This kit is intentionally minimal. ES/Kibana, APISIX, Parca, OpenObserve,
  Kafka exporter, and JumpServer integrations are optional follow-up adapters.
- `docker-hub.tpo.xzoa.com/opspilot/*` is used here because this is an explicit
  internal offline/test installation exception.
- Do not put secrets into this directory. Use Kubernetes Secrets or GitLab
  protected variables after the internal environment is running.
