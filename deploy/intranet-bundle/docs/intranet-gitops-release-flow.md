# 鍐呯綉 GitOps 鍙戝竷闂幆娴佺▼

## 鐩爣

鎶?RCA 骞冲彴鍙戝竷瑙勮寖鎴愪竴鏉″浐瀹氶摼璺細

```text
浠ｇ爜鍙樻洿 -> 鏋勫缓闀滃儚 -> 鎺ㄩ€佸唴缃?Registry -> 淇敼 GitOps 闀滃儚 tag -> GitLab 鎻愪氦 -> Argo CD 鑷姩鍚屾 -> 楠岃瘉 Backend/MCP
```

杩欐潯閾捐矾閫傚悎褰撳墠鍐呯綉鐜锛欸itLab銆丷egistry銆丄rgo CD銆並ubernetes銆丷CA Backend/MCP銆丱penSearch銆丳rometheus銆丳yroscope銆丗alco銆丅eyla 閮藉湪鍐呯綉銆?
## 瑙掕壊杈圭晫

| 缁勪欢 | 鑱岃矗 |
|---|---|
| GitLab | 淇濆瓨涓氬姟浠ｇ爜銆丷CA 浠ｇ爜銆丟itOps YAML銆佸彂甯冨巻鍙?|
| Registry | 淇濆瓨 RCA Backend/MCP 闀滃儚鍜岀涓夋柟缁勪欢闀滃儚 |
| Argo CD | 鍙礋璐ｆ妸 GitOps 浠撳簱澹版槑鐨?YAML 鍚屾鍒?Kubernetes |
| Kubernetes | 杩愯 RCA Backend/MCP 鍜岃娴嬬粍浠?|
| RCA Backend/MCP | 鎻愪緵鍙鎺掗殰 API銆丮CP 宸ュ叿銆丒vidence Pack |

Argo CD 涓嶆瀯寤洪暅鍍忥紝涔熶笉搴旇鐩存帴淇濆瓨婧愮爜銆傛簮鐮佽繘鍏ラ暅鍍忥紝杩愯鎬?PVC 鍙繚瀛?`data`銆乣outputs` 绛夌姸鎬佺洰褰曘€?
## 鍐呯綉鍓嶇疆鏉′欢

1. Kubernetes 鑺傜偣鍙互璁块棶鍐呯綉 Registry銆?2. containerd 宸查厤缃?HTTP/insecure registry锛?
```text
/etc/containerd/certs.d/192.168.48.1:5002/hosts.toml
```

3. GitOps 浠撳簱宸叉帴鍏?Argo CD銆?4. Argo CD 鍙互璁块棶鍐呯綉 GitLab銆?5. RCA Backend/MCP Deployment 浣跨敤闀滃儚鍚姩锛屼笉鍐嶉€氳繃 PVC 瑕嗙洊 `/opt/rca`銆?
## 鏍囧噯鎵嬪伐娴佺▼

### 1. 鐢熸垚闀滃儚 tag

寤鸿浣跨敤鏃ユ湡鏃堕棿鎴?Git commit锛?
```powershell
$tag = Get-Date -Format "yyyyMMdd-HHmmss"
```

### 2. 鏋勫缓骞舵帹閫侀暅鍍?
```powershell
cd D:\code\auto_inspection
docker build -t docker-hub.tpo.xzoa.com/auto-inspection/auto-inspection-rca:$tag .
docker push docker-hub.tpo.xzoa.com/auto-inspection/auto-inspection-rca:$tag
```

闆嗙兢鍐呬娇鐢ㄧ殑闀滃儚鍦板潃锛?
```text
docker-hub.tpo.xzoa.com/auto-inspection/auto-inspection-rca:<tag>
```

### 3. 淇敼 GitOps Deployment

闇€瑕佸悓姝ヤ慨鏀癸細

```text
D:\code\auto_inspection\worktrees\gitops-manifests\clusters\test\observability\auto-inspection-rca\deployment.yaml
D:\code\auto_inspection\worktrees\gitops-manifests\source\deploy\rca-service\deployment.yaml
D:\code\auto_inspection\worktrees\gitops-manifests\source\yaml\rca-service\deployment.yaml
```

鎶?backend 鍜?mcp 瀹瑰櫒闀滃儚鏀规垚锛?
```text
docker-hub.tpo.xzoa.com/auto-inspection/auto-inspection-rca:<tag>
```

### 4. GitOps 娓叉煋鍜?server dry-run

```powershell
cd D:\code\auto_inspection\worktrees\gitops-manifests
kubectl kustomize clusters/test/observability
kubectl apply --dry-run=server -k clusters/test/observability
```

### 5. 鎻愪氦骞舵帹閫?GitOps

```powershell
git add clusters/test/observability/auto-inspection-rca/deployment.yaml source/deploy/rca-service/deployment.yaml source/yaml/rca-service/deployment.yaml
git commit -m "Release RCA image <tag>"
git push origin main
```

### 6. 绛夊緟 Argo CD 鍚屾

```powershell
kubectl get application observability -n observability -o jsonpath='{.status.sync.revision} {.status.sync.status} {.status.health.status}'
```

鏈熸湜锛?
```text
<revision> Synced Healthy
```

### 7. 鍙戝竷鍚庨獙璇?
```powershell
kubectl rollout status deployment/auto-inspection-rca -n observability --timeout=240s
kubectl get pods -n observability -l app.kubernetes.io/name=auto-inspection-rca -o wide
```

妫€鏌ラ暅鍍忓拰鎸傝浇锛?
```powershell
kubectl get deploy auto-inspection-rca -n observability -o jsonpath='{.spec.template.spec.containers[*].image}'
```

```powershell
$pod = kubectl get pods -n observability -l app.kubernetes.io/name=auto-inspection-rca -o jsonpath='{.items[0].metadata.name}'
kubectl exec -n observability $pod -c backend -- sh -lc 'mount | grep /opt/rca || true; test -f /opt/rca/backend_server.py && echo backend-ok'
```

鍋ュ悍鎺ュ彛锛?
```powershell
Invoke-WebRequest -UseBasicParsing http://192.168.48.200:32180/api/health
Invoke-WebRequest -UseBasicParsing http://192.168.48.200:32180/api/health/details
```

MCP锛?
```text
POST http://192.168.48.200:32181/mcp initialize
POST http://192.168.48.200:32181/mcp tools/list
```

## 鑷姩鍖栬剼鏈?
宸叉彁渚涜剼鏈細

```text
D:\code\auto_inspection\scripts\release-rca-image-gitops.ps1
```

鍏堢湅鎵ц璁″垝锛屼笉鍋氫换浣曚慨鏀癸細

```powershell
powershell -ExecutionPolicy Bypass -File D:\code\auto_inspection\scripts\release-rca-image-gitops.ps1 -PlanOnly
```

鍙瀯寤洪暅鍍忋€佹帹閫侀暅鍍忋€佷慨鏀?GitOps銆佹湰鍦?dry-run锛屼笉鎻愪氦锛?
```powershell
powershell -ExecutionPolicy Bypass -File D:\code\auto_inspection\scripts\release-rca-image-gitops.ps1
```

瀹屾暣鍙戝竷锛氭瀯寤恒€佹帹閫併€佷慨鏀?GitOps銆佹彁浜ゃ€佹帹閫併€佺瓑寰?Argo銆侀獙璇侊細

```powershell
powershell -ExecutionPolicy Bypass -File D:\code\auto_inspection\scripts\release-rca-image-gitops.ps1 -Commit -Push -WaitArgo -Verify
```

鎸囧畾 tag锛?
```powershell
powershell -ExecutionPolicy Bypass -File D:\code\auto_inspection\scripts\release-rca-image-gitops.ps1 -Tag 20260429-prod01 -Commit -Push -WaitArgo -Verify
```

鍙噸鏂?patch GitOps锛屼笉閲嶆柊鏋勫缓闀滃儚锛?
```powershell
powershell -ExecutionPolicy Bypass -File D:\code\auto_inspection\scripts\release-rca-image-gitops.ps1 -Tag 20260429-prod01 -SkipDocker -Commit -Push -WaitArgo -Verify
```

## 鍚庣画鍙帴 CI

鍚庣画濡傛灉瑕佹洿鑷姩锛屽彲浠ュ湪 GitLab CI 涓媶鎴愪袱涓樁娈碉細

```text
build_image:
  - docker build
  - docker push

update_gitops:
  - 淇敼 GitOps 闀滃儚 tag
  - git commit
  - git push
```

CI token 鏉冮檺寤鸿鏈€灏忓寲锛?
- 鏋勫缓浠撳簱锛氬厑璁歌鍙栦唬鐮併€佹帹閫侀暅鍍忋€?- GitOps 浠撳簱锛氬彧鍏佽鍐欏叆鎸囧畾 GitOps 椤圭洰銆?- Argo CD锛氫笉闇€瑕?CI 鐩存帴鎿嶄綔锛岀户缁敱 Argo 鑷姩鍚屾銆?
## 鍥炴粴娴佺▼

鎺ㄨ崘鍥炴粴 GitOps commit锛岃€屼笉鏄墜鍔ㄦ敼绾夸笂 Deployment锛?
```powershell
cd D:\code\auto_inspection\worktrees\gitops-manifests
git log --oneline -- clusters/test/observability/auto-inspection-rca/deployment.yaml
```

鎵惧埌涓婁竴涓ǔ瀹氶暅鍍?tag 鍚庯紝淇敼 Deployment 闀滃儚 tag锛岄噸鏂版彁浜ゆ帹閫併€侫rgo CD 浼氳嚜鍔ㄦ妸闆嗙兢鍚屾鍥炶鐗堟湰銆?
## 鎺掗殰鍏ュ彛

闀滃儚鎷夊彇澶辫触锛?
```powershell
kubectl describe pod -n observability <pod>
kubectl get events -n observability --sort-by=.lastTimestamp
```

Argo 娌″悓姝ワ細

```powershell
kubectl get application observability -n observability -o yaml
```

Deployment 娌℃洿鏂帮細

```powershell
kubectl rollout history deployment/auto-inspection-rca -n observability
kubectl describe deployment auto-inspection-rca -n observability
```

Backend/MCP 涓嶅仴搴凤細

```powershell
kubectl logs -n observability deployment/auto-inspection-rca -c backend --tail=100
kubectl logs -n observability deployment/auto-inspection-rca -c mcp --tail=100
```
