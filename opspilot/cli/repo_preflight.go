package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (o repoLayoutOptions) defaults() repoLayoutOptions {
	if o.CIPath == "" {
		o.CIPath = ".gitlab-ci.yml"
	}
	if o.DeployPath == "" {
		o.DeployPath = filepath.Join("deploy", "k8s")
	}
	if o.NamespacePath == "" {
		o.NamespacePath = filepath.Join(o.DeployPath, "namespace.yaml")
	}
	if o.LimitRangePath == "" {
		o.LimitRangePath = filepath.Join(o.DeployPath, "limitrange.yaml")
	}
	if o.ResourceQuotaPath == "" {
		o.ResourceQuotaPath = filepath.Join(o.DeployPath, "resourcequota.yaml")
	}
	if o.ServiceAccountPath == "" {
		o.ServiceAccountPath = filepath.Join(o.DeployPath, "serviceaccount.yaml")
	}
	if o.QualityPath == "" {
		o.QualityPath = filepath.Join(".opspilot", "quality.yaml")
	}
	return o
}

func repoPreflight(project, catalogPath string, layout repoLayoutOptions) (repoPreflightResult, error) {
	layout = layout.defaults()
	detected, err := detectOnboardRepository(project, catalogPath)
	if err != nil {
		return repoPreflightResult{}, err
	}
	cfg := detected.Config
	if layout.Namespace != "" {
		cfg.Namespace = layout.Namespace
		cfg.NamespaceSrc = "preflight-override"
	}
	if err := cfg.defaults(); err != nil {
		return repoPreflightResult{}, err
	}
	items := []repoPolicyItem{
		checkRepoDockerfile(cfg),
		checkRepoCI(cfg, layout.CIPath),
		checkRepoFile("namespace", layout.NamespacePath, "generate deploy/k8s/namespace.yaml from ownership"),
		checkRepoFile("limitrange", layout.LimitRangePath, "generate deploy/k8s/limitrange.yaml for namespace defaults"),
		checkRepoFile("resourcequota", layout.ResourceQuotaPath, "generate deploy/k8s/resourcequota.yaml for namespace quota"),
		checkRepoFile("serviceaccount", layout.ServiceAccountPath, "generate deploy/k8s/serviceaccount.yaml for image pull access"),
		checkRepoDeployment(cfg, filepath.Join(layout.DeployPath, "deployment.yaml")),
		checkRepoFile("service", filepath.Join(layout.DeployPath, "service.yaml"), "generate deploy/k8s/service.yaml"),
		checkRepoFile("kustomization", filepath.Join(layout.DeployPath, "kustomization.yaml"), "generate deploy/k8s/kustomization.yaml"),
		checkRepoQuality(layout.QualityPath),
		checkRepoHealth(cfg),
	}
	items = append(items, checkRepoMiddleware(cfg)...)
	items = append(items, checkRepoStorage(cfg)...)
	result := repoPreflightResult{
		Service:   cfg.Name,
		Project:   cfg.GitLabProject,
		Language:  cfg.Language,
		Namespace: cfg.Namespace,
		Ready:     true,
		Items:     items,
		Config:    cfg,
	}
	for _, item := range items {
		if item.Status == "pass" {
			continue
		}
		result.Next = append(result.Next, item.Action)
		if item.Fixable {
			result.Autofixable = true
		}
		if item.Level == "blocker" {
			result.Ready = false
			result.Gaps = append(result.Gaps, item.Name)
		}
	}
	return result, nil
}

func checkRepoDockerfile(cfg onboardServiceConfig) repoPolicyItem {
	path := cfg.DockerPath
	if path == "" {
		path = "Dockerfile"
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return repoPolicyItem{Name: "dockerfile", Path: path, Status: "fail", Level: "blocker", Message: "missing", Fixable: true, Action: "run repo autofix --write to generate Dockerfile"}
		}
		return repoPolicyItem{Name: "dockerfile", Path: path, Status: "fail", Level: "blocker", Message: err.Error(), Fixable: false, Action: "fix Dockerfile filesystem error"}
	}
	text := string(body)
	issues := []string{}
	blocker := false
	if hasLatestBaseImage(text) {
		issues = append(issues, "base image uses latest tag")
		blocker = true
	}
	if containsAny(text, []string{"localhost", "127.0.0.1", "host.docker.internal"}) {
		issues = append(issues, "local-only endpoint found")
		blocker = true
	}
	if containsAny(text, []string{"COPY ../", "ADD ../"}) {
		issues = append(issues, "copies files outside build context")
		blocker = true
	}
	if containsDangerousPipe(text) {
		issues = append(issues, "shell download pipe detected")
		blocker = true
	}
	if !strings.Contains(strings.ToUpper(text), "EXPOSE ") {
		issues = append(issues, "EXPOSE missing")
	}
	if len(issues) == 0 {
		return repoPolicyItem{Name: "dockerfile", Path: path, Status: "pass", Level: "info", Message: "Dockerfile present"}
	}
	status := "warn"
	level := "warning"
	if blocker {
		status = "fail"
		level = "blocker"
	}
	return repoPolicyItem{Name: "dockerfile", Path: path, Status: status, Level: level, Message: strings.Join(issues, "; "), Fixable: true, Action: "run repo autofix --write --force to replace Dockerfile with platform template"}
}

func checkRepoCI(cfg onboardServiceConfig, path string) repoPolicyItem {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return repoPolicyItem{Name: "gitlab_ci", Path: path, Status: "fail", Level: "blocker", Message: "missing", Fixable: true, Action: "run repo autofix --write to generate platform CI include"}
		}
		return repoPolicyItem{Name: "gitlab_ci", Path: path, Status: "fail", Level: "blocker", Message: err.Error(), Fixable: false, Action: "fix .gitlab-ci.yml filesystem error"}
	}
	text := string(body)
	template := "/ci/templates/buildkit-gitops." + cfg.Language + ".yml"
	if strings.Contains(text, template) {
		return repoPolicyItem{Name: "gitlab_ci", Path: path, Status: "pass", Level: "info", Message: "platform template include detected"}
	}
	if strings.Contains(text, "buildctl-daemonless.sh") || strings.Contains(text, "BUILDKIT_IMAGE") {
		if cfg.GitLabProject == "platform/opspilot" {
			return repoPolicyItem{Name: "gitlab_ci", Path: path, Status: "pass", Level: "info", Message: "platform repository owns direct BuildKit CI"}
		}
		return repoPolicyItem{Name: "gitlab_ci", Path: path, Status: "warn", Level: "warning", Message: "direct BuildKit CI detected; platform include is preferred", Fixable: true, Action: "run repo autofix --write --force to replace CI with platform include"}
	}
	return repoPolicyItem{Name: "gitlab_ci", Path: path, Status: "fail", Level: "blocker", Message: "platform BuildKit/GitOps template not detected", Fixable: true, Action: "run repo autofix --write --force to replace CI with platform include"}
}

func checkRepoFile(name, path, action string) repoPolicyItem {
	if _, err := os.Stat(path); err == nil {
		return repoPolicyItem{Name: name, Path: path, Status: "pass", Level: "info", Message: "present"}
	} else if err != nil && !os.IsNotExist(err) {
		return repoPolicyItem{Name: name, Path: path, Status: "fail", Level: "blocker", Message: err.Error(), Fixable: false, Action: "fix manifest filesystem error"}
	}
	return repoPolicyItem{Name: name, Path: path, Status: "fail", Level: "blocker", Message: "missing", Fixable: true, Action: action}
}

func checkRepoQuality(path string) repoPolicyItem {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return repoPolicyItem{Name: "quality_config", Path: path, Status: "warn", Level: "warning", Message: "missing optional API quality checks", Fixable: true, Action: "run repo autofix --write to generate optional .opspilot/quality.yaml"}
		}
		return repoPolicyItem{Name: "quality_config", Path: path, Status: "warn", Level: "warning", Message: err.Error(), Fixable: false, Action: "fix quality config filesystem error"}
	}
	text := string(body)
	if strings.Contains(text, "enabled: false") {
		return repoPolicyItem{Name: "quality_config", Path: path, Status: "warn", Level: "warning", Message: "quality checks are explicitly disabled", Fixable: true, Action: "enable quality checks when service has a stable health endpoint"}
	}
	if !strings.Contains(text, "endpoints:") || !strings.Contains(text, "expectStatus:") {
		return repoPolicyItem{Name: "quality_config", Path: path, Status: "warn", Level: "warning", Message: "quality config has no endpoint assertions", Fixable: true, Action: "run repo autofix --write --force to regenerate optional quality config"}
	}
	return repoPolicyItem{Name: "quality_config", Path: path, Status: "pass", Level: "info", Message: "optional API quality checks configured"}
}

func checkRepoDeployment(cfg onboardServiceConfig, path string) repoPolicyItem {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return repoPolicyItem{Name: "deployment", Path: path, Status: "fail", Level: "blocker", Message: "missing", Fixable: true, Action: "generate deploy/k8s/deployment.yaml"}
		}
		return repoPolicyItem{Name: "deployment", Path: path, Status: "fail", Level: "blocker", Message: err.Error(), Fixable: false, Action: "fix deployment filesystem error"}
	}
	text := string(body)
	issues := []string{}
	blocker := false
	if containsAny(text, []string{"hostNetwork: true", "hostPID: true", "privileged: true"}) {
		issues = append(issues, "unsafe pod security field")
		blocker = true
	}
	if storageIssues, storageBlocker := deploymentStoragePolicyIssues(text, cfg); len(storageIssues) > 0 {
		issues = append(issues, storageIssues...)
		if storageBlocker {
			blocker = true
		}
	}
	if !strings.Contains(text, "readinessProbe:") {
		issues = append(issues, "readinessProbe missing")
		blocker = true
	}
	if !strings.Contains(text, "livenessProbe:") {
		issues = append(issues, "livenessProbe missing")
		blocker = true
	}
	if !hasDeploymentResources(text) {
		issues = append(issues, "CPU/memory requests and limits missing")
		blocker = true
	}
	if !strings.Contains(text, "namespace: "+cfg.Namespace) {
		issues = append(issues, "namespace does not match inferred namespace "+cfg.Namespace)
		blocker = true
	}
	if len(issues) == 0 {
		return repoPolicyItem{Name: "deployment", Path: path, Status: "pass", Level: "info", Message: "Deployment manifest present"}
	}
	status := "warn"
	level := "warning"
	if blocker {
		status = "fail"
		level = "blocker"
	}
	return repoPolicyItem{Name: "deployment", Path: path, Status: status, Level: level, Message: strings.Join(issues, "; "), Fixable: true, Action: "run repo autofix --write --force to regenerate Deployment manifest"}
}

func deploymentStoragePolicyIssues(text string, cfg onboardServiceConfig) ([]string, bool) {
	issues := []string{}
	blocker := false
	hostPaths := deploymentHostPathValues(text)
	if strings.Contains(text, "hostPath:") && len(hostPaths) == 0 {
		issues = append(issues, "hostPath path missing")
		blocker = true
	}
	platformHostPathCount := 0
	for _, hostPath := range hostPaths {
		if !isPlatformHostPath(hostPath) {
			issues = append(issues, "hostPath path "+hostPath+" is outside "+defaultHostPathRoot)
			blocker = true
			continue
		}
		platformHostPathCount++
	}
	if platformHostPathCount > 0 && !hasStorageManagedAnnotation(text) {
		issues = append(issues, "platform hostPath metadata annotation missing")
	}
	if hasStorageManagedAnnotation(text) && len(cfg.Storage) == 0 {
		issues = append(issues, "storage metadata present but no storage intent detected")
	}
	if deploymentHasUnboundedEmptyDir(text) {
		issues = append(issues, "emptyDir volume must include sizeLimit")
		blocker = true
	}
	return issues, blocker
}

func deploymentHostPathValues(text string) []string {
	paths := []string{}
	inHostPath := false
	hostIndent := 0
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := countLeadingSpaces(line)
		if inHostPath && indent <= hostIndent {
			inHostPath = false
		}
		if strings.HasPrefix(trimmed, "hostPath:") {
			inHostPath = true
			hostIndent = indent
			continue
		}
		if !inHostPath || !strings.HasPrefix(trimmed, "path:") {
			continue
		}
		_, value, _ := strings.Cut(trimmed, ":")
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if value != "" {
			paths = append(paths, value)
		}
	}
	return paths
}

func deploymentHasUnboundedEmptyDir(text string) bool {
	inEmptyDir := false
	emptyIndent := 0
	hasSizeLimit := false
	finish := func() bool {
		if inEmptyDir && !hasSizeLimit {
			return true
		}
		return false
	}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := countLeadingSpaces(line)
		if inEmptyDir && indent <= emptyIndent {
			if finish() {
				return true
			}
			inEmptyDir = false
			hasSizeLimit = false
		}
		if strings.HasPrefix(trimmed, "emptyDir:") {
			if strings.Contains(trimmed, "{}") {
				return true
			}
			inEmptyDir = true
			emptyIndent = indent
			hasSizeLimit = strings.Contains(trimmed, "sizeLimit:")
			continue
		}
		if inEmptyDir && strings.HasPrefix(trimmed, "sizeLimit:") {
			hasSizeLimit = true
		}
	}
	return finish()
}

func hasStorageManagedAnnotation(text string) bool {
	return strings.Contains(text, `opspilot.io/storage-managed: "true"`) ||
		strings.Contains(text, "opspilot.io/storage-managed: true")
}

func countLeadingSpaces(value string) int {
	return len(value) - len(strings.TrimLeft(value, " "))
}

func checkRepoHealth(cfg onboardServiceConfig) repoPolicyItem {
	if cfg.HealthPath == "" {
		return repoPolicyItem{Name: "health_path", Status: "warn", Level: "warning", Message: "health path missing; default /health will be used", Fixable: true, Action: "run repo autofix --write to persist health path"}
	}
	return repoPolicyItem{Name: "health_path", Status: "pass", Level: "info", Message: cfg.HealthPath}
}

func checkRepoMiddleware(cfg onboardServiceConfig) []repoPolicyItem {
	if len(cfg.Middleware) == 0 {
		return []repoPolicyItem{{
			Name:    "middleware",
			Status:  "pass",
			Level:   "info",
			Message: "none detected",
		}}
	}
	items := make([]repoPolicyItem, 0, len(cfg.Middleware))
	for _, item := range cfg.Middleware {
		message := fmt.Sprintf("%s -> %s, allocation=%s, resource=%s, secret=%s",
			item.Display, item.Mode, item.Allocation, item.Resource, item.Secret)
		if len(item.Evidence) > 0 {
			message += "; evidence: " + strings.Join(item.Evidence, "; ")
		}
		items = append(items, repoPolicyItem{
			Name:    "middleware_" + item.Name,
			Status:  "pass",
			Level:   "info",
			Message: message,
		})
	}
	return items
}

func checkRepoStorage(cfg onboardServiceConfig) []repoPolicyItem {
	if len(cfg.Storage) == 0 {
		return []repoPolicyItem{{
			Name:    "storage",
			Status:  "pass",
			Level:   "info",
			Message: "none detected",
		}}
	}
	items := make([]repoPolicyItem, 0, len(cfg.Storage))
	for _, item := range cfg.Storage {
		message := fmt.Sprintf("%s -> %s mount=%s", item.Purpose, item.Mode, item.MountPath)
		if item.Mode == "hostPath" {
			message += " hostPath=" + item.HostPath
			if item.SizeHint != "" {
				message += " sizeHint=" + item.SizeHint
			}
		}
		if item.SizeLimit != "" {
			message += " sizeLimit=" + item.SizeLimit
		}
		if len(item.Evidence) > 0 {
			message += "; evidence: " + strings.Join(item.Evidence, "; ")
		}
		items = append(items, repoPolicyItem{
			Name:    "storage_" + item.Name,
			Status:  "pass",
			Level:   "info",
			Message: message,
		})
	}
	return items
}

func shouldGenerateDockerfile(preflight repoPreflightResult) bool {
	for _, item := range preflight.Items {
		if item.Name == "dockerfile" && item.Fixable && item.Status != "pass" {
			return true
		}
	}
	return false
}

func hasLatestBaseImage(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(line), "FROM ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.HasSuffix(fields[1], ":latest") {
			return true
		}
	}
	return false
}

func containsDangerousPipe(text string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(text), " "))
	return (strings.Contains(normalized, "curl ") || strings.Contains(normalized, "wget ")) &&
		(strings.Contains(normalized, "| sh") || strings.Contains(normalized, "| bash"))
}

func hasDeploymentResources(text string) bool {
	return strings.Contains(text, "resources:") &&
		strings.Contains(text, "requests:") &&
		strings.Contains(text, "limits:") &&
		strings.Contains(text, "cpu:") &&
		strings.Contains(text, "memory:")
}
