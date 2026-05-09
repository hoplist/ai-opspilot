# RCA 存储架构设计

## 目标

调查结果现在拆成热/冷分层存储，目标是：

- 高频读取的元数据保持响应快
- 完整调查快照放到更适合长期留存的冷存储
- 搜索与聚合继续保留在 OpenSearch

## 当前存储层

### 1. 热存储

热调查元数据用于：

- 最近调查列表
- 高频摘要查询
- RCA 工作台历史卡片
- 推荐排序输入

支持驱动：

- SQLite
- MySQL

当前共享部署使用：

- MySQL
- 数据库：`auto_inspection`
- 表：`investigation_metadata`

### 2. 冷存储

冷存储保存完整 investigation JSON 归档。

当前驱动：

- MinIO

当前 bucket：

- `auto-inspection-archive`

对象路径结构：

- `investigations/YYYY/MM/DD/<investigation_id>.json`

### 3. 搜索层

OpenSearch 继续保存 investigation 文档，用于：

- 搜索
- 聚合
- 仪表盘图表

索引族：

- `inspection-investigations-*`

## 写入路径

每次调查完成后，当前会同时写入：

1. 本地 JSON 快照
2. OpenSearch investigation 索引
3. 热元数据存储
4. 冷归档存储

## 读取路径

完整调查读取优先级：

1. 本地文件缓存
2. 热存储指针
3. 冷归档对象

最近调查列表优先级：

1. 热元数据存储
2. 本地文件兜底

## 为什么这样拆

### 热存储

适合：

- 低延迟读取
- 结构化短数据
- 历史列表和摘要卡片

### 冷存储

适合：

- 完整 JSON 归档
- 长期留存
- 回放与导出

### OpenSearch

适合：

- 搜索
- 关联分析
- 图表聚合
- 文本证据检索

## 当前共享部署

共享 RCA 服务当前使用：

- `mysql-31326` 作为热存储
- `observability` 命名空间下的 MinIO 作为冷归档

## 后续优化建议

- 把 MySQL 和 MinIO 凭证完全收敛到独立 Secret
- 为 investigation 元数据表增加 schema migration
- 给 MinIO 增加归档生命周期策略
- 增加从冷存储回放完整 investigation 的恢复接口
