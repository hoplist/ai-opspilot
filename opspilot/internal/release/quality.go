package release

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/k8s"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/quality"
)

type QualitySettings struct {
	Enabled         bool
	RunnerImage     string
	ImagePullSecret string
	Ref             string
	TTLSeconds      int
	DeadlineSeconds int
}

type qualityConfigState struct {
	Config   quality.Config
	Raw      string
	Status   string
	Reason   string
	Warnings []string
}

func (r *Registry) QualityStatus(ctx context.Context, serviceName string, client *k8s.Client, settings QualitySettings) (map[string]any, []string, error) {
	service, ok := r.services[serviceName]
	if !ok {
		return nil, nil, fmt.Errorf("unknown release service: %s", serviceName)
	}
	settings = settings.defaults()
	config := r.qualityConfig(ctx, service, settings)
	if config.Status == "skipped" || config.Status == "invalid" {
		return map[string]any{
			"status":   config.Status,
			"reason":   config.Reason,
			"optional": true,
			"service":  service.Name,
		}, config.Warnings, nil
	}
	jobs, err := client.ListJobsByLabels(ctx, service.Namespace, map[string]string{
		"opspilot.io/quality-service": service.Name,
	}, 1)
	if err != nil {
		return map[string]any{
			"status":   "unavailable",
			"reason":   "quality_job_status_unavailable",
			"optional": true,
			"service":  service.Name,
		}, append(config.Warnings, "quality jobs: "+err.Error()), nil
	}
	if len(jobs.Items) == 0 {
		return map[string]any{
			"status":      "not_run",
			"reason":      "quality_job_not_found",
			"optional":    true,
			"configured":  true,
			"service":     service.Name,
			"namespace":   service.Namespace,
			"next_checks": []string{"run quality run service " + service.Name + " after Argo CD is Healthy"},
		}, config.Warnings, nil
	}
	job := jobs.Items[0]
	out := map[string]any{
		"status":     qualityStatusFromJob(job),
		"optional":   true,
		"configured": true,
		"service":    service.Name,
		"namespace":  service.Namespace,
		"job":        job,
	}
	log, err := client.ReadJobLog(ctx, service.Namespace, fmt.Sprint(job["name"]), 256*1024)
	if err != nil {
		return out, append(config.Warnings, "quality job logs: "+err.Error()), nil
	}
	out["logs"] = map[string]any{"bytes": len(log.Text), "truncated": log.Truncated}
	var report map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(log.Text)), &report); err != nil {
		out["logs_tail"] = limitTail(log.Text, 4096)
		return out, append(config.Warnings, "quality report parse: "+err.Error()), nil
	}
	out["report"] = report
	if status := fmt.Sprint(report["status"]); status != "" && status != "<nil>" {
		out["status"] = status
	}
	return out, config.Warnings, nil
}

func (r *Registry) RunQuality(ctx context.Context, serviceName, baseURLOverride string, client *k8s.Client, settings QualitySettings) (map[string]any, []string, error) {
	service, ok := r.services[serviceName]
	if !ok {
		return nil, nil, fmt.Errorf("unknown release service: %s", serviceName)
	}
	settings = settings.defaults()
	config := r.qualityConfig(ctx, service, settings)
	if config.Status == "skipped" || config.Status == "invalid" {
		return map[string]any{
			"status":   config.Status,
			"reason":   config.Reason,
			"optional": true,
			"service":  service.Name,
		}, config.Warnings, nil
	}
	if settings.RunnerImage == "" {
		return map[string]any{
			"status":   "unavailable",
			"reason":   "quality_runner_image_missing",
			"optional": true,
			"service":  service.Name,
		}, append(config.Warnings, "OPSPILOT_QUALITY_RUNNER_IMAGE is empty"), nil
	}
	cfg := config.Config
	if baseURLOverride != "" {
		cfg.BaseURL = baseURLOverride
	}
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return nil, config.Warnings, err
	}
	jobName := qualityJobName(service.Name)
	job := qualityJobManifest(service, jobName, settings, string(cfgJSON))
	created, err := client.CreateJob(ctx, service.Namespace, job)
	if err != nil {
		return map[string]any{
			"status":   "unavailable",
			"reason":   "quality_job_create_failed",
			"optional": true,
			"service":  service.Name,
		}, append(config.Warnings, "quality job create: "+err.Error()), nil
	}
	return map[string]any{
		"status":       "submitted",
		"optional":     true,
		"service":      service.Name,
		"namespace":    service.Namespace,
		"job_name":     jobName,
		"runner_image": settings.RunnerImage,
		"pull_secret":  settings.ImagePullSecret,
		"job":          k8s.JobSummary(created),
		"next_checks":  []string{"run quality status service " + service.Name + " to read the Job result"},
	}, config.Warnings, nil
}

func (r *Registry) qualityConfig(ctx context.Context, service Service, settings QualitySettings) qualityConfigState {
	if !settings.Enabled {
		return qualityConfigState{Status: "skipped", Reason: "quality_disabled"}
	}
	client := newGitLabClient(r.datasources.GitLabURL, r.datasources.GitLabToken)
	if !client.configured() {
		return qualityConfigState{Status: "skipped", Reason: "gitlab_datasource_missing"}
	}
	if service.GitLab == "" {
		return qualityConfigState{Status: "skipped", Reason: "gitlab_project_mapping_missing"}
	}
	ref := firstNonEmpty(settings.Ref, gitopsRef(r.datasources.GitOpsRef))
	raw, err := client.rawFile(ctx, service.GitLab, ".opspilot/quality.yaml", ref)
	if err != nil {
		reason := "quality_config_unavailable"
		if strings.Contains(err.Error(), "404") {
			return qualityConfigState{Status: "skipped", Reason: "quality_config_missing"}
		}
		return qualityConfigState{Status: "skipped", Reason: reason, Warnings: []string{"quality config: " + err.Error()}}
	}
	cfg, err := quality.ParseYAML(raw)
	if err != nil {
		return qualityConfigState{Status: "invalid", Reason: "quality_config_invalid", Raw: raw, Warnings: []string{"quality config parse: " + err.Error()}}
	}
	if !cfg.Enabled {
		return qualityConfigState{Config: cfg, Raw: raw, Status: "skipped", Reason: "quality_config_disabled"}
	}
	return qualityConfigState{Config: cfg, Raw: raw, Status: "configured"}
}

func (s QualitySettings) defaults() QualitySettings {
	if s.TTLSeconds <= 0 {
		s.TTLSeconds = 3600
	}
	if s.DeadlineSeconds <= 0 {
		s.DeadlineSeconds = 120
	}
	return s
}

func qualityJobManifest(service Service, jobName string, settings QualitySettings, cfgJSON string) map[string]any {
	labels := map[string]any{
		"app.kubernetes.io/name":          "opspilot-quality",
		"app.kubernetes.io/part-of":       "opspilot",
		"opspilot.io/managed":             "true",
		"opspilot.io/quality-service":     service.Name,
		"opspilot.io/quality-deployment":  service.Deployment,
		"opspilot.io/quality-trigger":     "manual",
		"opspilot.io/quality-runner-kind": "opspilot",
	}
	podSpec := map[string]any{
		"restartPolicy": "Never",
		"containers": []any{
			map[string]any{
				"name":            "quality-runner",
				"image":           settings.RunnerImage,
				"imagePullPolicy": "IfNotPresent",
				"command":         []any{"/usr/local/bin/opspilot", "quality", "runner"},
				"env": []any{
					map[string]any{"name": "OPSPILOT_QUALITY_CONFIG_JSON", "value": cfgJSON},
				},
				"resources": map[string]any{
					"requests": map[string]any{"cpu": "20m", "memory": "32Mi"},
					"limits":   map[string]any{"cpu": "200m", "memory": "128Mi"},
				},
			},
		},
	}
	if settings.ImagePullSecret != "" {
		podSpec["imagePullSecrets"] = []any{map[string]any{"name": settings.ImagePullSecret}}
	}
	return map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]any{
			"name":      jobName,
			"namespace": service.Namespace,
			"labels":    labels,
		},
		"spec": map[string]any{
			"ttlSecondsAfterFinished": settings.TTLSeconds,
			"activeDeadlineSeconds":   settings.DeadlineSeconds,
			"backoffLimit":            0,
			"template": map[string]any{
				"metadata": map[string]any{"labels": labels},
				"spec":     podSpec,
			},
		},
	}
}

func qualityStatusFromJob(job map[string]any) string {
	switch fmt.Sprint(job["state"]) {
	case "succeeded":
		return "completed"
	case "failed":
		return "failed"
	case "running":
		return "running"
	default:
		return "unknown"
	}
}

func qualityJobName(service string) string {
	base := sanitizeJobName(service)
	if len(base) > 40 {
		base = strings.Trim(base[:40], "-")
	}
	return fmt.Sprintf("%s-quality-%s", base, time.Now().UTC().Format("20060102150405"))
}

func sanitizeJobName(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "service"
	}
	return out
}

func limitTail(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[len(text)-limit:]
}
