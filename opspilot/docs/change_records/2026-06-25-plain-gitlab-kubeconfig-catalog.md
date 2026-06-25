# 2026-06-25 Plain GitLab Kubeconfig Catalog

## 目标

当前先简化新集群接入：不引入 Vault、SOPS、SealedSecret 或手工 Secret
挂载。内网测试阶段由私有 GitLab `platform/opspilot-config` 作为唯一人工
维护入口，管理员可以查看明文 kubeconfig，但 OpsPilot API/CLI 不返回
kubeconfig 内容。

## 本阶段改动

- `Cluster` 配置模型新增归属字段：
  - `region`
  - `network_zone`
  - `business_line`
  - `business`
  - `owner`
  - `description`
- `clusters catalog` 输出新增区域、网络区、业务线、负责人。
- `ClusterCatalogRaw()` 和 legacy env catalog 解析支持这些字段。
- `config/opspilot-config/clusters/node200.yaml` 补充现有集群归属信息。
- `config/opspilot-config/README.md` 新增明文 kubeconfig 目录规范：
  - `kubeconfigs/<cluster>.kubeconfig`
  - `kubeconfig_path: /etc/opspilot/config/current/kubeconfigs/<cluster>.kubeconfig`
- `config/opspilot-config/kubeconfigs/README.md` 作为目录占位，并说明不要使用
  `.yaml/.yml` 后缀，避免被配置加载器当成 OpsPilot 配置解析。

## 最简接入流程

1. 管理员把目标集群只读 kubeconfig 放入私有 GitLab 配置仓库：

   ```text
   kubeconfigs/gz-inner.kubeconfig
   ```

2. 在 `clusters/*.yaml` 记录集群元数据和业务归属：

   ```yaml
   clusters:
     - name: gz-inner
       environment: test
       region: guangzhou
       network_zone: inner
       business_line: collaboration
       business: Guangzhou collaboration test cluster
       owner: ops
       kubernetes_mode: remote
       kubeconfig_path: /etc/opspilot/config/current/kubeconfigs/gz-inner.kubeconfig
       kube_context: gz-inner
   ```

3. `config-sync` 同步到 `opspilot-core`：

   ```text
   /etc/opspilot/config/current/kubeconfigs/gz-inner.kubeconfig
   ```

4. 客户端只使用 cluster 名：

   ```powershell
   opspilot inspect cluster --cluster gz-inner
   opspilot inspect pod --cluster gz-inner -n <namespace> --pod <pod>
   ```

## 边界

- 该方案仅适用于当前内网测试阶段。
- 仓库访问权限就是明文密钥边界，只有管理员可读。
- OpsPilot 可显示 `kubeconfig_path`、`business_line`、`owner` 等元数据。
- OpsPilot 不得通过 API/CLI 输出 kubeconfig 文件内容。
- 后续正式环境可迁移到 SOPS、SealedSecret、Vault/OpenBao 或 External
  Secrets Operator，但不阻塞当前最简接入。

## 验证

- `go test ./internal/configloader ./internal/catalog ./cli`
- `go test ./...`
- `go vet ./...`
- `go run ./cli config validate --dir ./config/opspilot-config`
