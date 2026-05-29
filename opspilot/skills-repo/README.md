# OpsPilot runtime skills

This directory is the seed content for the server-side skills repository.

OpsPilot only loads skills from the `skills/` directory. The runtime loader
uses `skill.yaml` as deterministic metadata and treats `SKILL.md` as human/AI
guidance. Skills must map to approved OpsPilot commands and must not define
arbitrary shell execution.

Recommended GitLab target:

```text
platform/opspilot-skills
```

Recommended cluster mount:

```text
emptyDir + skills-sync sidecar -> /opt/opspilot/skills/current
```
