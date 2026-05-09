# 集群与 RCA 栈恢复记录（2026-04-21）

本文记录 2026-04-21 在 `192.168.48.200/201/202/206` 环境上完成的一次实际恢复过程，目标包括：

- 恢复 `192.168.48.200` 上的 Kubernetes control plane
- 在尽量不动 `192.168.48.201` 和 `192.168.48.202` worker 运行态的前提下恢复集群
- 将 `192.168.48.206:/srv/nfs` 中已有数据重新挂载为 PVC
- 恢复 `mysql-31326`、`observability`、`monitoring` 相关服务
- 验证 RCA 页面、RCA API、OpenSearch Dashboards saved objects 是否可用

## 1. 环境与节点

- `192.168.48.200`: `k8s-master-1`
- `192.168.48.201`: `k8s-worker-1`
- `192.168.48.202`: `k8s-worker-2`
- `192.168.48.206`: NFS 数据机，导出 `/srv/nfs`

恢复时使用的远程方式为 SSH。

## 2. 故障现象

最初现象：

- 本机 `kubectl` 当前 context 为 `kubernetes-admin@kubernetes`
- 访问 `192.168.48.200:6443` 被拒绝
- `kubectl` 无法连接 API Server

在 `192.168.48.200` 上进一步检查后确认：

- `kubelet`: `active`
- `containerd`: `active`
- `kube-scheduler`: 在运行
- `kube-controller-manager`: 在运行
- `etcd`: 静态 Pod 持续 `CrashLoopBackOff`
- `kube-apiserver`: 因依赖 etcd 不可用而反复退出
- `6443`: 未监听
- `2379/2380`: 未监听

## 3. 根因判断

关键日志来自 `etcd` 容器：

```text
panic: assertion failed: Page expected to be: 2787, but self identifies as 3774352084458420067
```

这说明 `etcd` 使用的 BoltDB 数据文件已经损坏，直接导致：

1. `etcd` 无法启动
2. `kube-apiserver` 连接不到 `127.0.0.1:2379`
3. Kubernetes control plane 整体不可用

损坏文件对应的数据目录为：

- `/var/lib/etcd/member/snap/db`

## 4. 恢复前保护性备份

在任何写操作前，先在 `192.168.48.200` 上做保护性备份，避免二次损失。

备份产物：

- 目录：`/root/k8s_recovery_backup_20260421_151130`
- 压缩包：`/root/k8s_recovery_backup_20260421_151130.tgz`

建议至少备份以下内容：

- `/etc/kubernetes`
- `/var/lib/etcd`
- 关键静态 Pod manifest
- 当前诊断输出与日志

实际恢复后，损坏的 etcd 目录未删除，而是保留为：

- `/var/lib/etcd.corrupt.20260421_151309`

## 5. etcd 快照定位

在 `192.168.48.200` 上查到可用历史快照：

- `/restore/k8s_restore_20250924_174131/k8s_20250924_171912/etcd-snapshot.db`

快照时间：

- `2025-09-24 17:19:12 +0800`

这是本次恢复中唯一确认可读、可用于恢复的 etcd 快照。

## 6. etcd 恢复步骤

下面是实际恢复时遵循的顺序。为了避免 kubelet 持续重启损坏组件，先暂停静态 Pod 拉起，再做快照恢复。

### 6.1 暂停 kubelet 和静态 Pod 重建

建议流程：

1. 停止 `kubelet`
2. 备份 `/etc/kubernetes/manifests`
3. 临时移走 `etcd.yaml`、`kube-apiserver.yaml` 等静态 Pod manifest，避免恢复过程中自动拉起

目标是让控制面进入一个“可操作但不会自动抖动”的状态。

### 6.2 保留损坏数据目录

不要覆盖原目录，先改名保留：

```text
/var/lib/etcd  ->  /var/lib/etcd.corrupt.20260421_151309
```

这样即便恢复失败，仍然可以继续做离线取证或尝试其他修复。

### 6.3 从快照恢复 etcd 数据

使用 `etcdctl snapshot restore` 将快照恢复到一个新的 etcd 数据目录。

恢复时要注意与当前静态 Pod manifest 中的配置一致，包括：

- name
- data-dir
- initial-cluster
- initial-advertise-peer-urls
- advertise-client-urls

恢复完成后，将新的数据目录替换为 `/var/lib/etcd`。

### 6.4 恢复静态 Pod manifest 并重启 kubelet

恢复顺序：

1. 放回 `etcd.yaml`
2. 放回 `kube-apiserver.yaml`
3. 启动 `kubelet`
4. 观察 `etcd` 与 `kube-apiserver` 是否转为稳定运行

### 6.5 恢复后验证

核心验证点：

- `ss -lntp | grep 2379`
- `ss -lntp | grep 6443`
- `kubectl get nodes -o wide`
- `kubectl get pods -n kube-system -o wide`

实际恢复结果：

- `etcd` 恢复正常
- `kube-apiserver` 恢复正常
- `kubectl cluster-info` 正常
- `k8s-master-1`、`k8s-worker-1`、`k8s-worker-2` 均恢复为 `Ready`

## 7. 恢复影响与风险

本次 control plane 恢复依赖历史 etcd 快照，因此必须明确：

- 集群对象状态回退到了 `2025-09-24 17:19:12 +0800` 附近
- 此时间点之后创建或变更的 Kubernetes 对象，默认不会自动保留
- 这也是“以前的 pod 不见了”的直接原因

因此，这次恢复的本质是：

- 恢复了控制面可用性
- 但 control plane 元数据回退到了历史时间点

## 8. Worker 节点处理原则

为了尽量保住 `201/202` 上可能还在运行的容器，本次处理遵循了一个重要原则：

- 不重装 worker
- 不清理 `/var/lib/containerd`
- 不重置 `kubelet`
- 不重建节点
- 优先只修复 `192.168.48.200` control plane

尽管最终历史 Pod 元数据未能全部保回，但这个策略仍然是相对风险最低的恢复方式。

## 9. 206 上的 NFS 数据梳理

在 `192.168.48.206` 上确认 `/srv/nfs` 下存在以下可恢复数据：

- `/srv/nfs/mysql-lab/mysql-31326`
- `/srv/nfs/monitoring/prometheus`
- `/srv/nfs/observability/opensearch`
- `/srv/nfs/observability/minio`
- `/srv/nfs/observability/auto-inspection-rca`

NFS 导出可用，网段允许 `192.168.48.0/24` 访问。

数据体量大致如下：

- Prometheus: `615M`
- OpenSearch: `153M`
- MinIO: `728K`
- auto-inspection-rca: `3.0M`
- MySQL: `223M`

## 10. PVC 与服务恢复顺序

本次恢复顺序按“先数据、再基础服务、最后应用接入”的原则执行：

1. 恢复 MySQL
2. 恢复 OpenSearch
3. 恢复 MinIO
4. 恢复 OpenSearch Dashboards
5. 恢复 RCA Backend
6. 恢复 OTel Collector / Fluent Bit
7. 恢复 Prometheus

### 10.1 MySQL 恢复

参考来源：

- `192.168.48.206:/opt/backup/mysql-31326`
- `192.168.48.206` 上原有 YAML

在仓库中新增：

- `deploy/db/mysql-31326/namespace.yaml`
- `deploy/db/mysql-31326/pv.yaml`
- `deploy/db/mysql-31326/pvc.yaml`
- `deploy/db/mysql-31326/mysql-31326.yaml`
- `deploy/db/mysql-31326/kustomization.yaml`

恢复思路：

- 将 `192.168.48.206:/srv/nfs/mysql-lab/mysql-31326` 暴露成 PV
- 绑定到 `db/pvc-db`
- 以 StatefulSet/单实例 MySQL 方式恢复 `mysql-31326`

恢复后验证：

- Pod `db/mysql-31326-0` 为 `Running`
- `auto_inspection` 数据库存在
- `investigation_metadata` 表存在
- 记录数查询返回 `1`

### 10.2 Observability 恢复

按仓库 `deploy/observability` 恢复：

- OpenSearch
- OpenSearch Dashboards
- MinIO
- OTel Collector
- Fluent Bit

本次额外调整：

- `deploy/observability/minio/deployment.yaml`
  将镜像改为 `docker.1ms.run/minio/minio:latest`
- `deploy/observability/otel-collector/deployment.yaml`
  将镜像改为 `docker.1ms.run/otel/opentelemetry-collector-contrib:0.139.0`

调整原因：

- 默认镜像源在当前环境拉取失败
- 切换为可拉取镜像后服务恢复正常

### 10.3 RCA 服务恢复

按仓库 `deploy/rca-service` 恢复，并补充：

- `deploy/rca-service/secret.yaml`
- `deploy/rca-service/kustomization.yaml` 中加入 `secret.yaml`

恢复结果：

- `auto-inspection-rca` 正常运行
- HTTP API 可访问
- MCP 入口可访问

### 10.4 Prometheus 恢复

参考目录：

- `deploy/monitoring/prometheus`

恢复方式：

- 先创建 namespace 与 PV
- 再通过 Helm 恢复 `auto-prometheus`

恢复后验证：

- `monitoring/auto-prometheus-server` 为 `Running`
- `/-/ready` 返回 `200`

## 11. 本次修改过的仓库文件

新增：

- `deploy/db/mysql-31326/kustomization.yaml`
- `deploy/db/mysql-31326/namespace.yaml`
- `deploy/db/mysql-31326/pv.yaml`
- `deploy/db/mysql-31326/pvc.yaml`
- `deploy/db/mysql-31326/mysql-31326.yaml`
- `deploy/rca-service/secret.yaml`

修改：

- `deploy/rca-service/kustomization.yaml`
- `deploy/observability/minio/deployment.yaml`
- `deploy/observability/otel-collector/deployment.yaml`

## 12. 关键服务恢复结果

最终主要服务状态：

- `db/mysql-31326-0`: `Running`
- `observability/opensearch-0`: `Running`
- `observability/opensearch-dashboards`: `Running`
- `observability/minio`: `Running`
- `observability/auto-inspection-rca`: `Running`
- `observability/otel-collector`: `Running`
- `observability/fluent-bit-logs`: `Running`
- `observability/fluent-bit-events`: `Running`
- `monitoring/auto-prometheus-server`: `Running`

主要入口：

- MySQL: `192.168.48.200:31326`
- OpenSearch: `http://192.168.48.200:32090`
- Dashboards: `http://192.168.48.200:32091`
- Prometheus: `http://192.168.48.200:32092`
- MinIO API: `http://192.168.48.200:32093`
- MinIO Console: `http://192.168.48.200:32094`
- RCA Backend: `http://192.168.48.200:32180`
- RCA MCP: `http://192.168.48.200:32181/mcp`

## 13. RCA 页面与 Dashboards 检查结果

### 13.1 RCA 页面

已验证以下入口：

- `http://192.168.48.200:32180/dashboard-rca/`
- `http://192.168.48.200:32180/api/health`
- `http://192.168.48.200:32180/api/investigations/latest`
- `http://192.168.48.200:32180/api/investigation-targets?limit=5`

检查结果：

- RCA 页面返回 `200`
- RCA 健康检查正常
- `latest investigation` 能读到恢复后的 MySQL/MinIO/OpenSearch 数据
- 当前 `incidents/list` 为空，但 `investigation-targets` 与 `investigations/latest` 正常，不属于前端坏链路

### 13.2 OpenSearch Dashboards

状态检查：

- `/api/status` 返回 `green`
- SavedObjects service 已完成 migration

对象检查：

- `index-pattern`: 共 `7` 个
- `search`: 共 `74` 个
- `dashboard`: 共 `2` 个
- `visualization`: 共 `10` 个

重点对象存在：

- `dashboard-auto-inspection-overview`
- `dashboard-langfuse-clickhouse-rca`
- `search-incidents-current`
- `investigation-20260421020710-3a3edcc4-logs`
- `investigation-20260421020710-3a3edcc4-events`
- `logs-k8s-data-view`
- `events-k8s-data-view`

引用一致性检查结果：

- 未发现 dashboard/search/visualization 到 data view 的缺失引用
- 返回结果为 `NO_MISSING_REFERENCES`

直接访问检查结果：

- `dashboard-auto-inspection-overview`: `200`
- `dashboard-langfuse-clickhouse-rca`: `200`
- `search-incidents-current`: `200`
- `investigation-20260421020710-3a3edcc4-logs`: `200`
- `investigation-20260421020710-3a3edcc4-events`: `200`

结论：

- 当前没有发现 Dashboards saved objects 缺页
- 当前没有发现关键页面坏链路

## 14. 建议的标准恢复顺序

如果后续再遇到同类问题，建议采用以下固定顺序：

1. 先判断是 control plane 故障还是数据面故障
2. 先做 `/etc/kubernetes` 和 `/var/lib/etcd` 的保护性备份
3. 确认是否存在可用 etcd 快照
4. 只恢复 control plane，不先动 worker
5. 集群 Ready 后，再恢复 NFS 挂载型服务
6. 先恢复数据库与存储，再恢复 OpenSearch，再恢复 RCA 服务
7. 最后检查 Dashboards saved objects 和 RCA 页面链路

## 15. 后续建议

- 增加 etcd 定时快照并同步到独立存储
- 将快照时间和保留策略纳入运维文档
- 为 `deploy/db/mysql-31326`、`observability`、`monitoring` 增加一键恢复脚本
- 为 RCA 页面增加一个显式的“数据已回退到快照时间点”提示
- 增加恢复后自动巡检脚本，覆盖：
  - kube-system 组件健康
  - RCA API 健康
  - Dashboards status
  - Saved objects 完整性
  - 关键服务 NodePort 可访问性
