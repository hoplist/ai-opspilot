# MCP / Skill 变更记录：只读复杂问题诊断能力

## 1. 基本信息

- 日期：2026-04-24
- 操作人：Codex
- 需求来源：用户要求完善当前 MCP 和 Skill，并保持 MCP 不直接操作服务器
- 关联对话摘要：用户希望 MCP 能更好支持内存溢出、复杂问题排查，并询问后续应接入哪些只读数据源
- 变更主题：新增只读复杂 Pod 问题证据包工具 `diagnose_pod`
- 影响范围：MCP server、Codex Skill、Skill helper script、Codex MCP 文档

## 2. 需求原文

```text
帮我完善目前的mcp和skill，另外如果是那种内存溢出或者是一些较为复杂的问题我还应该接入什么插件来优化我mcp所能读到的数据源呢，禁止mcp直接操作服务器是前提
```

## 3. 目标

- 增加复杂 Pod / Workload 问题的只读证据包工具
- 明确 MCP 和 Skill 的安全边界
- 让 Skill 在 OOM、CrashLoop、探针失败、调度失败、镜像拉取失败、延迟和业务错误场景下有固定排查顺序
- 给出后续只读数据源接入路线
- 将远端 `192.168.48.200:32181/mcp` 更新到新版能力

## 4. 安全边界

- 是否允许 MCP 直接操作服务器：否
- 是否允许 MCP 修改 Kubernetes 资源：否
- 是否允许 MCP 修改基础设施配置：否
- 是否允许生成 investigation 记录：是
- 是否涉及凭证变更：否
- 是否涉及远端部署：是

说明：

```text
MCP 只能查询 RCA backend、日志、事件、incident、investigation、Prometheus 资源摘要。
MCP 不允许 SSH、执行 shell 命令、修改 Kubernetes 资源或修改基础设施配置。
diagnose_pod 可能通过 backend 创建 investigation 记录，但不执行修复动作。
```

## 5. 修改文件

| 类型 | 文件 | 说明 |
| --- | --- | --- |
| 修改 | `auto_inspection/auto_inspection_mcp.py` | 新增 `diagnose_pod` 工具、症状查询词、证据包组合逻辑 |
| 修改 | `C:/Users/Administrator/.codex/skills/auto-inspection-rca/SKILL.md` | 增加安全边界、复杂问题工作流、只读数据源建议 |
| 修改 | `C:/Users/Administrator/.codex/skills/auto-inspection-rca/scripts/auto_inspection_backend.py` | 新增 `diagnose-pod` 命令，修正 MCP initialize/session 调用流程 |
| 修改 | `docs/codex_mcp_integration.md` | 记录 `diagnose_pod`、安全边界和推荐数据源 |
| 新增 | `docs/cn/mcp_readonly_observability_roadmap.md` | 记录只读 MCP 可观测数据源接入路线 |
| 新增 | `docs/cn/mcp_skill_change_record_template.md` | 增加后续变更记录模板 |
| 修改 | `docs/cn/README.md` | 增加新文档入口 |
| 新增 | `docs/cn/change_records/2026-04-24-mcp-skill-readonly-diagnose-pod.md` | 本次变更记录 |

## 6. 新增或修改的 MCP 工具

| 工具名 | 类型 | 入参 | 数据源 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- | --- |
| `diagnose_pod` | 新增 | `namespace`, `pod`, `workload_name`, `symptom`, `q`, `range_hours`, `size`, `use_ai` | backend health、investigation、logs、events、incidents、Prometheus resources | 是 | 生成复杂 Pod / Workload 问题证据包 |

支持的 `symptom`：

- `oom`
- `crashloop`
- `probe`
- `pending`
- `imagepull`
- `latency`
- `error`
- `unknown`

## 7. 新增或修改的 Skill 能力

- 触发场景：用户询问 OOM、CrashLoopBackOff、探针失败、镜像拉取失败、调度失败、延迟、复杂业务错误
- 推荐工作流：优先使用 `diagnose-pod`，再按日志、事件、Prometheus、incident、investigation 组合证据
- 输出格式：说明发生了什么、关键证据、可能根因、下一步动作、缺失数据源
- 回退方式：MCP 不可用时通过 backend HTTP API 组合证据包

## 8. 后端 API 变化

本次未新增 backend API。

使用已有 API：

| 方法 | 路径 | 类型 | 是否只读 | 说明 |
| --- | --- | --- | --- | --- |
| `GET` | `/api/health/details` | 复用 | 是 | 查询依赖健康 |
| `GET` | `/api/search/logs` | 复用 | 是 | 查询日志 |
| `GET` | `/api/search/events` | 复用 | 是 | 查询事件 |
| `GET` | `/api/incidents/search` | 复用 | 是 | 查询 incident |
| `GET` | `/api/resources` | 复用 | 是 | 查询资源摘要 |
| `POST` | `/api/investigate` | 复用 | 生成调查记录 | 生成 investigation |

## 9. 部署动作

- 是否同步到 NFS：是
- NFS 路径：`192.168.48.206:/srv/nfs/observability/auto-inspection-rca`
- 是否重启 Deployment：是
- Deployment：`auto-inspection-rca`
- Namespace：`observability`
- 备份文件：`/srv/nfs/observability/auto-inspection-rca/auto_inspection/auto_inspection_mcp.py.bak.20260424_141526`

命令或动作摘要：

```powershell
python -m py_compile auto_inspection\auto_inspection_mcp.py C:\Users\Administrator\.codex\skills\auto-inspection-rca\scripts\auto_inspection_backend.py

# 通过 SSH/SFTP 将本地 auto_inspection/auto_inspection_mcp.py 同步到：
# 192.168.48.206:/srv/nfs/observability/auto-inspection-rca/auto_inspection/auto_inspection_mcp.py

kubectl rollout restart deployment/auto-inspection-rca -n observability
kubectl rollout status deployment/auto-inspection-rca -n observability --timeout=180s
```

## 10. 验证记录

| 验证项 | 命令 / 方法 | 期望结果 | 实际结果 |
| --- | --- | --- | --- |
| Python 语法检查 | `python -m py_compile ...` | 通过 | 通过 |
| Skill 命令帮助 | `python ... auto_inspection_backend.py diagnose-pod --help` | 显示参数 | 通过 |
| Deployment rollout | `kubectl rollout status ...` | 成功滚动 | 通过 |
| MCP tools/list | JSON-RPC `tools/list` | 出现 `diagnose_pod` | 通过，工具数为 12 |
| MCP diagnose_pod | JSON-RPC `tools/call` | 返回证据包 | 通过 |
| Skill diagnose-pod | `python ... diagnose-pod ...` | 返回证据包 | 通过 |

远端 MCP `tools/list` 验证摘要：

```json
{
  "tool_count": 12,
  "has_diagnose_pod": true
}
```

真实调用验证摘要：

```json
{
  "mode": "read_only_evidence_bundle",
  "server_commands": "not_allowed",
  "kubernetes_mutations": "not_allowed"
}
```

## 11. 回滚方式

本地文件回滚：

```powershell
# 从版本控制或备份恢复以下文件：
# auto_inspection/auto_inspection_mcp.py
# C:/Users/Administrator/.codex/skills/auto-inspection-rca/SKILL.md
# C:/Users/Administrator/.codex/skills/auto-inspection-rca/scripts/auto_inspection_backend.py
# docs/codex_mcp_integration.md
```

远端 NFS 回滚：

```powershell
# 在 192.168.48.206 上恢复：
cp /srv/nfs/observability/auto-inspection-rca/auto_inspection/auto_inspection_mcp.py.bak.20260424_141526 /srv/nfs/observability/auto-inspection-rca/auto_inspection/auto_inspection_mcp.py
```

Deployment 回滚：

```powershell
kubectl rollout restart deployment/auto-inspection-rca -n observability
kubectl rollout status deployment/auto-inspection-rca -n observability --timeout=180s
```

## 12. 已知限制

- `diagnose_pod` 当前复用已有 backend API，尚未接入 trace、profiling、eBPF、发布变更数据
- `diagnose_pod` 当前会生成 investigation 记录，适合排障留痕，但会增加历史记录
- 业务上下游分析依赖日志中存在 `trace_id`、`request_id`、`service` 等字段

## 13. 后续建议

- 优先接入 OpenTelemetry Trace 和业务关联字段
- 为发布变更增加只读 API，再暴露 `release_recent_changes` 类 MCP 工具
- 为 profiling 和 eBPF 先做只读摘要 API，避免 MCP 直接访问节点
- 每次后续改动都按 `mcp_skill_change_record_template.md` 记录
