package main

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func detectOnboardRepository(project, catalogPath string) (onboardDetectResult, error) {
	name := serviceNameFromProject(project)
	if name == "" {
		wd, err := os.Getwd()
		if err != nil {
			return onboardDetectResult{}, err
		}
		name = filepath.Base(wd)
	}
	language := detectLanguage()
	dockerPath := detectDockerfile()
	port := detectPort(dockerPath)
	if port == 0 {
		port = defaultPortForLanguage(language)
	}
	namespace := resolveNamespace(project, name, catalogPath)
	cfg := onboardServiceConfig{
		Name:           name,
		GitLabProject:  project,
		Organization:   namespace.Organization,
		Group:          namespace.Group,
		Project:        namespace.Project,
		Language:       language,
		BuildEntry:     detectBuildEntry(language, name),
		BuildOutput:    "build/" + name,
		Port:           port,
		HealthPath:     defaultHealthPathForLanguage(language),
		Namespace:      namespace.Namespace,
		NamespaceSrc:   namespace.Source,
		Replicas:       1,
		Container:      name,
		DockerMode:     "existing",
		DockerPath:     dockerPath,
		CIMode:         "include",
		PromSource:     "node200-k8s",
		Resources:      resourceProfiles[defaultResourceProfile],
		NamespaceGuard: defaultNamespaceGuard,
	}
	if cfg.GitLabProject == "" {
		cfg.GitLabProject = defaultGitLabProject(cfg)
	}
	if cfg.DockerPath == "" {
		cfg.DockerPath = "Dockerfile"
		cfg.DockerMode = "generate"
	}
	cfg.Middleware = detectMiddlewareRequirements(cfg)
	cfg.Storage = detectStorageRequirements(cfg)
	files := map[string]bool{
		"dockerfile":     fileExists(cfg.DockerPath),
		"gitlab_ci":      fileExists(".gitlab-ci.yml"),
		"namespace":      fileExists(filepath.Join("deploy", "k8s", "namespace.yaml")),
		"limitrange":     fileExists(filepath.Join("deploy", "k8s", "limitrange.yaml")),
		"resourcequota":  fileExists(filepath.Join("deploy", "k8s", "resourcequota.yaml")),
		"deployment":     fileExists(filepath.Join("deploy", "k8s", "deployment.yaml")),
		"service":        fileExists(filepath.Join("deploy", "k8s", "service.yaml")),
		"kustomization":  fileExists(filepath.Join("deploy", "k8s", "kustomization.yaml")),
		"qualityConfig":  fileExists(filepath.Join(".opspilot", "quality.yaml")),
		"releaseMapping": fileExists("opspilot.release-service.txt"),
	}
	result := onboardDetectResult{Service: cfg.Name, Ready: true, Config: cfg, Files: files}
	if !files["dockerfile"] {
		result.Ready = false
		result.Gaps = append(result.Gaps, "dockerfile_missing")
		result.Next = append(result.Next, "generate a simple Dockerfile or add a project-owned Dockerfile")
	}
	if !files["gitlab_ci"] {
		result.Ready = false
		result.Gaps = append(result.Gaps, "gitlab_ci_missing")
		result.Next = append(result.Next, "generate .gitlab-ci.yml with BuildKit include")
	}
	if !files["namespace"] || !files["deployment"] || !files["service"] || !files["kustomization"] {
		result.Ready = false
		result.Gaps = append(result.Gaps, "deploy_yaml_missing")
		result.Next = append(result.Next, "generate deploy/k8s manifests")
	}
	if !files["limitrange"] || !files["resourcequota"] {
		result.Ready = false
		result.Gaps = append(result.Gaps, "namespace_guardrails_missing")
		result.Next = append(result.Next, "generate LimitRange and ResourceQuota manifests")
	}
	return result, nil
}

func withRepo[T any](repo string, fn func() (T, error)) (T, error) {
	var zero T
	wd, err := os.Getwd()
	if err != nil {
		return zero, err
	}
	if err := os.Chdir(repo); err != nil {
		return zero, err
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	return fn()
}

func detectLanguage() string {
	switch {
	case fileExists("go.mod"):
		return "go"
	case fileExists("pom.xml") || fileExists("build.gradle") || fileExists("build.gradle.kts"):
		return "java"
	case fileExists("pyproject.toml") || fileExists("requirements.txt"):
		return "python"
	case fileExists("package.json") && isFrontendPackage():
		return "frontend"
	case fileExists("package.json"):
		return "node"
	default:
		return "go"
	}
}

func isFrontendPackage() bool {
	body, err := os.ReadFile("package.json")
	if err != nil {
		return false
	}
	text := strings.ToLower(string(body))
	return containsAny(text, []string{
		`"vite"`,
		"@vitejs/",
		`"react-scripts"`,
		`"vue-cli-service"`,
		`"@vue/cli-service"`,
		`"@angular/cli"`,
		`"ng build"`,
	})
}

func detectDockerfile() string {
	candidates := []string{"Dockerfile", "docker/Dockerfile", "deploy/Dockerfile"}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

func detectPort(dockerPath string) int {
	if dockerPath == "" {
		return 0
	}
	body, err := os.ReadFile(dockerPath)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 2 && strings.EqualFold(fields[0], "EXPOSE") {
			port, _ := strconv.Atoi(strings.TrimSuffix(fields[1], "/tcp"))
			return port
		}
	}
	return 0
}

func detectBuildEntry(language, name string) string {
	switch language {
	case "go":
		if fileExists(filepath.Join("cmd", name, "main.go")) {
			return "./cmd/" + name
		}
		if fileExists("main.go") {
			return "."
		}
	case "node":
		return "."
	case "python":
		return "."
	case "frontend":
		return "."
	case "java":
		return "."
	}
	return "./cmd/" + name
}

func defaultPortForLanguage(language string) int {
	if language == "frontend" {
		return 80
	}
	return 8080
}

func defaultHealthPathForLanguage(language string) string {
	if language == "frontend" {
		return "/"
	}
	return "/health"
}

func resolveNamespace(project, name, catalogPath string) namespaceResolution {
	resolved := inferOwnership(project, name)
	mappings, err := readNamespaceCatalog(catalogPath)
	if err == nil {
		targets := []string{
			project,
			defaultGitLabProject(onboardServiceConfig{
				Name:         resolved.Service,
				Organization: resolved.Organization,
				Group:        resolved.Group,
				Project:      resolved.Project,
			}),
			"platform/" + resolved.Service,
			resolved.Service,
		}
		for _, target := range targets {
			for pattern, namespace := range mappings {
				if globMatch(pattern, target) {
					resolved.Namespace = namespace
					resolved.Source = "catalog"
					return resolved
				}
			}
		}
	}
	resolved.Namespace = defaultNamespace(resolved.Group, resolved.Project)
	resolved.Source = "auto_project"
	return resolved
}

func inferOwnership(project, name string) namespaceResolution {
	service := sanitizeDNSLabel(firstNonEmpty(serviceNameFromProject(project), name))
	projectPath := strings.Trim(project, "/")
	parts := []string{}
	if projectPath != "" {
		for _, part := range strings.Split(projectPath, "/") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
	}

	resolved := namespaceResolution{
		Organization: defaultOrganization,
		Group:        defaultGroup,
		Service:      service,
	}
	switch {
	case len(parts) >= 4:
		resolved.Organization = sanitizeDNSLabel(parts[0])
		resolved.Group = sanitizeDNSLabel(parts[1])
		resolved.Project = projectNameFromService(parts[2])
		resolved.Service = sanitizeDNSLabel(parts[len(parts)-1])
	case len(parts) >= 3:
		resolved.Organization = sanitizeDNSLabel(parts[0])
		resolved.Group = sanitizeDNSLabel(parts[1])
		resolved.Service = sanitizeDNSLabel(parts[len(parts)-1])
	case len(parts) >= 2:
		resolved.Service = sanitizeDNSLabel(parts[len(parts)-1])
	}
	if resolved.Project == "" {
		resolved.Project = projectNameFromService(resolved.Service)
	}
	if resolved.Service == "" {
		resolved.Service = sanitizeDNSLabel(name)
	}
	if resolved.Project == "" {
		resolved.Project = projectNameFromService(resolved.Service)
	}
	return resolved
}

func projectNameFromService(service string) string {
	service = sanitizeDNSLabel(service)
	if service == "" {
		return ""
	}
	parts := strings.Split(service, "-")
	if len(parts) > 1 && projectSuffixes[parts[len(parts)-1]] {
		parts = parts[:len(parts)-1]
	}
	return sanitizeDNSLabel(strings.Join(parts, "-"))
}

func defaultNamespace(group, project string) string {
	return sanitizeDNSLabel(defaultNamespacePrefix + "-" + firstNonEmpty(group, defaultGroup) + "-" + project)
}

func defaultGitLabProject(c onboardServiceConfig) string {
	org := firstNonEmpty(c.Organization, defaultOrganization)
	group := firstNonEmpty(c.Group, defaultGroup)
	project := firstNonEmpty(c.Project, projectNameFromService(c.Name))
	service := firstNonEmpty(c.Name, c.Container)
	return strings.Join([]string{org, group, project, service}, "/")
}

func gitOpsAppPath(c onboardServiceConfig) string {
	return "clusters/test/apps/" + strings.Join([]string{
		firstNonEmpty(c.Group, defaultGroup),
		firstNonEmpty(c.Project, projectNameFromService(c.Name)),
		c.Name,
	}, "/")
}

func argoAppName(c onboardServiceConfig) string {
	return sanitizeDNSLabel(strings.Join([]string{
		firstNonEmpty(c.Group, defaultGroup),
		firstNonEmpty(c.Project, projectNameFromService(c.Name)),
		c.Name,
	}, "-"))
}

func sanitizeDNSLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
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
		return ""
	}
	if len(out) <= 63 {
		return out
	}
	sum := sha1.Sum([]byte(out))
	suffix := "-" + hex.EncodeToString(sum[:])[:6]
	out = strings.Trim(out[:63-len(suffix)], "-") + suffix
	return strings.Trim(out, "-")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func readNamespaceCatalog(path string) (map[string]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := parseSimpleYAML(string(body))
	out := map[string]string{}
	for key, value := range values {
		if strings.HasPrefix(key, "namespaceMappings.") && value != "" {
			out[strings.TrimPrefix(key, "namespaceMappings.")] = value
		}
	}
	return out, nil
}

func globMatch(pattern, value string) bool {
	if pattern == value {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}
	return false
}

func serviceNameFromProject(project string) string {
	project = strings.Trim(project, "/")
	if project == "" {
		return ""
	}
	parts := strings.Split(project, "/")
	return parts[len(parts)-1]
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
