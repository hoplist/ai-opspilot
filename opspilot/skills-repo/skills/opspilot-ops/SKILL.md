# OpsPilot Ops

Use this skill as the default entry for OpsPilot investigations.

Route questions through approved OpsPilot commands first:

- `doctor`
- `capabilities`
- `check cluster`
- `check pod`
- `check service`
- `release status`

Do not bypass OpsPilot with direct cluster mutation. If evidence is missing,
state the gap and continue with available Kubernetes-first evidence.
