# 确定性 CLI 与 AI Skill 编排契约

OpsPilot 的核心形态是：

```text
确定性 CLI + AI Skill 编排
```

AI 不直接操作集群，也不直接拼接底层命令。AI 通过 Skill 判断问题类型，再调用稳定的 `opspilot` CLI 或 MCP 工具。

## opspilot CLI 负责

- 真正执行只读查询动作。
- 提供稳定命令入口。
- 暴露 `schema`。
- 做认证、配置、权限边界。
- 把复杂系统能力收敛成统一命令。
- 返回稳定 JSON。
- 对日志和大结果做限流、截断和脱敏。
- 对潜在动作命令强制二次确认。

## opspilot-skill 负责

- 告诉 AI 什么时候使用哪个命令。
- 先查 `opspilot schema` 或 `opspilot <group> --help`。
- 优先走 shortcut。
- 明确哪些命令只读，哪些命令必须用户确认。
- 把 CLI 结果整理成人能看的结论。
- 不允许臆测，不允许绕过 CLI 直接操作底层系统。

## 命令分组建议

```bash
opspilot schema
opspilot inventory overview
opspilot inventory servers
opspilot inventory containers
opspilot k8s pods --namespace prod --status abnormal
opspilot k8s logs pod --namespace prod --pod xxx --tail 300
opspilot k8s logs pod --namespace prod --pod xxx --previous
opspilot metrics top-nodes --by memory
opspilot metrics top-containers --by cpu
opspilot context pod --namespace prod --pod xxx
opspilot diagnose pod --namespace prod --pod xxx
opspilot release workload --namespace prod --name gateway
opspilot backup status --biz mysql-23306
```

## 只读命令

默认阶段只开放只读命令：

- `inventory`
- `k8s`
- `logs`
- `metrics`
- `context`
- `diagnose`
- `release`
- `backup status`

## 动作命令

后续如果加入动作命令，必须显式确认：

```bash
opspilot action restart-pod --namespace prod --pod xxx --confirm
opspilot action rollout-restart --namespace prod --workload gateway --confirm
opspilot action scale --namespace prod --workload gateway --replicas 3 --confirm
```

Skill 必须在调用动作前向用户确认影响范围。

## 输出约定

CLI 默认输出 JSON：

```json
{
  "ok": true,
  "data": {},
  "warnings": [],
  "source": {
    "backend": "opspilot-core",
    "time": "2026-05-14T00:00:00+08:00"
  }
}
```

人类终端可以支持 `--format table`，但 AI 默认使用 JSON。
