# auto_inspection 功能梳理

## 定位与输出
- 基于 Prometheus 的运维巡检与事件聚合：告警统计、资源巡检、健康画像、异常事件流水、AI 总结。
- 主要产物：`weekly_report.md`（周报）与 `data/*.json`（过程数据/事件流水）。

## 事件流水（发现 → 聚合 → 升级 → Runbook）
1. `discover_targets.py`：从 Prometheus 获取 `instance/job`，输出 `data/targets.json`。
2. `baseline_builder.py`：按 `targets.json` 拉取 28 天历史数据，生成 `data/baseline/{cpu,mem,disk}.json`（P50/P95/mean）。
3. `baseline_anomaly.py`：对比当前值与基线 P95，偏离 > 20% 记为异常，输出 `data/anomalies.json`。
4. `health_profile.py`：基于 CPU/内存/磁盘/Swap 即时值打分与分级，输出 `data/health_profiles.json`。
5. `anomaly_merge.py`：按实例合并异常，计算风险分/等级/主风险，并补充健康画像，输出 `data/events.json`。
6. `event_lifecycle.py`：结合历史事件标注 `new/ongoing/resolved`，更新 `data/events_history.json`，输出 `data/events_lifecycle.json`。
7. `event_escalation.py`：按“持续、回归、多信号”规则升级风险等级，输出 `data/events_escalated.json`。
8. `runbook_attach.py`：按风险类型绑定 Runbook（无匹配时用默认模板），输出 `data/events_with_runbook.json`。

## 周报生成（告警 + 资源 + AI）
- `prom_alert_summary.py`：按显式时间窗口统计告警条件成立时长，生成 Markdown 表格片段。
- `prom_resource_check.py`：按阈值对 CPU/内存/磁盘聚合值分红黄绿，生成 Markdown 列表片段。
- `ai_summary.py`：调用本地 Ollama，根据“事实文本”生成周报 AI 总结。
- `weekly_inspection.py`：统一时间窗口，串联以上模块并写出 `weekly_report.md`。

## 外部依赖与接口
- Prometheus HTTP API：`/api/v1/query` 与 `/api/v1/query_range`。
- 本地 Ollama：`/api/generate`。

## 优化方向（已确认）
1. 配置集中化：统一管理 Prometheus/Ollama 地址、时间窗口、阈值、权重、偏离比例等配置，支持环境变量覆盖。
2. 流程一键化：提供 CLI 入口，按依赖顺序串联 targets→baseline→anomaly→event→lifecycle→escalation→runbook→report，支持跳步/重跑。
3. 事件历史管理：对 `events_history.json` 增加保留窗口或归档策略，防止长期膨胀。
4. 风险计算一致性：合并异常时按去重信号计分，排序使用统一风险等级映射。
5. 基线/百分位一致性：P95 等统计口径统一（同一 percentile 实现）。
6. 外部请求可靠性：Prometheus/Ollama 请求增加重试与退避，错误信息更清晰。
7. Runbook 体系：补充 `runbooks/runbooks.json` 模板，支持按多信号或风险等级匹配。
8. 最小测试：补充关键工具函数的单元测试。
