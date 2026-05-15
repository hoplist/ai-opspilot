# OpsPilot RBAC

Default RBAC must remain read-only.

Required Kubernetes resources:

- `pods`
- `pods/log`
- `events`
- `namespaces`
- `services`
- `configmaps`
- `nodes`
- `deployments`
- `statefulsets`
- `daemonsets`
- `replicasets`

Do not grant mutation verbs by default.
