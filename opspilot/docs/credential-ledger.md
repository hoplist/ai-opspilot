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

## Read-Only User Configuration

For people who only need to inspect data, do not give them the application
runtime account. Give them a separate read-only account or a read-only replica.

Recommended modes:

| Scenario | Recommended configuration | Why |
| --- | --- | --- |
| Normal SQL troubleshooting | Dedicated read-only DB account on the same database | Simple and limited to `SELECT` plus metadata reads. |
| Heavy ad hoc queries | Read replica or exported readonly copy | Avoids slow queries affecting the write database. |
| Sync-style tools such as Obsidian | Separate readonly/export database or published file copy | Some clients write local/sync metadata even when the user only wants to read content. |
| Production data access | Temporary readonly account with TTL and audit | Keeps access revocable and visible. |

Minimum MySQL-style privileges for a readonly person:

```sql
CREATE USER 'readonly_xxx'@'%' IDENTIFIED BY '<temporary-password>';
GRANT SELECT, SHOW VIEW ON app_database.* TO 'readonly_xxx'@'%';
```

Do not grant `INSERT`, `UPDATE`, `DELETE`, `CREATE`, `ALTER`, `DROP`, `FILE`,
`SUPER`, or broad `*.*` privileges. If the client needs to write sync/cache
metadata, that is no longer a pure read-only user. Put that metadata in a
separate writable helper database or use a published readonly copy.

For OpsPilot-managed temporary access, the expected user instruction is:

```text
给 <service> 开一个 2 小时只读调试账号
```

OpsPilot should answer with the account scope, expiry, allowed operations, and
revocation status without exposing the password in persistent logs.

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
