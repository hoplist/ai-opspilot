# OpsPilot retention and disaster recovery

## Runtime Retention

Current local stores:

| Store | Default path | Retention |
| --- | --- | --- |
| audit | `/var/lib/opspilot/audit/audit.jsonl` | 7 days or 32 MiB |
| evidence packs | `/var/lib/opspilot/evidence-packs` | 3 days, 200 files, or about 96 MiB |
| error events | `/var/lib/opspilot/error-events` | 3 days, 100 files, or 32 MiB |

Kubernetes `emptyDir.sizeLimit` protects node disk from unlimited growth:

| Volume | Size limit |
| --- | --- |
| audit | 64 MiB |
| evidence-packs | 128 MiB |
| error-events | 64 MiB |

The size limit is a node-disk guardrail, not a business retention policy.
Application logic must still clean old files before the limit is reached.

## Backup Sources

| Data | Recovery source |
| --- | --- |
| Kubernetes desired state | GitOps repository |
| OpsPilot config | `platform/opspilot-config` GitLab repository |
| Runtime skills | `platform/opspilot-skills` GitLab repository plus image fallback |
| Cluster data | daily etcd backup |
| Container images | GitLab registry or approved private registry |

## Recovery Path

1. Restore cluster control plane from latest etcd backup when needed.
2. Reinstall or recover Argo CD.
3. Point Argo CD to the GitOps repository.
4. Restore OpsPilot deployment and config/skills sync.
5. Verify:
   - `opspilot config status`;
   - `opspilot capabilities`;
   - `opspilot inspect cluster`;
   - release status for `opspilot-core`.

## Data Loss Boundaries

Audit and evidence packs are recent operational evidence. Losing them should
not prevent cluster recovery, service deployment, or future inspections.
