# 2026-05-22 Release GitLab Job Logs

## Change

Added read-only GitLab job evidence for release troubleshooting.

New OpsPilot Core APIs:

- `GET /api/release/jobs?service=<service>`
- `GET /api/release/logs?service=<service>&job=<job-name>`
- `GET /api/release/logs?service=<service>&job_id=<gitlab-job-id>`

New CLI commands:

```powershell
opspilot release jobs --service opspilot-core --output human
opspilot release logs --service opspilot-core --job build-image --tail 200 --output human
opspilot release logs --service opspilot-core --job-id 123 --tail 200 --output human
```

## Behavior

- `release jobs` lists jobs from the latest GitLab pipeline for the mapped
  release service.
- `release logs` reads GitLab job trace through the GitLab API.
- Job logs are bounded by `--tail` and `--limit-bytes`; defaults are 200 lines
  and 128 KiB.
- If `--job-id` is omitted, OpsPilot searches the latest pipeline jobs by
  `--job`. If `--job` is also omitted, it returns the first job from the latest
  pipeline.

## Why

Previously `release status` could show that GitLab/BuildKit was failed, but the
operator still had to open GitLab to inspect the job trace. OpsPilot can now
answer where a build failed and show the relevant tail of the BuildKit or CI
log directly.

GitLab remains the source of truth for full job logs, retries, artifacts, and
write operations.
