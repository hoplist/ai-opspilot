# 2026-05-07 鍐呯綉閮ㄧ讲缁熶竴 namespace

## 鑳屾櫙

鍐呯綉鐜涓?auto-inspection 浣滀负涓€涓暣浣撻」鐩儴缃诧紝闆嗙兢閲岃繕浼氬瓨鍦ㄥ叾浠栨祴璇曢」鐩€備负浜嗗噺灏戜氦鍙夊奖鍝嶅拰鎺掓煡鎴愭湰锛屽唴缃戦儴缃插寘涓殑 Kubernetes 宸ヤ綔璐熻浇缁熶竴鏀惧叆鍚屼竴涓?namespace銆?
## 鍙樻洿

浠呬慨鏀?node206 鍐呯綉閮ㄧ讲鍖咃紝涓嶅悓姝ュ埌 GitLab/GitOps 杩滅銆?
缁熶竴 namespace锛?
```text
auto-inspection
```

宸叉浛鎹㈢殑鏃?namespace锛?
```text
argocd
db
database
monitoring
observability
```

瑕嗙洊鑼冨洿锛?
- Argo CD bootstrap/core/Application manifests
- MySQL
- Prometheus Helm values/install script
- OpenSearch銆丱penSearch Dashboards銆丮inIO
- Fluent Bit銆丱Tel Collector銆丅eyla銆丗alco
- Pyroscope銆丄lloy
- RCA Backend/MCP
- 鐩稿叧鑴氭湰鍜岄儴缃叉枃妗ｄ腑鐨?`kubectl -n` 鏌ヨ鍛戒护

## 楠岃瘉

宸插湪鏈湴瀵?node206 閮ㄧ讲鍖呮墽琛屾覆鏌撴鏌ワ細

```text
kubectl kustomize gitops-manifests/source/deploy/db/mysql-31326
kubectl kustomize gitops-manifests/clusters/test/observability
kubectl kustomize gitops-manifests/clusters/test/argocd-core
helm template auto-prometheus prometheus-community/prometheus --version 28.15.0 --namespace observability -f gitops-manifests/source/yaml/monitoring/prometheus/values.yaml
```

娓叉煋缁撴灉涓殑 namespace 宸茬粺涓€涓?`auto-inspection`銆?
## 浣跨敤鎻愰啋

閮ㄧ讲鍓嶅厛鍒涘缓鎴栫‘璁?namespace锛?
```bash
kubectl create namespace observability --dry-run=client -o yaml | kubectl apply -f -
```

鍚庣画鏌ョ湅缁熶竴浣跨敤锛?
```bash
kubectl get pods -n observability -o wide
kubectl get svc -n observability
```
