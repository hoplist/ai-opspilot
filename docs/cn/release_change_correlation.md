# 发布变更数据只读接入说明

本文记录 `auto_inspection` RCA / MCP 对发布变更数据的只读接入方式，用于回答“问题是否由最近发布、镜像版本、Deployment revision、Helm 元数据或 ConfigMap 变化引入”这类问题。

## 1. 目标

- 查询 Pod / Workload 当前运行的发布元数据。
- 查询命名空间内近期 Deployment / StatefulSet / DaemonSet / ReplicaSet / ConfigMap 变化摘要。
- 将 incident 时间窗口与发布变更时间窗口做只读关联，辅助判断“变更后异常”。
- 保持 MCP 只读：不 SSH、不执行服务器命令、不修改 Kubernetes 资源。

## 2. 新增 MCP 工具

| 工具 | 作用 | 常用入参 | 只读边界 |
| --- | --- | --- | --- |
| `release_for_workload` | 查询 Pod 或 Workload 对应的发布元数据 | `namespace`, `pod`, `workload_name`, `workload_kind`, `include_configmaps` | 只读取 Kubernetes metadata/spec/status 摘要 |
| `release_recent_changes` | 查询时间窗口内近期发布/配置变化 | `namespace`, `range_hours`, `service`, `workload_name`, `limit` | 只读取 workload 与 ConfigMap 元数据 |
| `correlate_change_with_incident` | 将异常时间窗口与发布变化做关联 | `namespace`, `pod`, `workload_name`, `range_hours`, `limit` | 只返回证据与候选关联，不执行修复 |

## 3. 后端 API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/releases/workload` | 查询单个 Pod / Workload 的镜像、revision、Helm 标注、ConfigMap 引用 |
| `GET` | `/api/releases/recent-changes` | 查询命名空间内近期 workload / ConfigMap 变化摘要 |
| `GET` | `/api/releases/correlate` | 查询 incident 时间窗口附近是否存在发布变更候选 |

## 4. 读取范围

允许读取：

- `deployments.apps`
- `statefulsets.apps`
- `daemonsets.apps`
- `replicasets.apps`
- `configmaps`
- Pod ownerReferences 与 workload selector
- workload 容器镜像、imagePullPolicy、replicas、readyReplicas、resourceVersion、generation
- Helm 常见 label / annotation：`meta.helm.sh/release-name`、`meta.helm.sh/release-namespace`、`helm.sh/chart`、`app.kubernetes.io/managed-by`
- Deployment revision 与变更注解：`deployment.kubernetes.io/revision`、`kubectl.kubernetes.io/restartedAt`、`kubernetes.io/change-cause`

不允许读取或操作：

- 不读取 Kubernetes Secret 内容。
- 不解析 Helm release Secret 正文。
- 不执行 `helm history`、`kubectl rollout`、`kubectl patch`、`kubectl apply`、`kubectl delete` 等变更命令。
- 不通过 MCP SSH 到服务器。

## 5. 使用示例

查询 Pod 当前运行版本：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py release-workload --namespace observability --pod opensearch-0
```

查询最近 24 小时发布变更：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py release-changes --namespace observability --range-hours 24 --limit 10
```

关联某个 Pod 的异常窗口与发布变更：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py release-correlate --namespace observability --pod opensearch-0 --range-hours 24 --limit 10
```

## 6. 输出字段

典型输出会包含：

- `mode`: 只读模式，例如 `read_only_release_for_workload`
- `safety`: MCP 安全边界，固定标明 `server_commands=not_allowed`、`kubernetes_mutations=not_allowed`
- `workload`: workload 类型、名称、镜像、replicas、revision、Helm 元数据
- `configmaps`: 关联 ConfigMap 的名称、创建时间、resourceVersion、data keys
- `items`: 最近变化列表
- `release`: 当前 Pod / Workload 解析出的发布元数据
- `recent_changes`: incident 时间窗口附近的发布变更候选
- `assessment`: 只读关联评估摘要，包含是否解析到 workload、候选变化数量和限制说明
- `errors`: 只读查询期间遇到的非致命错误

## 7. 已知限制

- Kubernetes workload metadata 只能反映当前对象与保留的 ReplicaSet 信息，不等价于完整发布历史。
- Helm history 如果只存在于 Secret 中，本方案不会读取 Secret 正文；后续建议接 Argo CD API、Helm 只读网关或 GitOps 发布记录来补齐。
- ConfigMap 当前只返回 metadata 与 data keys，不返回完整配置值，避免把敏感配置扩散到 MCP 输出中。
- 如果镜像 tag 没有 digest 或 Git commit 标注，只能判断 tag 级别版本，无法精确到镜像内容。

## 8. 推荐后续扩展

- 接入 Argo CD 只读 API，读取应用同步状态、revision、sync history、health。
- 接入镜像仓库只读 API，读取 tag -> digest -> build commit 映射。
- 建立发布记录索引，将 Git commit、镜像 digest、Helm chart version、ConfigMap checksum、发布人、发布时间统一写入只读数据源。
