# Falco

P1 runtime evidence component.

- Runs as a privileged DaemonSet.
- Uses the Falco `modern_ebpf` engine.
- Emits JSON runtime events to stdout for collection by the existing log pipeline.
- Automatic enforcement and response actions are not enabled.
