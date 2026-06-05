package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/k8s"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/nodeagent"
	prom "github.com/dualistpeng-netizen/ai-observability/opspilot/internal/prometheus"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/skillregistry"
)

func buildCapabilities(ctx context.Context, client *k8s.Client, promRegistry *prom.Registry, agentRegistry *nodeagent.Registry, logClient *logsearch.Client, releaseRegistry *release.Registry, qualitySettings release.QualitySettings) (map[string]any, []string, error) {
	warnings := []string{}
	capabilities := []map[string]any{}
	availableEvidence := []string{}
	missingEvidence := []string{}

	add := func(item map[string]any) {
		capabilities = append(capabilities, item)
		if boolMapValue(item, "available") {
			availableEvidence = append(availableEvidence, stringSliceValue(item["available_evidence"])...)
			return
		}
		missingEvidence = append(missingEvidence, stringSliceValue(item["missing_evidence"])...)
	}

	k8sReady := false
	k8sDetails := client.Health()
	if _, err := client.ListPods(ctx, "", "", "", 1); err != nil {
		warnings = append(warnings, "kubernetes: "+err.Error())
		k8sDetails["error"] = err.Error()
	} else {
		k8sReady = true
	}
	add(capabilityItem("kubernetes_api", "Kubernetes API", "core", true, k8sReady,
		[]string{"Pod 状态", "Pod 事件", "Deployment/ReplicaSet/Node 基础信息"},
		[]string{"Kubernetes API 未接入或 RBAC 不足，无法读取 Pod 状态、事件和工作负载。"},
		"Kubernetes API is the minimum Pod troubleshooting source.", k8sDetails))
	add(capabilityItem("pod_logs", "Kubernetes Pod Logs", "core", true, k8sReady,
		[]string{"当前容器日志", "短窗口 kubernetes logs"},
		[]string{"pods/log 权限不可用或 Kubernetes API 不可用，无法读取当前容器日志。"},
		"Pod logs are read on demand through the Kubernetes pods/log API.", map[string]any{"depends_on": "kubernetes_api"}))

	promHealth := promRegistry.Health(ctx)
	promReady := boolMapValue(promHealth, "configured") && boolMapValue(promHealth, "ready")
	if boolMapValue(promHealth, "configured") && !promReady {
		warnings = append(warnings, "prometheus: configured but not ready")
	}
	add(capabilityItem("prometheus_metrics", "Prometheus Metrics", "metrics", boolMapValue(promHealth, "configured"), promReady,
		[]string{"CPU/Memory 当前值和趋势", "Top Pod", "节点资源", "磁盘挂载点"},
		[]string{"Prometheus 未接入或不可达，无法判断 CPU/Memory 趋势、Top Pod、节点资源和磁盘挂载点。"},
		"Prometheus enriches Pod and cluster checks; Kubernetes-only inspection can still continue.", promHealth))

	logHealth := logClient.Health(ctx)
	logReady := boolMapValue(logHealth, "configured") && boolMapValue(logHealth, "ready")
	if boolMapValue(logHealth, "configured") && !logReady {
		warnings = append(warnings, "logsearch: configured but not ready")
	}
	add(capabilityItem("elk_logs", "ELK/OpenSearch Logs", "logs", boolMapValue(logHealth, "configured"), logReady,
		[]string{"历史日志", "已轮转日志", "跨 Pod 日志搜索"},
		[]string{"ELK/OpenSearch 未接入或不可达，无法查询历史日志或已轮转日志。"},
		"Log search is optional; Kubernetes current logs remain available when pod_logs is ready.", logHealth))

	add(capabilityItem("service_logs", "Service Log Correlation", "logs", logClient.ServiceLogsConfigured(), logReady && logClient.ServiceLogsConfigured(),
		[]string{"按接口 URI 查询服务日志", "service-only 初步排查"},
		[]string{"服务日志索引未配置，无法按接口 URI 做服务侧日志关联。"},
		"Service log correlation can work without APISIX when service logs are indexed.", logHealth))
	add(capabilityItem("apisix_logs", "APISIX Access Logs", "gateway", logClient.APISIXConfigured(), logReady && logClient.APISIXConfigured(),
		[]string{"按域名/接口查询网关 access 日志", "网关到服务的链路证据"},
		[]string{"APISIX 日志未配置，无法通过域名或接口做网关链路关联；可继续做 service-only 排查。"},
		"APISIX evidence is optional and should not block Pod-first investigation.", logHealth))

	agentHealth := agentRegistry.Health(ctx)
	agentReady := boolMapValue(agentHealth, "configured") && boolMapValue(agentHealth, "ready")
	if boolMapValue(agentHealth, "configured") && !agentReady {
		warnings = append(warnings, "node agent: configured but not ready")
	}
	add(capabilityItem("docker_agent", "Docker Node Agent", "host", boolMapValue(agentHealth, "configured"), agentReady,
		[]string{"node206 Docker 容器状态", "Docker inspect/stats/logs"},
		[]string{"Docker node-agent 未接入或不可达，无法查询宿主机 Docker 容器。"},
		"Docker agent is only needed for non-Kubernetes containers or host-side checks.", agentHealth))

	releaseHealth := releaseRegistry.Health()
	add(capabilityItem("release_mapping", "Release Service Mapping", "release", boolMapValue(releaseHealth, "configured"), boolMapValue(releaseHealth, "configured"),
		[]string{"服务到 namespace/deployment/image 的映射", "发布状态入口"},
		[]string{"发布服务映射未配置，无法按服务名查询发布链路；仍可按 namespace/pod 排查。"},
		"Release mapping is required for service-level release evidence, not for raw Pod inspection.", releaseHealth))
	add(capabilityItem("gitlab_release", "GitLab/Registry/GitOps Evidence", "release", boolMapValue(releaseHealth, "gitlab_configured"), boolMapValue(releaseHealth, "gitlab_configured"),
		[]string{"GitLab pipeline", "BuildKit job", "Registry tag", "GitOps desired image"},
		[]string{"GitLab token/GitOps 数据源未配置，无法查询 pipeline、BuildKit、Registry 或 GitOps 证据。"},
		"GitLab evidence is required for release and rollback evidence chains.", releaseHealth))
	qualityReady := qualitySettings.Enabled && qualitySettings.RunnerImage != ""
	add(capabilityItem("quality_checks", "Optional API Quality Checks", "release", qualitySettings.Enabled, qualityReady,
		[]string{"Post-deploy API smoke checks", "response-time evidence", "quality Job logs"},
		[]string{"Optional quality runner is not configured; release and Kubernetes inspection continue without API quality evidence."},
		"Quality checks are optional release evidence and do not block core troubleshooting.", map[string]any{
			"enabled":      qualitySettings.Enabled,
			"runner_image": qualitySettings.RunnerImage,
			"pull_secret":  qualitySettings.ImagePullSecret,
			"job_ttl":      qualitySettings.TTLSeconds,
			"deadline":     qualitySettings.DeadlineSeconds,
		}))

	argoDetails := map[string]any{"namespace": "argocd"}
	argoReady := false
	if _, err := client.ListArgoApplications(ctx, "argocd", 1); err != nil {
		warnings = append(warnings, "argocd: "+err.Error())
		argoDetails["error"] = err.Error()
	} else {
		argoReady = true
	}
	add(capabilityItem("argocd", "Argo CD Applications", "release", true, argoReady,
		[]string{"Argo CD sync/health 状态", "GitOps reconciliation 证据"},
		[]string{"Argo CD Application CRD/RBAC 不可用，无法读取 sync/health；Kubernetes Pod 排查仍可继续。"},
		"Argo CD is optional unless release evidence is requested.", argoDetails))

	skillCatalog, skillWarnings := skillregistry.RegistryFromEnv("", true)
	warnings = append(warnings, skillWarnings...)
	add(capabilityItem("skills_registry", "OpsPilot Skills Registry", "ai", true, skillCatalog.IntegratedCount > 0,
		[]string{"AI skill routing metadata", "integrated skill-to-command mapping", "safe follow-up instructions"},
		[]string{"Skills registry is empty, so AI cannot route evidence to domain-specific troubleshooting rules."},
		"Skills registry maps Codex skills into deterministic OpsPilot evidence and command surfaces.", skillregistry.Summary(skillCatalog)))

	return map[string]any{
		"ready":              k8sReady,
		"capabilities":       capabilities,
		"available_evidence": uniqueStringsCore(availableEvidence),
		"missing_evidence":   uniqueStringsCore(missingEvidence),
		"skills":             skillregistry.Summary(skillCatalog),
		"summary": map[string]any{
			"core_ready":       k8sReady,
			"capability_count": len(capabilities),
			"available_count":  countAvailable(capabilities),
			"missing_count":    len(capabilities) - countAvailable(capabilities),
		},
	}, uniqueStringsCore(warnings), nil
}

func capabilityItem(name, label, category string, configured, ready bool, availableEvidence, missingEvidence []string, message string, details map[string]any) map[string]any {
	available := configured && ready
	status := "missing"
	switch {
	case available:
		status = "ready"
	case configured:
		status = "not_ready"
	}
	return map[string]any{
		"name":               name,
		"label":              label,
		"category":           category,
		"configured":         configured,
		"ready":              ready,
		"available":          available,
		"status":             status,
		"available_evidence": availableEvidence,
		"missing_evidence":   missingEvidence,
		"message":            message,
		"details":            details,
	}
}

func sourceQuery(r *http.Request) string {
	return r.URL.Query().Get("source")
}

func hostQuery(r *http.Request) string {
	return r.URL.Query().Get("host")
}

func addPodMetrics(ctx context.Context, promRegistry *prom.Registry, source string, target map[string]any, namespace, pod string) {
	if !promRegistry.Configured() {
		appendWarning(target, "prometheus: not configured")
		return
	}
	metrics, _, err := promRegistry.SinglePodMetrics(ctx, source, namespace, pod)
	if err != nil {
		appendWarning(target, "prometheus: "+err.Error())
		return
	}
	evidence, ok := target["evidence"].(map[string]any)
	if !ok {
		evidence = map[string]any{}
		target["evidence"] = evidence
	}
	evidence["metrics"] = metrics
}

func appendWarning(target map[string]any, warning string) {
	warnings := []string{}
	for _, raw := range toSlice(target["warnings"]) {
		warnings = append(warnings, fmt.Sprint(raw))
	}
	warnings = append(warnings, warning)
	target["warnings"] = warnings
}

func toSlice(value any) []any {
	if slice, ok := value.([]any); ok {
		return slice
	}
	return []any{}
}

func boolMapValue(values map[string]any, key string) bool {
	value, ok := values[key].(bool)
	return ok && value
}

func stringSliceValue(value any) []string {
	out := []string{}
	switch typed := value.(type) {
	case []string:
		out = append(out, typed...)
	case []any:
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				out = append(out, text)
			}
		}
	}
	return out
}

func uniqueStringsCore(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func countAvailable(items []map[string]any) int {
	count := 0
	for _, item := range items {
		if boolMapValue(item, "available") {
			count++
		}
	}
	return count
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func boolEnv(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
