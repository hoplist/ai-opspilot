# 2026-06-09 skills mirror repository

## Goal

Make OpsPilot skills work like an internal mirror repository: external skill
packs can be inventoried and classified in GitLab, while OpsPilot only loads
approved runtime skills from `skills/`.

## Changed

- Added mirror metadata support in OpsPilot:
  - `/api/skills/sources`
  - `/api/skills/candidates`
  - `opspilot skills sources`
  - `opspilot skills candidates`
- Added `skillregistry.MirrorWithSkillsDir` to read mirror metadata next to the
  synced `skills/` directory.
- Added GitLab skills repository structure:
  - `registry.yaml`
  - `import-rules.yaml`
  - `upstream/garrytan-gstack/inventory.yaml`
  - `candidates/*/candidate.yaml`
- Kept runtime loading limited to `skills/`; `candidates/` and `upstream/` are
  inventory and review material only.

## Decision

`garrytan/gstack` is mirrored as a source of workflow ideas. Only adapted
runtime skills are enabled. Client-only skills such as browser, iOS, and
pair-agent flows are marked unsupported until OpsPilot has matching backend
capabilities.

## Validation Plan

- Validate GitLab runtime skills with `opspilot skills validate`.
- Verify `opspilot skills sources` shows mirror source counts.
- Verify `opspilot skills candidates` shows candidate and unsupported entries.
- Run `go test ./opspilot/...` and `go vet ./opspilot/...`.
