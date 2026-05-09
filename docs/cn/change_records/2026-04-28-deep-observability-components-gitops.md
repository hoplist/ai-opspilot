# 2026-04-28 深层排查组件 GitOps 部署

## 背景

- 当前阶段不建设 P3 网络证据，不接入 Hubble、Tetragon、Calico/CoreDNS 网络排查。
- 按 P0/P1/P2 顺序部署深层排查组件，并通过 `platform/gitops-manifests` 交给 Argo CD 自动同步。

## 本次部署范围

P0：OTel eBPF / Beyla

- 新增 `beyla` DaemonSet。
- 通过 eBPF 无侵入采集 HTTP/gRPC/DB 调用、RED 指标、服务依赖与 trace span。
- OTLP HTTP 输出到现有 `otel-collector.observability.svc.cluster.local:4318`。
- 排除 `observability`、`argocd`、`kube-system` 等系统命名空间，避免自采集和噪声。

P1：Falco runtime 事件

- 新增 `falco` DaemonSet。
- 使用 `modern_ebpf` engine。
- 输出 JSON 到 stdout，由现有日志采集链路进入 OpenSearch。
- 仅作为 runtime 证据源，不启用自动阻断或 enforcement。

P2：Pyroscope / Alloy eBPF profiling

- 新增 `pyroscope` Deployment 与 NodePort Service。
- 新增 `alloy-pyroscope-ebpf` DaemonSet。
- Alloy 使用 `pyroscope.ebpf` 采集本机 Pod profile，并写入 `pyroscope`。
- Pyroscope 数据使用 `pyroscope-data` PVC，后端为静态 NFS PV `pyroscope-nfs-pv-206`。

## GitOps 文件

- `clusters/test/observability/beyla/*`
- `clusters/test/observability/falco/*`
- `clusters/test/observability/pyroscope/*`
- `source/deploy/observability/beyla/*`
- `source/deploy/observability/falco/*`
- `source/deploy/observability/pyroscope/*`
- `source/yaml/observability/beyla/*`
- `source/yaml/observability/falco/*`
- `source/yaml/observability/pyroscope/*`

## Pyroscope 持久化

- 新增 PV：`pyroscope-nfs-pv-206`
- 新增 PVC：`observability/pyroscope-data`
- 容量：`20Gi`
- 访问模式：`ReadWriteMany`
- 回收策略：`Retain`
- NFS 路径：`192.168.48.206:/srv/nfs/observability/pyroscope`
- 挂载路径：`/var/lib/pyroscope`

## 安全边界

- 所有组件只采集证据，不执行修复动作。
- Beyla、Falco、Alloy 需要 eBPF/hostPID/privileged 权限，部署范围限定在 observability GitOps 管理路径。
- Falco 不启用自动响应或阻断。
- MCP 后续只通过 backend 只读 API 读取摘要，不直接访问节点或运行主机命令。

## 验证记录

- `kubectl kustomize clusters/test/observability` 通过。
- `kubectl apply --dry-run=client -k clusters/test/observability` 通过。
- `kubectl apply --dry-run=server -k clusters/test/observability` 通过。
- GitOps 已提交并推送到 `platform/gitops-manifests`：
  - `79f645a add deep observability ebpf components`
- `3baa24e fix deep observability component startup`
- `c1cdb49 use reachable image proxy for observability components`
- `a565bb4 persist pyroscope profiles on nfs pvc`
- Argo CD `observability` Application 已同步：
  - sync：`Synced`
  - health：`Healthy`
  - revision：`c1cdb49c33a6e4f00ebce8b996620392a066be5b`
- Rollout 结果：
  - `daemonset/beyla` 成功。
  - `daemonset/falco` 成功。
  - `daemonset/alloy-pyroscope-ebpf` 成功。
  - `deployment/pyroscope` 成功。
- 服务健康：
  - `pyroscope.observability.svc.cluster.local:4040/ready` 返回 `200 ready`。
  - `otel-collector.observability.svc.cluster.local:13133/` 返回 `200`。
- 运行日志：
  - Beyla 已开始 instrumenting process。
  - Falco 已加载 `modern BPF probe` 与 container plugin，并加载默认规则。
- Alloy 已加载 `pyroscope.ebpf`，日志显示 `eBPF tracer loaded` 与 `Attached tracer program`。
- `pyroscope-data` PVC 已绑定：
  - PV：`pyroscope-nfs-pv-206`
  - PVC：`observability/pyroscope-data`
  - 状态：`Bound`
- Pyroscope 已切换到 PVC 持久化后重新 rollout 成功。
- Argo CD `observability` Application 持久化变更后状态：
  - sync：`Synced`
  - health：`Healthy`
  - revision：`a565bb408cbc7def84201fe5ac0ae464b3f51608`
- `pyroscope.observability.svc.cluster.local:4040/ready` 返回 `200 ready`。

## 部署修正记录

- 初次部署时 `docker.1ms.run` 镜像代理在部分节点出现 layer not found；直接使用 Docker Hub 又被集群 containerd 重写到不可用的内部 mirror `10.236.188.138:5001`。
- 最终保留 `docker.1ms.run` 作为当前可用镜像代理，并依赖节点重试完成拉取。
- Falco 首次启动失败是因为 container plugin 配置缺少 `bpm` engine；已补齐 `bpm`、`libvirt_lxc`、`lxc` 等 engine 字段。
- Beyla 已补充 `/.cache` 与 `/sys/fs/bpf` 挂载，减少 Java 注入缓存与 pinned map 相关警告。
- Pyroscope PVC 首次挂载失败是因为 NFS 服务端路径不存在；已在 `192.168.48.206` 创建 `/srv/nfs/observability/pyroscope` 并设置写权限。
