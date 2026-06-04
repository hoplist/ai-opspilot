# Credential ledger

## Purpose

Credentials must be understandable without exposing secret values. The ledger
records what each credential is for, where it lives, who owns it, what it can
access, and how it should be rotated.

The ledger is not a secret store. Secret values stay in Kubernetes Secrets,
GitLab CI/CD variables, node/containerd config, or a future external secret
manager.

## Credential Classes

| Class | Example | Storage |
| --- | --- | --- |
| `platform-runtime` | `opspilot-release-secrets` | Kubernetes Secret |
| `skills-runtime` | `opspilot-skills-secrets` | Kubernetes Secret |
| `ci-gitops` | `GITOPS_TOKEN` | GitLab CI/CD variable |
| `image-pull` | `gitlab-registry-pull` | Kubernetes Secret or node/containerd config |
| `app-runtime` | `skillshub-api-mysql-conn` | Kubernetes Secret |
| `debug-temporary` | `debug_skillshub_20260604` | Database/user system plus ledger event |
| `observability-datasource` | `prod-elk` credentials | Kubernetes Secret |
| `cluster-access` | `opspilot-cluster-prod-a` | Kubernetes Secret or external secret manager |

## Current Credentials

| Name | Class | Location | Used by | Notes |
| --- | --- | --- | --- | --- |
| `opspilot-release-secrets` | `platform-runtime` | `opspilot` namespace Secret | `opspilot-core` | Contains `OPSPILOT_GITLAB_TOKEN`. |
| `opspilot-skills-secrets` | `skills-runtime` | `opspilot` namespace Secret | `skills-sync` sidecar | Contains Git credentials for `platform/opspilot-skills`. |
| `gitlab-registry-pull` | `image-pull` | `opspilot` and selected app namespaces | Workload image pulls | May be replaced by node/containerd registry auth. |
| `GITOPS_TOKEN` | `ci-gitops` | GitLab CI/CD variables | BuildKit/GitOps pipeline | Writes GitOps desired state. |
| `RUNNER_AUTH_TOKEN` | `ci-runner` | Runner setup only | node206 GitLab Runner registration | Should not be stored in app repos. |

## Ledger Record Template

```yaml
name: skillshub-api-mysql-conn
class: app-runtime
environment: test
scope:
  cluster: node200-test
  namespace: cicd-devex-skillshub
  service: skillshub-api
storage:
  type: kubernetes-secret
  namespace: cicd-devex-skillshub
  name: skillshub-api-mysql-conn
keys:
  - MYSQL_HOST
  - MYSQL_PORT
  - MYSQL_DATABASE
  - MYSQL_USER
  - MYSQL_PASSWORD
permissions:
  mysql:
    database: skillshub
    privileges:
      - select
      - insert
      - update
      - delete
owner:
  team: devex
  platformOwner: opspilot
rotation:
  policy: generated
  interval: 90d
  lastRotated: ""
expiry: ""
audit:
  createdBy: opspilot
  createdAt: ""
  lastUsedAt: ""
blastRadius:
  - skillshub database only
```

## Add Credential Flow

OpsPilot should eventually expose plan-first commands:

```text
opspilot credentials plan app-db --service skillshub-api --type mysql --mode shared
opspilot credentials plan datasource --name prod-elk --type elasticsearch
opspilot credentials plan cluster --name prod-a
```

The first output is a plan:

- what Secret or GitLab variable will be created;
- which service or datasource will use it;
- what permissions it grants;
- rollback and rotation notes;
- whether the credential is long-lived or temporary.

Actual creation should be gated by explicit confirmation and policy.

## Developer Database Debugging

Long-lived service credentials are for workloads, not people. Developers should
get temporary credentials for local debugging:

```text
opspilot db access --service skillshub-api --mode readonly --ttl 2h
```

The temporary account should be:

- limited to the service database;
- read-only by default;
- time-limited;
- audited;
- automatically revoked.

## Observability Datasource Credentials

A datasource credential should be scoped to the datasource:

```yaml
name: prod-elk
class: observability-datasource
storage:
  type: kubernetes-secret
  namespace: opspilot
keys:
  - ELK_PROD_USERNAME
  - ELK_PROD_PASSWORD
usedBy:
  - opspilot-core
permissions:
  - read index apisix-*
  - read index service-logs-*
```

OpsPilot should report missing datasource credentials as evidence gaps instead
of failing the whole Pod or service investigation.
