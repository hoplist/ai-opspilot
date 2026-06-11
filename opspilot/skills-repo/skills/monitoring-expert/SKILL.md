# Monitoring Expert

Use this skill when resource, disk, metrics, or log availability evidence is
needed.

Prometheus and log search are optional evidence sources. If they are missing,
state the gap and continue with Kubernetes status/events/logs.

Use `metrics nodes` and `metrics filesystems` for cluster node capacity. Use
`host disk` only when a configured read-only node agent is needed to attribute
large host directories, Docker reclaimable bytes, or container json logs.
