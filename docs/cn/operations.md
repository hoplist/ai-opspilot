# RCA 运维指南

## 1. 日常检查项

建议每天至少检查以下几类状态：

- OpenSearch Pod 是否正常
- Fluent Bit DaemonSet 是否全量 Running
- OTel Collector 是否正常
- Prometheus targets 是否健康
- backend 与 MCP 是否可访问

## 2. 推荐验证入口

### 2.1 OpenSearch Dashboards

重点查看：

- `dashboard-auto-inspection-overview`
- `search-incidents-current`
- `search-k8s-logs-recent-errors`
- `search-k8s-events-warnings`

### 2.2 RCA API

重点查看：

- `/api/incidents/list`
- `/api/investigation-targets`
- `/api/investigations/latest`

## 3. 保留策略

当前已为以下索引安装 ISM retention policy：

- logs：14 天
- events：30 天
- incidents：60 天
- investigations：90 天

可通过配置项调整：

- `OPENSEARCH_RETENTION_LOGS_DAYS`
- `OPENSEARCH_RETENTION_EVENTS_DAYS`
- `OPENSEARCH_RETENTION_INCIDENTS_DAYS`
- `OPENSEARCH_RETENTION_INVESTIGATIONS_DAYS`

## 4. 快照基线

当前已注册本地文件系统 snapshot repository：

- repository: `auto-inspection-local-fs`
- location: `/usr/share/opensearch/data/snapshots`

当前这是一个“可运行基线”，建议后续补：

- 定时快照
- 独立备份存储
- 备份保留策略

## 5. 磁盘保护

当前 OpenSearch 已启用磁盘阈值：

- low: 85%
- high: 90%
- flood stage: 95%

建议日常关注：

- PVC / NFS 剩余空间
- 是否出现只读索引
- 是否出现写入失败

## 6. 日志规范化质量检查

建议定期确认以下字段是否持续有值：

- `severity`
- `logger`
- `message`
- `message_normalized`
- `service`
- `exception_type`
- `exception_message`
- `stack_language`

如果明显缺失，优先检查：

- Fluent Bit `configmap-logs.yaml`
- 应用日志输出格式
- OpenSearch template 是否已刷新

## 7. 事件与推荐质量检查

建议关注：

- incident 是否能进入 `search-incidents-current`
- 调查对象是否能把 `investigation + event` 合并
- 高风险对象是否具备 `restart_total / waiting_reason / last_terminated_reason`

## 8. 常见问题

### 8.1 Dashboards 能打开但没有新图表

处理方式：

1. 执行 `python bootstrap_dashboards.py`
2. 刷新浏览器缓存
3. 检查 Dashboards saved object 是否创建成功

### 8.2 OpenSearch bootstrap 失败

优先检查：

- OpenSearch 是否已 Ready
- `_plugins/_ism` 是否可用
- snapshot `path.repo` 是否已配置

### 8.3 没有 incident 数据

优先检查：

1. Fluent Bit logs / events 是否写入 OpenSearch
2. Prometheus 与 kube-state-metrics 是否有数据
3. pipeline 是否已执行到 `runbook` 阶段

### 8.4 RCA 页面摘要为空

优先检查：

- `/api/investigations/latest`
- Prometheus context 是否有 `restart_total / request / limit`
- K8s Pod fallback 是否正常

## 9. 后续建议

建议继续补齐：

- OpenSearch 安全认证
- snapshot 定时任务
- 后端定时健康巡检
- 更细粒度的日志 parser 与异常抽取
