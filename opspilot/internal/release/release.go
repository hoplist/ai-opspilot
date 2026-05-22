package release

import (
	"context"
	"fmt"
	"strings"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/k8s"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
	prom "github.com/dualistpeng-netizen/ai-observability/opspilot/internal/prometheus"
)

type Service struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Deployment string `json:"deployment"`
	Container  string `json:"container,omitempty"`
	Source     string `json:"source"`
	Image      string `json:"image,omitempty"`
	GitLab     string `json:"gitlab_project,omitempty"`
	GitOps     string `json:"gitops_path,omitempty"`
	ArgoCD     string `json:"argocd_app,omitempty"`
}

type Registry struct {
	services    map[string]Service
	order       []string
	datasources Datasources
}

func NewRegistry(raw string) *Registry {
	return NewRegistryWithDatasources(raw, Datasources{})
}

func NewRegistryWithDatasources(raw string, datasources Datasources) *Registry {
	services := map[string]Service{}
	order := []string{}
	for _, item := range strings.Split(raw, ";") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		name, attrs, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		service := Service{Name: strings.TrimSpace(name)}
		for _, pair := range strings.Split(attrs, ",") {
			key, value, ok := strings.Cut(strings.TrimSpace(pair), ":")
			if !ok {
				key, value, ok = strings.Cut(strings.TrimSpace(pair), "=")
			}
			if !ok {
				continue
			}
			switch strings.TrimSpace(key) {
			case "namespace", "ns":
				service.Namespace = strings.TrimSpace(value)
			case "deployment", "deploy":
				service.Deployment = strings.TrimSpace(value)
			case "container":
				service.Container = strings.TrimSpace(value)
			case "source":
				service.Source = strings.TrimSpace(value)
			case "image":
				service.Image = strings.TrimSpace(value)
			case "gitlab", "gitlab_project":
				service.GitLab = strings.TrimSpace(value)
			case "gitops", "gitops_path":
				service.GitOps = strings.TrimSpace(value)
			case "argocd", "argocd_app":
				service.ArgoCD = strings.TrimSpace(value)
			}
		}
		if service.Name == "" || service.Namespace == "" || service.Deployment == "" {
			continue
		}
		services[service.Name] = service
		order = append(order, service.Name)
	}
	return &Registry{services: services, order: order, datasources: datasources}
}

func (r *Registry) Configured() bool {
	return len(r.services) > 0
}

func (r *Registry) Services() []string {
	return append([]string{}, r.order...)
}

func (r *Registry) Status(ctx context.Context, serviceName string, client *k8s.Client, promRegistry *prom.Registry, logClient *logsearch.Client) (map[string]any, []string, error) {
	service, ok := r.services[serviceName]
	if !ok {
		return nil, nil, fmt.Errorf("unknown release service: %s", serviceName)
	}
	warnings := []string{}
	gaps := []string{}
	evidence := map[string]any{}
	stage := "unknown"
	status := "unknown"

	deployment, err := client.DeploymentStatus(ctx, service.Namespace, service.Deployment)
	if err != nil {
		gaps = append(gaps, "kubernetes_deployment_missing")
		warnings = append(warnings, "deployment: "+err.Error())
	} else {
		evidence["kubernetes"] = deployment
		if deploymentImage := firstDeploymentImage(deployment); deploymentImage != "" {
			service.Image = deploymentImage
		}
		desired := intFromAny(deployment["desired_replicas"])
		ready := intFromAny(deployment["ready_replicas"])
		updated := intFromAny(deployment["updated_replicas"])
		switch {
		case desired > 0 && ready >= desired && updated >= desired:
			stage = "rollout"
			status = "healthy"
		case desired > 0 && ready < desired:
			stage = "rollout"
			status = "progressing"
		default:
			stage = "rollout"
			status = "unknown"
		}
		if selector, ok := deployment["selector_match_labels"].(map[string]any); ok {
			pods, err := client.ListPodsByLabels(ctx, service.Namespace, selector, 20)
			if err != nil {
				gaps = append(gaps, "kubernetes_pods_missing")
				warnings = append(warnings, "pods: "+err.Error())
			} else {
				evidence["pods"] = pods
				if pods.TotalCount == 0 {
					status = "degraded"
					gaps = append(gaps, "no_matching_pods")
				}
				addPodMetrics(ctx, evidence, promRegistry, service.Source, pods.Items, &warnings, &gaps)
				addPodLogs(ctx, evidence, client, logClient, service.Namespace, service.Name, pods.Items, &warnings, &gaps)
			}
		}
	}

	evidence["gitlab_pipeline"] = map[string]any{"status": "unknown"}
	evidence["buildkit"] = map[string]any{"status": "unknown"}
	evidence["registry"] = map[string]any{"status": "unknown", "image": service.Image}
	evidence["gitops"] = map[string]any{"status": "unknown", "path": service.GitOps}
	addGitLabEvidence(ctx, r.datasources, service, &evidence, &warnings, &gaps)
	if service.ArgoCD == "" {
		evidence["argocd"] = map[string]any{"sync_status": "Unknown", "health_status": "Unknown"}
		gaps = append(gaps, "argocd_app_mapping_missing")
	} else if argo, err := client.ArgoApplicationStatus(ctx, "argocd", service.ArgoCD); err != nil {
		evidence["argocd"] = map[string]any{"sync_status": "Unknown", "health_status": "Unknown", "app": service.ArgoCD}
		gaps = append(gaps, "argocd_datasource_missing")
		warnings = append(warnings, "argocd: "+err.Error())
	} else {
		evidence["argocd"] = argo
		if fmt.Sprint(argo["sync_status"]) == "OutOfSync" || fmt.Sprint(argo["health_status"]) == "Degraded" {
			status = "degraded"
			stage = "argocd"
		}
	}

	return map[string]any{
		"service":     service.Name,
		"environment": "test",
		"namespace":   service.Namespace,
		"deployment":  service.Deployment,
		"image":       service.Image,
		"stage":       stage,
		"status":      status,
		"evidence":    evidence,
		"gaps":        unique(gaps),
		"next_checks": nextChecks(status, gaps),
	}, warnings, nil
}

func (r *Registry) Jobs(ctx context.Context, serviceName string) (map[string]any, []string, error) {
	service, ok := r.services[serviceName]
	if !ok {
		return nil, nil, fmt.Errorf("unknown release service: %s", serviceName)
	}
	warnings := []string{}
	if service.GitLab == "" {
		return nil, warnings, fmt.Errorf("gitlab project mapping is missing for release service: %s", serviceName)
	}
	client := newGitLabClient(r.datasources.GitLabURL, r.datasources.GitLabToken)
	jobs, err := client.latestPipelineJobs(ctx, service.GitLab)
	if err != nil {
		return nil, warnings, err
	}
	jobs["service"] = service.Name
	return jobs, warnings, nil
}

func (r *Registry) JobTrace(ctx context.Context, serviceName string, jobID int64, jobName string, limitBytes, tailLines int) (map[string]any, []string, error) {
	service, ok := r.services[serviceName]
	if !ok {
		return nil, nil, fmt.Errorf("unknown release service: %s", serviceName)
	}
	warnings := []string{}
	if service.GitLab == "" {
		return nil, warnings, fmt.Errorf("gitlab project mapping is missing for release service: %s", serviceName)
	}
	client := newGitLabClient(r.datasources.GitLabURL, r.datasources.GitLabToken)
	selectedJobID := jobID
	selectedJobName := jobName
	if selectedJobID == 0 {
		jobs, err := client.latestPipelineJobs(ctx, service.GitLab)
		if err != nil {
			return nil, warnings, err
		}
		for _, item := range anySlice(jobs["items"]) {
			job, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if jobName == "" || fmt.Sprint(job["name"]) == jobName {
				selectedJobID = int64FromAny(job["id"])
				selectedJobName = fmt.Sprint(job["name"])
				break
			}
		}
	}
	if selectedJobID == 0 {
		return nil, warnings, fmt.Errorf("gitlab job not found for release service: %s", serviceName)
	}
	trace, err := client.jobTrace(ctx, service.GitLab, selectedJobID)
	if err != nil {
		return nil, warnings, err
	}
	trace, truncatedBytes := limitTailBytes(trace, limitBytes)
	trace, truncatedLines := limitTailLines(trace, tailLines)
	return map[string]any{
		"service":         service.Name,
		"project":         service.GitLab,
		"job_id":          selectedJobID,
		"job_name":        selectedJobName,
		"text":            trace,
		"bytes":           len(trace),
		"truncated":       truncatedBytes || truncatedLines,
		"truncated_bytes": truncatedBytes,
		"truncated_lines": truncatedLines,
	}, warnings, nil
}

func addGitLabEvidence(ctx context.Context, datasources Datasources, service Service, evidence *map[string]any, warnings, gaps *[]string) {
	client := newGitLabClient(datasources.GitLabURL, datasources.GitLabToken)
	if !client.configured() {
		*gaps = append(*gaps, "gitlab_datasource_missing", "registry_token_or_api_missing", "gitops_datasource_missing")
		return
	}
	if service.GitLab == "" {
		*gaps = append(*gaps, "gitlab_project_mapping_missing")
	} else if pipeline, err := client.latestPipeline(ctx, service.GitLab); err != nil {
		*gaps = append(*gaps, "gitlab_pipeline_missing")
		*warnings = append(*warnings, "gitlab pipeline: "+err.Error())
	} else {
		(*evidence)["gitlab_pipeline"] = pipeline
		(*evidence)["buildkit"] = map[string]any{"status": pipeline["status"], "source": "gitlab_pipeline"}
	}
	if service.GitLab == "" || service.Image == "" {
		*gaps = append(*gaps, "registry_project_or_image_missing")
	} else if registry, err := client.registryTag(ctx, service.GitLab, service.Image); err != nil {
		*gaps = append(*gaps, "registry_tag_missing")
		*warnings = append(*warnings, "registry: "+err.Error())
	} else {
		(*evidence)["registry"] = registry
	}
	gitopsProject := datasources.GitOpsProject
	if gitopsProject == "" {
		gitopsProject = service.GitLab
	}
	if service.GitOps == "" {
		*gaps = append(*gaps, "gitops_path_mapping_missing")
		return
	}
	raw, err := client.rawFile(ctx, gitopsProject, service.GitOps, datasources.GitOpsRef)
	if err != nil {
		*gaps = append(*gaps, "gitops_datasource_missing")
		*warnings = append(*warnings, "gitops: "+err.Error())
		return
	}
	desiredImage := desiredImageFromManifest(raw, service.Container)
	status := "unknown"
	if desiredImage == "" {
		status = "image_missing"
		*gaps = append(*gaps, "gitops_image_missing")
	} else if service.Image != "" && desiredImage == service.Image {
		status = "matches_cluster"
	} else {
		status = "differs_from_cluster"
	}
	(*evidence)["gitops"] = map[string]any{
		"status":        status,
		"project":       gitopsProject,
		"path":          service.GitOps,
		"ref":           gitopsRef(datasources.GitOpsRef),
		"desired_image": desiredImage,
	}
}

func gitopsRef(ref string) string {
	if ref == "" {
		return "main"
	}
	return ref
}

func firstDeploymentImage(deployment map[string]any) string {
	containers, _ := deployment["containers"].([]any)
	if len(containers) == 0 {
		return ""
	}
	first, _ := containers[0].(map[string]any)
	return fmt.Sprint(first["image"])
}

func addPodMetrics(ctx context.Context, evidence map[string]any, promRegistry *prom.Registry, source string, pods []map[string]any, warnings, gaps *[]string) {
	if promRegistry == nil || !promRegistry.Configured() {
		*gaps = append(*gaps, "prometheus_datasource_missing")
		return
	}
	items := []any{}
	for _, pod := range pods {
		name := fmt.Sprint(pod["name"])
		namespace := fmt.Sprint(pod["namespace"])
		metrics, _, err := promRegistry.SinglePodMetrics(ctx, source, namespace, name)
		if err != nil {
			*warnings = append(*warnings, "metrics "+name+": "+err.Error())
			continue
		}
		items = append(items, metrics)
	}
	if len(items) == 0 {
		*gaps = append(*gaps, "pod_metrics_missing")
	}
	evidence["metrics"] = items
}

func addPodLogs(ctx context.Context, evidence map[string]any, client *k8s.Client, logClient *logsearch.Client, namespace, service string, pods []map[string]any, warnings, gaps *[]string) {
	k8sLogs := []any{}
	for _, pod := range pods {
		name := fmt.Sprint(pod["name"])
		log, err := client.ReadPodLog(ctx, k8s.LogRequest{Namespace: namespace, Pod: name, TailLines: 80, SinceSeconds: 1800, LimitBytes: 128 * 1024})
		if err != nil {
			*warnings = append(*warnings, "logs "+name+": "+err.Error())
			continue
		}
		k8sLogs = append(k8sLogs, map[string]any{"pod": name, "bytes": len(log.Text), "truncated": log.Truncated})
	}
	if len(k8sLogs) == 0 {
		*gaps = append(*gaps, "kubernetes_logs_missing")
	}
	evidence["kubernetes_logs"] = k8sLogs
	if logClient == nil {
		*gaps = append(*gaps, "elk_logs_missing")
		return
	}
	search, err := logClient.Search(ctx, logsearch.SearchRequest{Namespace: namespace, Query: service, Limit: 1})
	if err != nil {
		*gaps = append(*gaps, "elk_logs_missing")
		*warnings = append(*warnings, "elk: "+err.Error())
		return
	}
	evidence["elk_logs"] = search
	if intFromAny(search["total"]) == 0 && intFromAny(search["item_count"]) == 0 {
		*gaps = append(*gaps, "elk_logs_empty")
	}
}

func nextChecks(status string, gaps []string) []string {
	checks := []string{}
	if status != "healthy" {
		checks = append(checks, "inspect Kubernetes Deployment conditions and matching Pods")
	}
	for _, gap := range gaps {
		switch gap {
		case "argocd_datasource_missing":
			checks = append(checks, "configure Argo CD read-only datasource")
		case "gitlab_datasource_missing":
			checks = append(checks, "configure GitLab read-only token for pipeline evidence")
		case "gitops_datasource_missing":
			checks = append(checks, "configure GitOps repository read-only evidence")
		}
	}
	return unique(checks)
}

func intFromAny(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func int64FromAny(value any) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func anySlice(value any) []any {
	if slice, ok := value.([]any); ok {
		return slice
	}
	if maps, ok := value.([]map[string]any); ok {
		out := make([]any, 0, len(maps))
		for _, item := range maps {
			out = append(out, item)
		}
		return out
	}
	return []any{}
}

func limitTailBytes(text string, limit int) (string, bool) {
	if limit <= 0 || len(text) <= limit {
		return text, false
	}
	return text[len(text)-limit:], true
}

func limitTailLines(text string, tail int) (string, bool) {
	if tail <= 0 {
		return text, false
	}
	lines := strings.Split(text, "\n")
	if len(lines) <= tail {
		return text, false
	}
	return strings.Join(lines[len(lines)-tail:], "\n"), true
}

func unique(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
