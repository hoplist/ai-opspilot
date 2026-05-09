# Auto Inspection RCA 使用手册与常见排障 Runbook

## 常用入口

- RCA Backend: `http://192.168.48.200:32180`
- RCA MCP: `http://192.168.48.200:32181/mcp`
- OpenSearch Dashboards: `http://192.168.48.200:32091`
- Prometheus: `http://192.168.48.200:32092`
- Pyroscope: `http://192.168.48.200:32095`

CLI helper：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py <command>
```

## 推荐排障入口

优先使用 Evidence Pack：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py context-pod --namespace <ns> --pod <pod> --symptom error --range-hours 6
```

Workload：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py context-workload --namespace <ns> --workload-name <name> --workload-kind Deployment --range-hours 6
```

Namespace：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py --prefer-backend context-workload --namespace observability --workload-name opensearch --workload-kind StatefulSet --range-hours 6
```

Evidence Pack 会聚合：

- 资源趋势
- 日志
- Kubernetes Events
- incidents
- release/change
- GitLab/Argo 证据
- Beyla/OTel RED 指标
- Falco runtime 事件
- Pyroscope profile 热点

## 常用命令

查日志：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py search-logs --namespace <ns> --pod <pod> --size 50
```

查事件：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py search-events --namespace <ns> --pod <pod> --size 50
```

查服务 RED：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py service-red-metrics --namespace <ns> --service <service> --limit 10
```

查 Falco runtime：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py runtime-events --namespace <ns> --pod <pod> --range-hours 6
```

查 profile：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py profile-hotspots --service-name <service> --range-hours 6
```

查告警风险：

```text
http://192.168.48.200:32180/api/alerts?range_hours=1
```

## Runbook：Pod 内存上涨/OOM 风险

1. 查询 Evidence Pack：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py context-pod --namespace <ns> --pod <pod> --symptom memory --range-hours 6
```

2. 检查：

- `summary.top_signals`
- `evidence.resources.pods`
- `events` 是否有 `OOMKilled`、`Killing`
- `logs` 是否有内存异常
- `profile_hotspots` 是否有 CPU 热点伴随

3. 判断：

- 内存持续上涨：疑似泄漏或缓存膨胀。
- 接近 limit：优先确认 limit 是否过小。
- OOMKilled：结合最后终止时间和日志定位。

## Runbook：CPU 高或 CPU Throttling

1. 查询 Workload Evidence Pack：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py context-workload --namespace <ns> --workload-name <workload> --workload-kind Deployment --symptom latency --range-hours 3
```

2. 查询 profile：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py profile-hotspots --service-name <service> --range-hours 3 --limit 10
```

3. 判断：

- CPU 使用高且 profile 有热点：看热点函数。
- CPU 不高但 throttling 高：调整 CPU limit/request 或扩容。
- CPU 高伴随请求量高：看 Beyla RED。

## Runbook：偶发 5xx/504

1. 先查 gateway 或业务日志：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py search-logs --namespace <gateway-ns> --q "504" --size 50
```

2. 查服务 RED：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py service-red-metrics --namespace <ns> --service <service> --limit 10
```

3. 查后端 Evidence Pack：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py context-workload --namespace <ns> --workload-name <workload> --symptom latency --range-hours 1
```

4. 判断：

- APISIX 有 504，Beyla 后端无请求：问题可能在 gateway 到 upstream。
- APISIX 504，Beyla 后端耗时高：后端处理慢。
- 同时有发布变化：优先关联 GitLab/Argo。
- 同时有重启/探针失败：优先看 Pod 事件。

## Runbook：发布后异常

1. 查 Argo CD：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py argocd-status --app-name <app>
```

2. 查发布上下文：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py release-context --app-name <app> --ref main
```

3. 查 workload 运行版本：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py release-workload --namespace <ns> --workload-name <workload> --workload-kind Deployment
```

4. 判断：

- Argo revision 是否变化。
- GitLab commit/pipeline 是否异常。
- image digest 是否对应预期版本。
- 异常时间是否紧跟发布。

## Runbook：Runtime 可疑行为

查询 Falco：

```powershell
python C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py runtime-events --namespace <ns> --pod <pod> --range-hours 24
```

重点看：

- rule
- priority
- process
- user
- output_fields

Falco 只作为证据源，不自动阻断。

## 日常巡检建议

每天或变更后查看：

```text
/api/health/details
/api/alerts?range_hours=24
```

重点关注：

- OpenSearch/Prometheus/Pyroscope 是否可用。
- Pod 内存是否有持续上涨。
- GitOps/Argo 是否 OutOfSync。
- RCA Backend/MCP 是否健康。
