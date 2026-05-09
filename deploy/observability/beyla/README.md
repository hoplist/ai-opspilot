# Beyla

P0 deep observability component for zero-code application telemetry.

- Runs as a privileged DaemonSet with `hostPID: true`.
- Exports OTLP metrics and traces to `otel-collector:4318`.
- Excludes system and observability namespaces to avoid noisy self-instrumentation.
- Does not mutate workloads.
