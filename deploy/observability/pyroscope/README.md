# Pyroscope And Alloy eBPF

P2 profiling evidence component.

- `pyroscope` stores profiling data and exposes HTTP on port `4040`.
- `alloy-pyroscope-ebpf` runs as a privileged DaemonSet and sends eBPF profiles to Pyroscope.
- Pyroscope data is stored on `pyroscope-data`, backed by the static hostPath PV `pyroscope-hostpath-pv-xzyc115-19` at `/data/auto-inspection/pyroscope`.

