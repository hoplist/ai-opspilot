# RCA 产品说明

## 产品定位

本项目定位为一套面向 Kubernetes 场景的日志优先型 AI RCA 平台，用于把：

- Kubernetes 容器日志
- Kubernetes Events
- Prometheus 指标上下文
- AI 调查结果

统一组织成可检索、可调查、可复用的排障产品。

## 核心能力

### 1. 全链路问题证据聚合

平台将以下信息整合到单一排障入口中：

- OpenSearch 中的规范化日志
- OpenSearch 中的 Kubernetes Events
- OpenSearch 中的 incidents 与 investigations
- Prometheus 中的 Pod / Node / kube-state 指标
- K8s API fallback 返回的 Pod 状态与容器状态

### 2. AI 调查与 RCA

后端会把“日志 + K8s 事件 + Prometheus 上下文”拼装成统一调查输入，输出：

- 一句话结论
- 根因排序
- 支持证据 / 反证
- 关键时间线
- 建议动作
- 需要人工确认项

### 3. 当前问题事件单一事实源

当前问题事件统一以 OpenSearch `inspection-incidents-*` 为主事实源。

本地 JSON 工件仅作为：

- 缓存
- 回退
- 导出中间产物

### 4. 调查对象推荐

调查对象列表会综合以下信号进行推荐排序：

- incident 风险等级
- runbook 是否存在
- investigation 历史次数
- restart total / restart increase
- OOMKilled / CrashLoopBackOff / NotReady 等强信号
- 当前 source fingerprint 下的最新 incident 与 investigation 数据

### 5. 可视化与工作台

当前包含两类入口：

- OpenSearch Dashboards
  用于 Discover、Saved Search、Dashboard 图表查看
- 本地 RCA 页面 `dashboard-rca`
  用于直接发起调查、查看摘要卡片、跳转日志/事件/Dashboard

## 核心数据域

当前主要索引：

- `logs-k8s-*`
- `events-k8s-*`
- `inspection-incidents-*`
- `inspection-investigations-*`

当前统一关键字段：

- `cluster`
- `namespace`
- `pod`
- `container`
- `node`
- `service`
- `severity`
- `logger`
- `message`
- `message_normalized`
- `exception_type`
- `exception_message`
- `stack_language`

## 典型使用方式

### 场景 1：值班排障

1. 打开 `Current Incidents`
2. 选择高风险 Pod
3. 点击 `Run RCA`
4. 查看摘要卡片与根因排序
5. 点击 `Open Logs / Open Events / Open Dashboard`

### 场景 2：最近问题复盘

1. 打开 `Investigations - Recent`
2. 按命名空间或对象检索
3. 查看历史调查结果与对应证据

### 场景 3：在 Codex 对话中调用

后端与 MCP 已经打通，后续可通过 Skill / MCP 直接触发：

- 日志搜索
- 事件搜索
- incident 列表
- 一键调查

## 当前交付边界

本轮已完成：

- 日志规范化与结构化异常提取
- incidents 单一事实源
- 调查对象推荐增强
- RCA 摘要卡片
- Dashboard 图表升级
- OpenSearch retention / snapshot / disk watermark 基线

仍建议后续继续增强：

- snapshot 定时任务
- OpenSearch 认证与安全插件
- 更细粒度的日志 parser
- RCA 页中文文案与交互细节进一步清洗
