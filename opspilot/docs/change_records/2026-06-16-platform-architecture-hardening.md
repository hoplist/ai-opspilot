# 2026-06-16 platform architecture hardening

## Goal

Complete the approved hardening work as implementation, not only planning:

1. Move GitLab project create/reuse for `repo upload --confirm` into
   `opspilot-core` so the CLI remains a thin local wrapper.
2. Add a bounded datasource routing layer for multi-cluster and multi-ES log
   investigation. Kibana remains UI metadata; Elasticsearch/OpenSearch remains
   the query target.
3. Add an Argo CD core live-path migration plan with render diff and health
   validation before switching from the compatibility path.
4. Add formal ADR and NFR documents for capacity, failure modes, permission
   model, retention, and disaster recovery.

## Implementation Order

1. Document the architecture baseline and acceptance criteria.
2. Implement the read-only datasource route resolver.
3. Move server-owned GitLab project create/reuse into `opspilot-core`.
4. Add Argo CD render-diff tooling and documentation.
5. Validate locally, then release through the standard GitLab Runner ->
   BuildKit -> Registry -> GitOps -> Argo CD flow.

## Boundaries

- The first `repo upload` hardening phase does not upload local source archives
  to the server. The CLI still runs local precheck and `git push` because only
  the client can read the local repository.
- `opspilot-core` owns GitLab project create/reuse, allowed target policy, and
  audit. The CLI may still need a push token until archive upload or another
  server-side source ingestion mode exists.
- Log routing must be bounded by time window, result limit, and datasource
  ordering. Global search remains explicit instead of automatic.
- Argo CD live path migration must not be performed without render diff and
  health verification.

## Minimum Validation

- `go test ./...`
- `go vet ./...`
- `opspilot config validate --dir ./opspilot/config/opspilot-config`
- `opspilot logs route --host <domain> --output human` or equivalent resolver
  test coverage
- `opspilot repo upload-plan --repo . --name <name>`
- API test for `POST /api/repo/upload-target` with fake GitLab
- Argo CD render diff command must produce an explicit old/new path comparison
- Standard release evidence: GitLab pipeline, GitOps update, Argo CD sync, and
  `opspilot-core` rollout status

## Follow-up

- A fully thin `repo upload` can be added later through archive upload:
  CLI -> source archive -> core -> GitLab Repository Files/Commits API.
- Production clusters and production ES/Kibana endpoints remain opt-in and must
  not be connected without explicit confirmation.
