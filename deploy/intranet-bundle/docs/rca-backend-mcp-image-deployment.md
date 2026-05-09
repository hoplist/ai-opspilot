# RCA Backend/MCP 闀滃儚鍖栭儴缃茶鏄?
## 鐩爣

灏?`auto-inspection-rca` 浠庤繍琛屾椂瀹夎渚濊禆鍜?NFS 婧愮爜鎸傝浇锛岃皟鏁翠负闀滃儚鍖栧彂甯冿細

- 浠ｇ爜鍜?Python 渚濊禆鍐呯疆鍒伴暅鍍忋€?- Backend 鍜?MCP 浣跨敤鍚屼竴涓暅鍍忥紝閫氳繃 `RCA_SERVICE` 鍖哄垎鍚姩妯″紡銆?- PVC 鍙繚鐣欒繍琛屾€佹暟鎹洰褰曪紝涓嶅啀瑕嗙洊 `/opt/rca` 婧愮爜鐩綍銆?
## 闀滃儚

褰撳墠闀滃儚锛?
```text
docker-hub.tpo.xzoa.com/auto-inspection/auto-inspection-rca:20260429-rca-image
```

鏈満 registry 瀹瑰櫒锛?
```text
registry-private
localhost:5002 -> registry:5000
```

鏋勫缓涓庢帹閫侊細

```powershell
docker build -t docker-hub.tpo.xzoa.com/auto-inspection/auto-inspection-rca:20260429-rca-image .
docker push docker-hub.tpo.xzoa.com/auto-inspection/auto-inspection-rca:20260429-rca-image
```

闆嗙兢鎷夊彇鍦板潃浣跨敤锛?
```text
docker-hub.tpo.xzoa.com/auto-inspection/auto-inspection-rca:20260429-rca-image
```

## 鑺傜偣 containerd 閰嶇疆

鐢变簬鏈満 registry 浣跨敤 HTTP锛岄渶瑕佸湪 Kubernetes 鑺傜偣閰嶇疆 containerd insecure registry銆?
鑺傜偣锛?
```text
192.168.48.200
192.168.48.201
192.168.48.202
```

閰嶇疆鏂囦欢锛?
```text
/etc/containerd/certs.d/192.168.48.1:5002/hosts.toml
```

鍐呭锛?
```toml
server = "http://192.168.48.1:5002"

[host."http://192.168.48.1:5002"]
  capabilities = ["pull", "resolve", "push"]
  skip_verify = true
```

閰嶇疆鍚庨噸鍚細

```bash
systemctl restart containerd
systemctl is-active containerd
```

## GitOps 閮ㄧ讲

Deployment 鍙樻洿锛?
- `backend` 瀹瑰櫒锛?  - `image: docker-hub.tpo.xzoa.com/auto-inspection/auto-inspection-rca:20260429-rca-image`
  - `RCA_SERVICE=backend`
- `mcp` 瀹瑰櫒锛?  - `image: docker-hub.tpo.xzoa.com/auto-inspection/auto-inspection-rca:20260429-rca-image`
  - `RCA_SERVICE=mcp`

PVC 鎸傝浇锛?
```text
/opt/rca/data    -> app-state PVC subPath data
/opt/rca/outputs -> app-state PVC subPath outputs
```

涓嶅啀鎸傝浇锛?
```text
/opt/rca
```

閬垮厤 NFS 婧愮爜瑕嗙洊闀滃儚鍐呬唬鐮併€?
## 楠岃瘉

闀滃儚鎷夊彇娴嬭瘯锛?
```powershell
kubectl run rca-image-pull-test -n observability `
  --image=docker-hub.tpo.xzoa.com/auto-inspection/auto-inspection-rca:20260429-rca-image `
  --restart=Never `
  --image-pull-policy=Always `
  --command -- /bin/sh -lc 'python -V && test -f /opt/rca/backend_server.py && echo ok'
```

鏈熸湜杈撳嚭锛?
```text
Python 3.12.x
ok
```

GitOps 楠岃瘉锛?
```powershell
kubectl kustomize clusters/test/observability
kubectl apply --dry-run=server -k clusters/test/observability
```

涓婄嚎鍚庨獙璇侊細

```powershell
kubectl rollout status deployment/auto-inspection-rca -n observability
curl http://192.168.48.200:32180/api/health
```

## 鍙戝竷瑙勮寖

姣忔 RCA 浠ｇ爜鍙樻洿寤鸿锛?
1. 鏈湴瀹屾垚浠ｇ爜楠岃瘉銆?2. 鏋勫缓鍞竴 tag 闀滃儚銆?3. 鎺ㄩ€佸埌 registry銆?4. 淇敼 GitOps Deployment 闀滃儚 tag銆?5. `kubectl apply --dry-run=server -k clusters/test/observability`銆?6. 鎻愪氦骞舵帹閫?`platform/gitops-manifests`銆?7. 绛?Argo CD 鍚屾骞堕獙璇?Backend/MCP銆?
涓嶅缓璁户缁湪杩愯 Pod 涓墜宸?`kubectl cp` 瑕嗙洊浠ｇ爜锛涘彧鍏佽浣滀负涓存椂璋冭瘯鎵嬫銆?