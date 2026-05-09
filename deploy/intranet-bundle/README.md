# Auto Inspection RCA Intranet Bundle

杩欎釜鐩綍鐢ㄤ簬鎶婂唴缃戦儴缃查渶瑕佺殑鏂囦欢闆嗕腑鍒颁竴璧凤紝鏂逛究杩佺Щ鍒?node206 鎴栧叾浠栧唴缃戦儴缃叉満銆?
## Recommended Placement

```text
node206:
  GitLab
  hostPath data on xzyc115-19`r`n  image pre-pull / image relay

Kubernetes:
  Argo CD
  MySQL / Prometheus
  Observability / RCA
```

## GitLab

GitLab 闇€瑕佽ˉ杩涢儴缃叉竻鍗曪紝浣嗗缓璁綔涓哄钩鍙板墠缃湇鍔★紝涓嶅缓璁敱 Argo CD 鎵樼銆?
鍘熷洜鏄?GitLab 鏄?GitOps 鐨勬簮澶达紱濡傛灉璁?Argo CD 鍙嶈繃鏉ョ鐞?GitLab锛屾晠闅滄椂瀹规槗褰㈡垚婧愬ご鍜屾墽琛屽櫒浜掔浉渚濊禆鐨勯棶棰樸€?
褰撳墠 node206 宸茶繍琛?GitLab锛?
```text
image: docker-hub.tpo.xzoa.com/auto-inspection/gitlab-ce:latest
http:  192.168.48.206:8929
ssh:   192.168.48.206:2224
```

鍙傝€冮儴缃叉枃浠讹細

```text
services/gitlab/docker-compose.yml
```

鐢熶骇寤鸿鎶?GitLab 鏁版嵁鐩綍鍥哄畾鍒?`/srv/gitlab` 鎴栧悓绫绘寔涔呭寲鐩綍锛屽苟绾冲叆澶囦唤銆?
## Deploy Order

1. Prepare node206: GitLab and image pre-pull.
2. Prepare Kubernetes node xzyc115-19: containerd registry mirror and `/data/observability/<app>` hostPath data directories.
3. Deploy MySQL.
4. Deploy Prometheus.
5. Deploy Argo CD.
6. Deploy Observability/RCA GitOps Application.
7. Build and push RCA Backend/MCP image.
8. Verify Backend/MCP, OpenSearch, Prometheus, Pyroscope, Beyla, Falco, and Alloy.

## Key Files

```text
README.md
images/auto-inspection-images.txt
scripts/pull-images-node206.sh
scripts/deploy-order.ps1
services/gitlab/docker-compose.yml
services/registry/docker-compose.yml
gitops-manifests/
docs/
```

## Pull Images On node206

Copy this bundle to node206, then run:

```bash
cd /opt/auto-inspection-intranet-bundle
bash scripts/pull-images-node206.sh
```

You can also execute it remotely from the Windows management machine:

```powershell
ssh root@192.168.48.206 "cd /opt/auto-inspection-intranet-bundle && bash scripts/pull-images-node206.sh"
```

## Deploy Entry

On a management machine that has `kubectl` and `helm` configured:

```powershell
cd D:\code\auto_inspection\deploy\intranet-bundle
powershell -ExecutionPolicy Bypass -File .\scripts\deploy-order.ps1
```

The script is preview-only by default. Add `-Apply` only when you want to execute the commands.

## Notes

- Do not write GitLab root passwords, tokens, MinIO keys, or other secrets into this directory.
- Move Kubernetes Secrets to SOPS, SealedSecret, or ExternalSecret when the platform is promoted.
- GitLab is a prerequisite service and should not be managed by Argo CD.
- For production intranet use, mirror all images into the formal internal Registry or Harbor.


