# OpsPilot architecture baseline

This directory is the current architecture baseline for OpsPilot. Change
records explain when something changed; these documents explain the current
accepted model.

## Documents

- `nfr.md`: non-functional requirements and operating limits.
- `failure-modes.md`: expected failures and minimum operator response.
- `permissions.md`: action risk, auth extension points, and audit boundaries.
- `retention-and-dr.md`: local retention, backups, and recovery paths.
- `argocd-core-migration.md`: live path migration procedure for Argo CD core.
- `adr/`: accepted architecture decisions.

## Current Platform Shape

```text
CLI / Web / AI
-> opspilot-core API
-> service catalog / datasource catalog / skills registry
-> bounded evidence collection
-> audit record and evidence pack
-> read-only result or plan-first controlled action
```

The platform should stay simple for the current internal test phase. Avoid new
operators, heavy middleware, and duplicated observability storage unless the
current file-backed and adapter-backed model stops meeting the NFRs.
