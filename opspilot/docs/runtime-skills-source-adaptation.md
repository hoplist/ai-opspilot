# OpsPilot Runtime Skills Source Adaptation

## Goal

Use public Coding Agent skill packs as reference material while keeping OpsPilot
skills server-side, deterministic, GitLab-managed, and callable by OpsPilot
without requiring client-local skill installation.

This design is based on the direction from the Linux.do skill-pack discussion:
skills should be selected by scenario, kept lightweight at the entry point, and
expanded through references/examples only when needed. OpsPilot should not copy
every external skill into production blindly.

Reference: https://linux.do/t/topic/1802808

## Non-Goals

- Do not require users or CLI clients to install local gstack, Codex, Claude, or
  other client-side skill packs.
- Do not let GitLab skills define arbitrary shell execution.
- Do not import every recommended public skill repository as-is.
- Do not make missing external skills block Kubernetes, release, or log
  troubleshooting.

## Current Model

```text
User / CLI / AI
  -> OpsPilot API
  -> skills registry loader
  -> /opt/opspilot/skills/current/skills
  -> skill.yaml routing metadata
  -> SKILL.md AI guidance
  -> approved OpsPilot command/API execution
```

The source of truth is the GitLab repository:

```text
tpo/platform/opspilot/opspilot-skills
```

The cluster runtime syncs this repository into the OpsPilot Pod by git-sync:

```text
/opt/opspilot/skills/current/skills
```

## External Skill Sources

External repositories are treated as a catalog of ideas, not as direct runtime
dependencies.

| Source Type | Example | OpsPilot Treatment |
| --- | --- | --- |
| Workflow packs | `garrytan/gstack`, `SuperClaude Framework` | Translate into server-side runtime workflows such as review, ship, investigate, and devex review. |
| Vendor skill packs | `anthropics/skills`, `vercel-labs/agent-skills`, `MiniMax-AI/skills` | Extract patterns, examples, and prompt structure only after review. |
| Planning packs | `planning-with-files` | Convert to OpsPilot planning or fix dry-run guidance. |
| Aggregators | `VoltAgent/awesome-agent-skills` | Use as discovery index only. Nothing is imported automatically. |
| Integration packs | `ComposioHQ/skills` | Integrate only if the target integration is already approved and credentialed in OpsPilot. |

## Skill Package Shape

Each OpsPilot runtime skill should use progressive disclosure:

```text
skills/<name>/
  skill.yaml          # small routing and capability metadata
  SKILL.md            # execution guidance, boundaries, decision rules
  references/         # optional deeper playbooks
  examples/           # optional input/output examples
  tests/              # optional validation fixtures
```

`skill.yaml` stays small. It should answer:

- What is the skill called?
- When should OpsPilot route to it?
- What evidence should be collected?
- Which approved OpsPilot commands can be used?
- What boundaries apply?

`SKILL.md` carries the real expert procedure:

- evidence collection order
- blocker/warning rules
- missing-evidence wording
- safe repair or rollback boundaries
- output format

`examples/` should be added for high-value skills so AI can learn stable
response patterns without relying on long entry files.

## Import Policy

External skills must pass a manual adaptation step before entering GitLab.

```text
discover -> classify -> adapt -> review -> publish -> sync -> verify
```

1. Discover from approved source repositories or posts.
2. Classify into OpsPilot categories:
   `release`, `rca`, `security`, `devex`, `code-quality`, `database`,
   `kubernetes`, `monitoring`, or `planning`.
3. Adapt to OpsPilot runtime commands.
4. Review for safety and usefulness.
5. Publish to `tpo/platform/opspilot/opspilot-skills`.
6. Let git-sync update the cluster copy.
7. Verify with OpsPilot `capabilities`, skill registry evidence, and one
   natural-language routing test.

## Safety Rules

Runtime skills may:

- route natural language to existing OpsPilot capabilities
- request read-only Kubernetes, GitLab, GitOps, Argo CD, metrics, and log
  evidence
- generate dry-run fix plans
- explain missing evidence
- recommend controlled next actions

Runtime skills may not:

- embed raw credentials
- run arbitrary shell commands
- download code from the internet at request time
- mutate cluster state without an approved OpsPilot action path
- bypass GitLab/GitOps release controls
- declare a release complete without rollout evidence

## Online and Offline Behavior

OpsPilot does not need public internet access to call skills.

It needs only internal GitLab access when syncing or updating skills:

```text
OpsPilot Pod -> node206 GitLab -> tpo/platform/opspilot/opspilot-skills.git
```

Runtime calls read the local synced directory. If GitLab is temporarily
unavailable, already-synced skills continue to work until the Pod is recreated.

Recommended next hardening:

```text
GitLab skills source
  -> git-sync runtime copy
  -> image-bundled fallback skills
  -> optional persistent cache
```

## gstack Adaptation

`garrytan/gstack` is not used as a client-side dependency for OpsPilot users.
Its workflow concepts are translated into OpsPilot runtime skills:

| gstack Concept | OpsPilot Runtime Skill | Purpose |
| --- | --- | --- |
| review | `gstack-review` | Pre-landing code, CI, Dockerfile, GitOps, and preflight review. |
| cso | `gstack-cso` | Security, credentials, RBAC, container hardening, and mutation boundary review. |
| devex-review | `gstack-devex-review` | Beginner workflow and natural-language usability review. |
| ship | `gstack-ship` | Standard release evidence gate. |
| investigate | `gstack-investigate` | Read-only RCA and failed-release diagnosis. |

Every OpsPilot code submission should pass this sequence before release:

```text
gstack-review
  -> gstack-cso
  -> gstack-devex-review
  -> tests / vet / preflight
  -> GitLab Runner
  -> BuildKit
  -> Registry
  -> GitOps
  -> Argo CD
  -> rollout verification
```

## Scoring New Skills

New external skill candidates should be scored before adaptation.

| Score Area | Question |
| --- | --- |
| Relevance | Does this improve OpsPilot release, RCA, code review, security, or beginner usage? |
| Executability | Can it map to approved OpsPilot commands instead of arbitrary shell? |
| Safety | Does it avoid leaking secrets and uncontrolled mutations? |
| Evidence Value | Does it improve facts, examples, or decision rules? |
| Maintenance | Can it stay small and understandable in GitLab? |

Suggested acceptance:

- 4-5 points: adapt now
- 3 points: keep as candidate
- 1-2 points: do not import

## Rollout Plan

### Phase 1: Current Foundation

- GitLab-backed runtime skills repository.
- git-sync sidecar to sync skills into OpsPilot.
- Initial gstack runtime skills.
- `skills_registry` capability evidence.

### Phase 2: Examples and Decision Trees

- Add `examples/` for the highest-value skills:
  `opspilot-ops`, `auto-inspection-rca`, `gstack-ship`,
  `gstack-investigate`, `secure-code-guardian`, and `code-reviewer`.
- Add failed-release, CrashLoop, ImagePullBackOff, missing ELK, missing APISIX,
  and GitOps drift examples.
- Add expected output shape for AI-readable evidence packs.

### Phase 3: Skill Import Tooling

- Add a small `skills import-plan` command that reads a candidate source and
  generates a review-only adaptation proposal.
- Add validation for `skill.yaml` fields, category, commands, and boundaries.
- Add a registry report showing integrated, candidate, rejected, and stale
  skills.

### Phase 4: Runtime Fallback

- Bundle a minimal fallback skills set inside the OpsPilot image.
- Prefer GitLab synced skills when available.
- Fall back to bundled skills when GitLab sync is unavailable after Pod
  recreation.

## Required Change Control

Each skill adaptation should record:

- source repository or URL
- adapted skill name
- reason for import
- approved command mappings
- safety boundaries
- examples added or still missing
- release verification evidence

This keeps the skills repository understandable and prevents it from turning
into an unreviewed pile of prompt fragments.
