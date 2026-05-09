# 鍐呯綉閮ㄧ讲鏈嶅姟娓呭崟

## 鑼冨洿

鏈枃鏁寸悊褰撳墠 auto-inspection RCA 骞冲彴鍦ㄥ唴缃戦儴缃叉椂闇€瑕佺殑鏈嶅姟銆侀暅鍍忋€佸惎鍔ㄩ『搴忋€佹毚闇茬鍙ｃ€侀儴缃叉柟寮忓拰瀵瑰簲閮ㄧ讲鏂囦欢銆?
褰撳墠浠?GitOps 浠撳簱涓哄噯锛?
```text
D:\code\auto_inspection\worktrees\gitops-manifests
```

Argo CD 褰撳墠鍚屾鍏ュ彛锛?
```text
apps/observability-application.yaml
clusters/test/observability
```

璇存槑锛?
- `clusters/test/observability` 鏄綋鍓?Argo CD 瀹為檯鍚屾璺緞銆?- `source/yaml` 鍜?`source/deploy` 鏄潵婧愬壇鏈?缁存姢鍓湰锛屼笉鏄綋鍓?Argo CD 鐨勭洿鎺ュ悓姝ュ叆鍙ｃ€?- Prometheus銆丮ySQL銆丟itLab銆丷egistry銆丄rgo CD 灞炰簬骞冲彴鍓嶇疆渚濊禆鎴栫嫭绔嬮儴缃查」銆?
## 鎬讳綋閮ㄧ讲椤哄簭

鎺ㄨ崘椤哄簭锛?
1. 鍐呯綉鍩虹渚濊禆锛欿ubernetes銆丯FS銆乧ontainerd registry 閰嶇疆銆?2. 鍐呯綉鍒跺搧渚濊禆锛欸itLab銆丷egistry銆?3. 鏁版嵁渚濊禆锛歁ySQL銆丳rometheus銆?4. Argo CD锛欳RD銆乧ore 缁勪欢銆丄pplication銆?5. Observability 鍩虹瀛樺偍锛歁inIO銆丱penSearch銆?6. 鏁版嵁閲囬泦锛欶luent Bit銆丱Tel Collector銆?7. 娣卞眰鎺掗殰缁勪欢锛欱eyla銆丗alco銆丳yroscope銆丄lloy銆?8. RCA Backend/MCP銆?9. 瀹氭椂浠诲姟锛歄penSearch snapshot銆丷CA alert notify dry-run銆?
Kustomize/Argo CD 涓嶄弗鏍间繚璇佽祫婧愭寜涓氬姟渚濊禆椤哄簭鍚姩锛屾墍浠ュ叧閿粍浠堕渶瑕佷緷闈?readiness/liveness probe 鍜屾湇鍔＄閲嶈瘯鏉ュ厹搴曘€?
## 鍓嶇疆渚濊禆娓呭崟

| 椤圭洰 | 鐢ㄩ€?| 闀滃儚 | 鏆撮湶绔彛 | 閮ㄧ讲鏂瑰紡 | 閮ㄧ讲鏂囦欢 |
|---|---|---|---|---|---|
| GitLab | 淇濆瓨 GitOps 浠撳簱銆佷唬鐮佷粨搴?| `docker-hub.tpo.xzoa.com/auto-inspection/gitlab-ce:latest` | `192.168.48.206:8929`锛孲SH `192.168.48.206:2224` | node206 Docker 鍓嶇疆鏈嶅姟 | `deploy/intranet-bundle/services/gitlab/docker-compose.yml` |
| Registry | 淇濆瓨鍐呯綉闀滃儚 | `docker-hub.tpo.xzoa.com/auto-inspection/registry:2` | `localhost:5002 -> 5000`锛岄泦缇よ闂?`192.168.48.1:5002` | Docker 鍓嶇疆鏈嶅姟 | `deploy/intranet-bundle/services/registry/docker-compose.yml` |
| containerd registry 閰嶇疆 | 鍏佽鑺傜偣鎷夊彇 HTTP registry | 鏃?| 鏃?| 鑺傜偣閰嶇疆 | `/etc/containerd/certs.d/192.168.48.1:5002/hosts.toml` |
| hostPath 数据目录 | hostPath 数据目录，固定到 `xzyc115-19` | 无 | `/data/auto-inspection/<应用名>` | 节点本地目录 | 各组件 Deployment/StatefulSet/Helm values |
| MySQL | RCA investigation 鐑瓨鍌?| `docker-hub.tpo.xzoa.com/auto-inspection/mysql:8.0.26`锛宨nit `docker-hub.tpo.xzoa.com/auto-inspection/busybox:1.36` | NodePort `31326`锛孲ervice `3306` | Kustomize/鎵嬪伐 apply | `source/deploy/db/mysql-31326` |
| Prometheus | 鎸囨爣鏌ヨ銆丷CA 璧勬簮璇佹嵁銆丅eyla RED 鏌ヨ闈?| `docker-hub.tpo.xzoa.com/auto-inspection/prometheus:v3.11.0`锛宍docker-hub.tpo.xzoa.com/auto-inspection/prometheus-config-reloader:v0.90.1`锛宍docker-hub.tpo.xzoa.com/auto-inspection/kube-state-metrics:v2.18.0`锛宍docker-hub.tpo.xzoa.com/auto-inspection/node-exporter:v1.10.2` | NodePort `32092`锛孲ervice `9090` | Helm | `source/yaml/monitoring/prometheus/install.ps1`锛宍source/yaml/monitoring/prometheus/values.yaml` |


## hostPath 数据目录约定

当前内网不再使用 NFS。所有平台持久化数据统一固定到 `xzyc115-19` 节点，工作负载直接使用 `hostPath` volume，不创建 PV/PVC；业务 Pod 通过 `nodeSelector` 固定到带有 `auto-inspection/storage-node=xzyc115-19` 标签的节点。当前集群实际承载节点为 `k8s-worker-2`。

目录约定：

```text
/data/auto-inspection/mysql-31326
/data/auto-inspection/prometheus
/data/auto-inspection/minio
/data/auto-inspection/opensearch
/data/auto-inspection/pyroscope
/data/auto-inspection/auto-inspection-rca
```

注意：如果集群中已经存在旧 NFS PV/PVC，需要先停应用、备份或迁移数据，然后删除旧 PVC/PV；新版本工作负载会直接挂载 hostPath，不再创建 PVC。

## Argo CD 娓呭崟

| 鏈嶅姟 | 鐢ㄩ€?| 闀滃儚 | 鏆撮湶绔彛 | 閮ㄧ讲鏂瑰紡 | 閮ㄧ讲鏂囦欢 |
|---|---|---|---|---|---|
| argocd-application-controller | GitOps reconcile 鎺у埗鍣?| `docker-hub.tpo.xzoa.com/auto-inspection/argocd:v3.3.8` | ClusterIP metrics | GitOps/bootstrap | `clusters/test/argocd-core/statefulsets.yaml` |
| argocd-server | Argo CD API/UI | `docker-hub.tpo.xzoa.com/auto-inspection/argocd:v3.3.8` | NodePort `32084` HTTP锛宍32443` HTTPS | GitOps/bootstrap | `clusters/test/argocd-core/deployments.yaml`锛宍clusters/test/argocd-core/services.yaml` |
| argocd-repo-server | 鎷夊彇 Git 浠撳簱骞舵覆鏌?manifests | `docker-hub.tpo.xzoa.com/auto-inspection/argocd:v3.3.8` | ClusterIP `8081`锛宍8084` | GitOps/bootstrap | `clusters/test/argocd-core/deployments.yaml`锛宍clusters/test/argocd-core/services.yaml` |
| argocd-redis | Argo CD 缂撳瓨 | `docker-hub.tpo.xzoa.com/auto-inspection/redis:8.2.3-alpine` | ClusterIP `6379` | GitOps/bootstrap | `clusters/test/argocd-core/deployments.yaml`锛宍clusters/test/argocd-core/services.yaml` |
| argocd-dex-server | Argo CD SSO/OIDC 缁勪欢 | `docker-hub.tpo.xzoa.com/auto-inspection/dex:v2.43.0` | ClusterIP `5556/5557/5558` | GitOps/bootstrap | `clusters/test/argocd-core/deployments.yaml`锛宍clusters/test/argocd-core/services.yaml` |
| argocd-applicationset-controller | ApplicationSet 鎺у埗鍣?| `docker-hub.tpo.xzoa.com/auto-inspection/argocd:v3.3.8` | ClusterIP `7000/8080` | GitOps/bootstrap | `clusters/test/argocd-core/deployments.yaml`锛宍clusters/test/argocd-core/services.yaml` |
| argocd-notifications-controller | 閫氱煡鎺у埗鍣?| `docker-hub.tpo.xzoa.com/auto-inspection/argocd:v3.3.8` | metrics `9001` | GitOps/bootstrap | `clusters/test/argocd-core/deployments.yaml` |

Argo CD bootstrap 鏂囦欢锛?
```text
apps/argocd-bootstrap-project.yaml
apps/argocd-bootstrap-application.yaml
apps/argocd-core-project.yaml
apps/argocd-core-application.yaml
clusters/test/argocd-bootstrap
clusters/test/argocd-core
```

Observability Application锛?
```text
apps/observability-project.yaml
apps/observability-application.yaml
```

## Observability/RCA 鏈嶅姟娓呭崟

| 椤哄簭 | 鏈嶅姟 | 绫诲瀷 | 鐢ㄩ€?| 闀滃儚 | 鏆撮湶绔彛 | 閮ㄧ讲鏂囦欢 |
|---:|---|---|---|---|---|---|
| 1 | MinIO | Deployment + PVC + Service | investigation 鍐峰瓨鍌ㄣ€佸綊妗ｅ璞″瓨鍌?| `docker.1ms.run/minio/minio:latest` | NodePort `32093` API -> `9000`锛宍32094` Console -> `9001` | `clusters/test/observability/minio` |
| 2 | OpenSearch | StatefulSet + PVC + Service | 鏃ュ織銆佷簨浠躲€乮ncident銆乮nvestigation銆乼race 绱㈠紩瀛樺偍 | `opensearchproject/opensearch:2.19.5`锛宨nit `docker-hub.tpo.xzoa.com/auto-inspection/busybox:1.36` | NodePort `32090` -> `9200`锛宮etrics `9600`锛沨eadless `9200/9300/9600` | `clusters/test/observability/opensearch` |
| 3 | OpenSearch Dashboards | Deployment + Service | OpenSearch 鏌ヨ UI | `opensearchproject/opensearch-dashboards:2` | NodePort `32091` -> `5601` | `clusters/test/observability/opensearch-dashboards` |
| 4 | Fluent Bit logs | DaemonSet | 閲囬泦鍚勮妭鐐瑰鍣ㄦ棩蹇楀埌 OpenSearch | `fluent/fluent-bit:3.2.8` | 瀹瑰櫒 API `2020`锛屾棤 NodePort | `clusters/test/observability/fluent-bit/daemonset-logs.yaml` |
| 5 | Fluent Bit events | Deployment | 閲囬泦 Kubernetes Events 鍒?OpenSearch | `fluent/fluent-bit:3.2.8` | 鏃?NodePort | `clusters/test/observability/fluent-bit/deployment-events.yaml` |
| 6 | OTel Collector | Deployment + Service | 鎺ユ敹 Beyla OTLP锛屾毚闇?Prometheus 鎸囨爣鍑哄彛 | `docker.1ms.run/otel/opentelemetry-collector-contrib:0.139.0` | ClusterIP `4317` OTLP gRPC锛宍4318` OTLP HTTP锛宍8889` metrics锛宍13133` health | `clusters/test/observability/otel-collector` |
| 7 | Beyla | DaemonSet | eBPF 鏃犱镜鍏ヤ笟鍔¤皟鐢?RED 鎸囨爣閲囬泦 | `docker.1ms.run/grafana/beyla:latest` | 鏃?NodePort锛屽彂閫佸埌 OTel Collector `4318` | `clusters/test/observability/beyla` |
| 8 | Falco | DaemonSet | runtime 瀹夊叏/杩涚▼浜嬩欢璇佹嵁 | `docker.1ms.run/falcosecurity/falco:0.43.1` | 瀹瑰櫒 health `8765`锛屾棤 NodePort | `clusters/test/observability/falco` |
| 9 | Pyroscope | Deployment + PVC + Service | profile 瀛樺偍鍜屾煡璇?UI/API | `docker.m.daocloud.io/grafana/pyroscope:1.15.1` | NodePort `32095` -> `4040` | `clusters/test/observability/pyroscope/pyroscope-deployment.yaml`锛宍pyroscope-service.yaml` |
| 10 | Alloy Pyroscope eBPF | DaemonSet | eBPF profiling 閲囬泦骞跺啓鍏?Pyroscope | `grafana/alloy:latest` | 瀹瑰櫒 HTTP `12345`锛屾棤 NodePort | `clusters/test/observability/pyroscope/alloy-daemonset.yaml` |
| 11 | RCA Backend/MCP | Deployment + PVC + Service | RCA API銆丮CP Server銆丒vidence Pack銆佸彧璇绘帓闅滃叆鍙?| `192.168.48.1:5002/auto-inspection-rca:20260429-rca-image` | NodePort `32180` Backend -> `18080`锛宍32181` MCP -> `18081` | `clusters/test/observability/auto-inspection-rca` |
| 12 | OpenSearch Snapshot | CronJob | 姣忔棩 OpenSearch snapshot | `python:3.12-slim` | 鏃?| `clusters/test/observability/opensearch-snapshot/cronjob.yaml` |
| 13 | RCA Alert Notify Dry-run | CronJob | 瀹氭椂璋冪敤 `/api/alerts/notify`锛屽綋鍓嶅彧 dry-run 涓嶉€氱煡 | `docker.1ms.run/curlimages/curl:8.10.1` | 鏃?| `clusters/test/observability/auto-inspection-rca/alert-notify-cronjob.yaml` |

## 褰撳墠澶栭儴璁块棶绔彛

| 鏈嶅姟 | URL |
|---|---|
| Argo CD HTTP | `http://192.168.48.200:32084` |
| Argo CD HTTPS | `https://192.168.48.200:32443` |
| Prometheus | `http://192.168.48.200:32092` |
| OpenSearch | `http://192.168.48.200:32090` |
| OpenSearch Dashboards | `http://192.168.48.200:32091` |
| MinIO API | `http://192.168.48.200:32093` |
| MinIO Console | `http://192.168.48.200:32094` |
| Pyroscope | `http://192.168.48.200:32095` |
| RCA Backend | `http://192.168.48.200:32180` |
| RCA MCP | `http://192.168.48.200:32181/mcp` |
| MySQL | `192.168.48.200:31326` |

## Observability GitOps 鍏ュ彛

Argo CD Application锛?
```text
apps/observability-application.yaml
```

褰撳墠鍚屾璺緞锛?
```text
clusters/test/observability
```

Kustomize 璧勬簮椤哄簭锛?
```text
fluent-bit
minio
opensearch
opensearch-dashboards
opensearch-snapshot
otel-collector
beyla
falco
pyroscope
auto-inspection-rca
```

浠庝笟鍔′緷璧栬搴︼紝鎺ㄨ崘瀹為檯妫€鏌ラ『搴忥細

```text
minio -> opensearch -> opensearch-dashboards -> fluent-bit -> otel-collector -> beyla -> falco -> pyroscope -> alloy -> auto-inspection-rca -> cronjobs
```

## 闀滃儚鍚屾娓呭崟

鍐呯綉姝ｅ紡閮ㄧ讲鍓嶏紝闇€瑕佺‘淇濅互涓嬮暅鍍忓凡缁忚兘浠庡唴缃?Registry 鎷夊彇锛屾垨鑺傜偣鍏峰鍙闂殑闀滃儚浠ｇ悊銆?
```text
192.168.48.1:5002/auto-inspection-rca:20260429-rca-image
docker.1ms.run/minio/minio:latest
opensearchproject/opensearch:2.19.5
opensearchproject/opensearch-dashboards:2
fluent/fluent-bit:3.2.8
docker.1ms.run/otel/opentelemetry-collector-contrib:0.139.0
docker.1ms.run/grafana/beyla:latest
docker.1ms.run/falcosecurity/falco:0.43.1
docker.m.daocloud.io/grafana/pyroscope:1.15.1
grafana/alloy:latest
python:3.12-slim
docker.1ms.run/curlimages/curl:8.10.1
docker-hub.tpo.xzoa.com/auto-inspection/busybox:1.36
docker-hub.tpo.xzoa.com/auto-inspection/mysql:8.0.26
docker-hub.tpo.xzoa.com/auto-inspection/prometheus:v3.11.0
docker-hub.tpo.xzoa.com/auto-inspection/prometheus-config-reloader:v0.90.1
docker-hub.tpo.xzoa.com/auto-inspection/kube-state-metrics:v2.18.0
docker-hub.tpo.xzoa.com/auto-inspection/node-exporter:v1.10.2
docker-hub.tpo.xzoa.com/auto-inspection/argocd:v3.3.8
docker-hub.tpo.xzoa.com/auto-inspection/dex:v2.43.0
docker-hub.tpo.xzoa.com/auto-inspection/redis:8.2.3-alpine
```

鐢熶骇寤鸿缁熶竴鏀瑰啓涓哄唴缃?Registry 鍦板潃锛屼緥濡傦細

```text
registry.infra.local/argoproj/argocd:v3.3.8
registry.infra.local/auto-inspection-rca:<tag>
```

## 閮ㄧ讲鍛戒护

### 鍐呯綉閮ㄧ讲鍖?
褰撳墠宸叉暣鐞嗛泦涓儴缃插寘锛?
```text
D:\code\auto_inspection\deploy\intranet-bundle
```

鍖呭惈锛?
```text
gitops-manifests/
services/gitlab/docker-compose.yml
services/registry/docker-compose.yml
images/auto-inspection-images.txt
scripts/pull-images-node206.sh
scripts/deploy-order.ps1
docs/
```

鍚屾鍒?node206 鍚庯紝鍙湪 node206 涓婇鎷夐暅鍍忥細

```bash
cd /opt/auto-inspection-intranet-bundle
bash scripts/pull-images-node206.sh
```

### MySQL

```powershell
cd D:\code\auto_inspection\worktrees\gitops-manifests\source\deploy\db\mysql-31326
kubectl apply -k .
```

### Prometheus

```powershell
cd D:\code\auto_inspection\worktrees\gitops-manifests\source\yaml\monitoring\prometheus
powershell -ExecutionPolicy Bypass -File .\install.ps1
```

#
## hostPath 数据目录约定

当前内网不再使用 NFS。所有平台持久化数据统一固定到 `xzyc115-19` 节点，工作负载直接使用 `hostPath` volume，不创建 PV/PVC；业务 Pod 通过 `nodeSelector` 固定到带有 `auto-inspection/storage-node=xzyc115-19` 标签的节点。当前集群实际承载节点为 `k8s-worker-2`。

目录约定：

```text
/data/auto-inspection/mysql-31326
/data/auto-inspection/prometheus
/data/auto-inspection/minio
/data/auto-inspection/opensearch
/data/auto-inspection/pyroscope
/data/auto-inspection/auto-inspection-rca
```

注意：如果集群中已经存在旧 NFS PV/PVC，需要先停应用、备份或迁移数据，然后删除旧 PVC/PV；新版本工作负载会直接挂载 hostPath，不再创建 PVC。

## Argo CD Applications

棣栨 bootstrap 鏃讹細

```powershell
cd D:\code\auto_inspection\worktrees\gitops-manifests
kubectl apply -f apps\argocd-bootstrap-project.yaml
kubectl apply -f apps\argocd-bootstrap-application.yaml
kubectl apply -f apps\argocd-core-project.yaml
kubectl apply -f apps\argocd-core-application.yaml
```

Observability 搴旂敤锛?
```powershell
kubectl apply -f apps\observability-project.yaml
kubectl apply -f apps\observability-application.yaml
```

### 鐩存帴 dry-run

```powershell
cd D:\code\auto_inspection\worktrees\gitops-manifests
kubectl kustomize clusters/test/observability
kubectl apply --dry-run=server -k clusters/test/observability
```

## 楠岃瘉娓呭崟

```powershell
kubectl get application -n argocd
kubectl get pods -n observability -o wide
kubectl get svc -n observability
kubectl get pods -n monitoring -o wide
kubectl get svc -n monitoring
```

鏍稿績鍋ュ悍妫€鏌ワ細

```powershell
Invoke-WebRequest -UseBasicParsing http://192.168.48.200:32180/api/health
Invoke-WebRequest -UseBasicParsing http://192.168.48.200:32180/api/health/details
Invoke-WebRequest -UseBasicParsing http://192.168.48.200:32090
Invoke-WebRequest -UseBasicParsing http://192.168.48.200:32092/-/ready
Invoke-WebRequest -UseBasicParsing http://192.168.48.200:32095/ready
```

RCA MCP锛?
```text
POST http://192.168.48.200:32181/mcp
method: initialize
method: tools/list
```

## 鏈€灏忓彲鐢ㄩ儴缃?
濡傛灉鍙兂鍏堣窇 RCA 鍩虹鑳藉姏锛屾渶灏忛泦鍚堟槸锛?
```text
GitLab
Registry
hostPath`r`nMySQL
Prometheus
Argo CD
OpenSearch
Fluent Bit logs/events
MinIO
RCA Backend/MCP
```

娣卞眰鎺掗殰鑳藉姏鍐嶈ˉ锛?
```text
OTel Collector
Beyla
Falco
Pyroscope
Alloy
```

## 娉ㄦ剰浜嬮」

- 鐢熶骇鍐呯綉涓嶈渚濊禆鍏綉闀滃儚婧愶紝鎵€鏈夐暅鍍忓簲鎻愬墠鍚屾鍒板唴缃?Registry銆?- Secret 涓嶅缓璁槑鏂囨斁 GitOps锛屽悗缁簲鏇挎崲涓?SOPS銆丼ealedSecret 鎴?ExternalSecret銆?- RCA Backend/MCP 浠ｇ爜搴旈€氳繃闀滃儚鍙戝竷锛屼笉鍐嶉€氳繃 PVC 瑕嗙洊 `/opt/rca`銆?- OpenSearch銆丮inIO銆丳yroscope銆丮ySQL 閮戒緷璧栨寔涔呭寲瀛樺偍锛岃縼绉诲墠鍏堢‘璁?hostPath/PVC 璺緞銆?- Argo CD 鍙悓姝?YAML锛屼笉璐熻矗鏋勫缓闀滃儚銆?




