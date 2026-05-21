# Kubernetes Pod 日志按需读取设计

## 目标

OpsPilot 不默认全量采集 Kubernetes 容器日志到 OpenSearch/ELK。日常排障中，AI 和运维人员通常只需要某个 Pod 或 Workload 的短窗口日志。

因此默认方案是：

```text
opspilot-core -> Kubernetes API pods/log -> 短窗口日志 -> Evidence Pack
```

## 日志分层

| 日志类型 | 默认方式 | 用途 |
| --- | --- | --- |
| Pod 当前日志 | Kubernetes API `pods/log` 按需读取 | 最近异常排查 |
| Pod previous 日志 | Kubernetes API `pods/log?previous=true` 按需读取 | CrashLoopBackOff / 重启排查 |
| Kubernetes Event | Kubernetes API 读取 | 调度、探针、镜像、驱逐等证据 |
| 网关日志 | ELK | 504、5xx、慢请求 |
| 业务日志 | ELK | request_id、trace_id、user_id、order_id 关联 |
| 普通服务器关键日志 | ELK 或现有日志平台 | 按需接入 |

## Backend API

建议接口：

- `GET /api/k8s/logs/pod`
- `GET /api/k8s/logs/workload`
- `GET /api/k8s/logs/recent`
- `GET /api/context/pod`
- `GET /api/context/workload`

建议参数：

- `namespace`
- `pod`
- `container`
- `tail_lines`
- `since_seconds`
- `limit_bytes`
- `previous`
- `timestamps`
- `q`

## 查询保护

默认限制：

- `tail_lines <= 300`
- `since_seconds <= 1800`
- `limit_bytes <= 1MiB`
- Workload 日志查询只选择异常、最近重启或最新的少量 Pod。
- MCP 工具返回更小窗口。
- 同一 Pod 短时间重复查询使用缓存。
- 对日志内容做脱敏和截断。

## RBAC

只读权限：

```yaml
apiGroups:
  - ""
resources:
  - pods
  - pods/log
  - events
  - namespaces
  - services
  - configmaps
verbs:
  - get
  - list
  - watch
---
apiGroups:
  - apps
resources:
  - deployments
  - statefulsets
  - daemonsets
  - replicasets
verbs:
  - get
  - list
  - watch
```

不授予：

- `create`
- `update`
- `patch`
- `delete`
- `exec`
- `attach`
- `portforward`
- `secrets` 正文读取

## 使用边界

适合：

- 最近异常 Pod 排查。
- CrashLoopBackOff previous 日志。
- 告警触发后拉相关 Pod 短窗口日志。
- AI Evidence Pack。

不适合：

- 最近 7 天全局全文搜索。
- 合规审计长期留存。
- 大量用户持续 tail 全集群日志。
- 查已删除很久的 Pod 历史日志。

长期检索继续交给 ELK。
