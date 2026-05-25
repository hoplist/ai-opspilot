package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type onboardServiceConfig struct {
	Name          string `json:"name"`
	GitLabProject string `json:"gitlab_project"`
	Organization  string `json:"organization,omitempty"`
	Group         string `json:"group,omitempty"`
	Project       string `json:"project,omitempty"`
	Language      string `json:"language"`
	BuildEntry    string `json:"build_entry"`
	BuildOutput   string `json:"build_output"`
	Port          int    `json:"port"`
	HealthPath    string `json:"health_path"`
	Namespace     string `json:"namespace"`
	NamespaceSrc  string `json:"namespace_source,omitempty"`
	Replicas      int    `json:"replicas"`
	Container     string `json:"container"`
	DockerMode    string `json:"dockerfile_mode"`
	DockerPath    string `json:"dockerfile_path"`
	CIMode        string `json:"ci_mode"`
	PromSource    string `json:"prometheus_source"`
}

type onboardWriteResult struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

type onboardResult struct {
	Service        string               `json:"service"`
	Mode           string               `json:"mode"`
	Files          []onboardWriteResult `json:"files"`
	ReleaseMapping string               `json:"release_mapping"`
}

func onboardCommand(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected: onboard service, check, detect, or generate")
	}
	switch args[0] {
	case "service":
		return onboardServiceCommand(args[1:], out)
	case "check":
		return onboardCheckCommand(args[1:], out)
	case "detect":
		return onboardDetectCommand(args[1:], out)
	case "generate":
		return onboardGenerateCommand(args[1:], out)
	default:
		return fmt.Errorf("expected: onboard service, check, detect, or generate")
	}
}

type onboardDetectResult struct {
	Service string               `json:"service"`
	Ready   bool                 `json:"ready"`
	Config  onboardServiceConfig `json:"config"`
	Files   map[string]bool      `json:"files"`
	Gaps    []string             `json:"gaps"`
	Next    []string             `json:"next"`
}

type namespaceResolution struct {
	Namespace    string
	Source       string
	Organization string
	Group        string
	Project      string
	Service      string
}

const (
	defaultOrganization    = "tpo"
	defaultGroup           = "devex"
	defaultNamespacePrefix = "cicd"
)

var projectSuffixes = map[string]bool{
	"admin":   true,
	"api":     true,
	"core":    true,
	"job":     true,
	"service": true,
	"web":     true,
	"worker":  true,
}

func onboardDetectCommand(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("onboard detect", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	project := fs.String("project", "", "GitLab project path, for example tpo/devex/skillshub/skillshub-api")
	catalog := fs.String("namespace-catalog", "opspilot.namespaces.yaml", "namespace catalog path")
	_ = fs.Parse(args)
	result, err := withRepo(*repo, func() (onboardDetectResult, error) {
		return detectOnboardRepository(*project, *catalog)
	})
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(body))
	return err
}

func onboardGenerateCommand(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("onboard generate", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	project := fs.String("project", "", "GitLab project path, for example tpo/devex/skillshub/skillshub-api")
	catalog := fs.String("namespace-catalog", "opspilot.namespaces.yaml", "namespace catalog path")
	write := fs.Bool("write", false, "write generated files")
	force := fs.Bool("force", false, "overwrite existing generated files")
	_ = fs.Parse(args)
	result, err := withRepo(*repo, func() (onboardResult, error) {
		detected, err := detectOnboardRepository(*project, *catalog)
		if err != nil {
			return onboardResult{}, err
		}
		cfg := detected.Config
		files := append([]generatedFile{{path: "opspilot.service.yaml", body: serviceConfigTemplate(cfg)}}, onboardFiles(cfg)...)
		results := make([]onboardWriteResult, 0, len(files))
		for _, file := range files {
			action := "planned"
			if *write {
				action, err = writeGeneratedFile(file.path, file.body, *force)
				if err != nil {
					return onboardResult{}, err
				}
			}
			results = append(results, onboardWriteResult{Path: file.path, Action: action})
		}
		return onboardResult{
			Service:        cfg.Name,
			Mode:           writeMode(*write),
			Files:          results,
			ReleaseMapping: releaseMapping(cfg),
		}, nil
	})
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(body))
	return err
}

func onboardServiceCommand(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("onboard service", flag.ExitOnError)
	configPath := fs.String("config", "opspilot.service.yaml", "service onboarding config")
	write := fs.Bool("write", false, "write generated files")
	force := fs.Bool("force", false, "overwrite existing generated files")
	_ = fs.Parse(args)
	cfg, err := readOnboardServiceConfig(*configPath)
	if err != nil {
		return err
	}
	if err := cfg.defaults(); err != nil {
		return err
	}
	files := onboardFiles(cfg)
	results := make([]onboardWriteResult, 0, len(files))
	for _, file := range files {
		action := "planned"
		if *write {
			action, err = writeGeneratedFile(file.path, file.body, *force)
			if err != nil {
				return err
			}
		}
		results = append(results, onboardWriteResult{Path: file.path, Action: action})
	}
	result := onboardResult{
		Service:        cfg.Name,
		Mode:           writeMode(*write),
		Files:          results,
		ReleaseMapping: releaseMapping(cfg),
	}
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(body))
	return err
}

type onboardCheckItem struct {
	Name     string `json:"name"`
	Path     string `json:"path,omitempty"`
	OK       bool   `json:"ok"`
	Required bool   `json:"required"`
	Message  string `json:"message,omitempty"`
}

type onboardCheckResult struct {
	Service string             `json:"service"`
	Ready   bool               `json:"ready"`
	Items   []onboardCheckItem `json:"items"`
	Missing []string           `json:"missing"`
	Next    []string           `json:"next"`
}

func onboardCheckCommand(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("onboard check", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	configPath := fs.String("config", "opspilot.service.yaml", "service onboarding config")
	_ = fs.Parse(args)
	result, err := withRepo(*repo, func() (onboardCheckResult, error) {
		cfg, err := readOnboardServiceConfig(*configPath)
		if err != nil {
			return onboardCheckResult{}, err
		}
		if err := cfg.defaults(); err != nil {
			return onboardCheckResult{}, err
		}
		return checkOnboardRepository(cfg), nil
	})
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(body))
	if err != nil {
		return err
	}
	if !result.Ready {
		return fmt.Errorf("repository is not ready for OpsPilot release onboarding")
	}
	return nil
}

func checkOnboardRepository(cfg onboardServiceConfig) onboardCheckResult {
	items := []onboardCheckItem{
		checkFile("dockerfile", cfg.DockerPath, true, "Dockerfile used by BuildKit"),
		checkCI(cfg.Language),
		checkFile("namespace", filepath.Join("deploy", "k8s", "namespace.yaml"), true, "Kubernetes Namespace manifest"),
		checkFile("deployment", filepath.Join("deploy", "k8s", "deployment.yaml"), true, "Kubernetes Deployment manifest"),
		checkFile("service", filepath.Join("deploy", "k8s", "service.yaml"), true, "Kubernetes Service manifest"),
		checkFile("kustomization", filepath.Join("deploy", "k8s", "kustomization.yaml"), true, "Kustomize entrypoint"),
		checkFile("release_mapping", "opspilot.release-service.txt", false, "OpsPilot release service mapping"),
	}
	result := onboardCheckResult{Service: cfg.Name, Ready: true, Items: items}
	for _, item := range items {
		if item.OK {
			continue
		}
		if item.Required {
			result.Ready = false
			result.Missing = append(result.Missing, item.Name)
		}
		result.Next = append(result.Next, nextOnboardAction(item))
	}
	return result
}

func checkFile(name, path string, required bool, message string) onboardCheckItem {
	if _, err := os.Stat(path); err == nil {
		return onboardCheckItem{Name: name, Path: path, OK: true, Required: required, Message: message}
	} else if err != nil && !os.IsNotExist(err) {
		return onboardCheckItem{Name: name, Path: path, OK: false, Required: required, Message: err.Error()}
	}
	return onboardCheckItem{Name: name, Path: path, OK: false, Required: required, Message: "missing"}
}

func checkCI(language string) onboardCheckItem {
	body, err := os.ReadFile(".gitlab-ci.yml")
	if err != nil {
		if os.IsNotExist(err) {
			return onboardCheckItem{Name: "buildkit_ci", Path: ".gitlab-ci.yml", OK: false, Required: true, Message: "missing"}
		}
		return onboardCheckItem{Name: "buildkit_ci", Path: ".gitlab-ci.yml", OK: false, Required: true, Message: err.Error()}
	}
	text := string(body)
	template := "/ci/templates/buildkit-gitops." + language + ".yml"
	if strings.Contains(text, template) || strings.Contains(text, "buildctl-daemonless.sh") || strings.Contains(text, "BUILDKIT_IMAGE") {
		return onboardCheckItem{Name: "buildkit_ci", Path: ".gitlab-ci.yml", OK: true, Required: true, Message: "BuildKit CI detected"}
	}
	return onboardCheckItem{Name: "buildkit_ci", Path: ".gitlab-ci.yml", OK: false, Required: true, Message: "GitLab CI exists but BuildKit template or buildctl usage was not detected"}
}

func nextOnboardAction(item onboardCheckItem) string {
	switch item.Name {
	case "dockerfile":
		return "create Dockerfile or set dockerfile.mode: generate then run opspilot onboard service --write"
	case "buildkit_ci":
		return "generate .gitlab-ci.yml with opspilot onboard service --write or include /ci/templates/buildkit-gitops.<language>.yml"
	case "namespace", "deployment", "service", "kustomization":
		return "generate deploy/k8s manifests with opspilot onboard service --write"
	case "release_mapping":
		return "copy opspilot.release-service.txt into OpsPilot release service config"
	default:
		return "run opspilot onboard service --write"
	}
}

func writeMode(write bool) string {
	if write {
		return "write"
	}
	return "plan"
}

type generatedFile struct {
	path string
	body string
}

func onboardFiles(cfg onboardServiceConfig) []generatedFile {
	files := []generatedFile{}
	if cfg.DockerMode == "generate" {
		files = append(files, generatedFile{path: cfg.DockerPath, body: dockerfileTemplate(cfg)})
	}
	if cfg.CIMode == "" || cfg.CIMode == "include" || cfg.CIMode == "generate" {
		files = append(files, generatedFile{path: ".gitlab-ci.yml", body: gitlabCIIncludeTemplate(cfg)})
	}
	files = append(files,
		generatedFile{path: filepath.Join("deploy", "k8s", "namespace.yaml"), body: namespaceTemplate(cfg)},
		generatedFile{path: filepath.Join("deploy", "k8s", "deployment.yaml"), body: deploymentTemplate(cfg)},
		generatedFile{path: filepath.Join("deploy", "k8s", "service.yaml"), body: serviceTemplate(cfg)},
		generatedFile{path: filepath.Join("deploy", "k8s", "kustomization.yaml"), body: kustomizationTemplate()},
		generatedFile{path: "opspilot.release-service.txt", body: releaseMapping(cfg) + "\n"},
	)
	return files
}

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
		port = 8080
	}
	namespace := resolveNamespace(project, name, catalogPath)
	cfg := onboardServiceConfig{
		Name:          name,
		GitLabProject: project,
		Organization:  namespace.Organization,
		Group:         namespace.Group,
		Project:       namespace.Project,
		Language:      language,
		BuildEntry:    detectBuildEntry(language, name),
		BuildOutput:   "build/" + name,
		Port:          port,
		HealthPath:    "/health",
		Namespace:     namespace.Namespace,
		NamespaceSrc:  namespace.Source,
		Replicas:      1,
		Container:     name,
		DockerMode:    "existing",
		DockerPath:    dockerPath,
		CIMode:        "include",
		PromSource:    "node200-k8s",
	}
	if cfg.GitLabProject == "" {
		cfg.GitLabProject = defaultGitLabProject(cfg)
	}
	if cfg.DockerPath == "" {
		cfg.DockerPath = "Dockerfile"
		cfg.DockerMode = "generate"
	}
	files := map[string]bool{
		"dockerfile":     fileExists(cfg.DockerPath),
		"gitlab_ci":      fileExists(".gitlab-ci.yml"),
		"namespace":      fileExists(filepath.Join("deploy", "k8s", "namespace.yaml")),
		"deployment":     fileExists(filepath.Join("deploy", "k8s", "deployment.yaml")),
		"service":        fileExists(filepath.Join("deploy", "k8s", "service.yaml")),
		"kustomization":  fileExists(filepath.Join("deploy", "k8s", "kustomization.yaml")),
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
	case fileExists("package.json"):
		return "node"
	case fileExists("pyproject.toml") || fileExists("requirements.txt"):
		return "python"
	default:
		return "go"
	}
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
	}
	return "./cmd/" + name
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
	case len(parts) >= 4 && parts[0] == defaultOrganization:
		resolved.Organization = sanitizeDNSLabel(parts[0])
		resolved.Group = sanitizeDNSLabel(parts[1])
		resolved.Project = projectNameFromService(parts[2])
		resolved.Service = sanitizeDNSLabel(parts[len(parts)-1])
	case len(parts) >= 3 && parts[0] == defaultOrganization:
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

func writeGeneratedFile(path, body string, force bool) (string, error) {
	if _, err := os.Stat(path); err == nil && !force {
		return "skipped_existing", nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return "written", nil
}

func serviceConfigTemplate(c onboardServiceConfig) string {
	return fmt.Sprintf(`name: %s
gitlabProject: %s
ownership:
  organization: %s
  group: %s
  project: %s

language: %s

build:
  entry: %s
  output: %s

runtime:
  port: %d
  healthPath: %s

deploy:
  namespace: %s
  namespaceSource: %s
  replicas: %d
  container: %s

dockerfile:
  mode: %s
  path: %s

ci:
  mode: %s

release:
  prometheusSource: %s
`, c.Name, c.GitLabProject, c.Organization, c.Group, c.Project, c.Language, c.BuildEntry, c.BuildOutput, c.Port, c.HealthPath, c.Namespace, firstNonEmpty(c.NamespaceSrc, "manual"), c.Replicas, c.Container, c.DockerMode, c.DockerPath, c.CIMode, c.PromSource)
}

func readOnboardServiceConfig(path string) (onboardServiceConfig, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return onboardServiceConfig{}, err
	}
	values := parseSimpleYAML(string(body))
	return onboardServiceConfig{
		Name:          values["name"],
		GitLabProject: values["gitlabProject"],
		Organization:  values["ownership.organization"],
		Group:         values["ownership.group"],
		Project:       values["ownership.project"],
		Language:      values["language"],
		BuildEntry:    values["build.entry"],
		BuildOutput:   values["build.output"],
		Port:          intFromString(values["runtime.port"], 0),
		HealthPath:    values["runtime.healthPath"],
		Namespace:     values["deploy.namespace"],
		NamespaceSrc:  values["deploy.namespaceSource"],
		Replicas:      intFromString(values["deploy.replicas"], 0),
		Container:     values["deploy.container"],
		DockerMode:    values["dockerfile.mode"],
		DockerPath:    values["dockerfile.path"],
		CIMode:        values["ci.mode"],
		PromSource:    values["release.prometheusSource"],
	}, nil
}

func (c *onboardServiceConfig) defaults() error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return fmt.Errorf("service name is required")
	}
	if c.Language == "" {
		c.Language = "go"
	}
	resolved := inferOwnership(c.GitLabProject, c.Name)
	if c.Organization == "" {
		c.Organization = resolved.Organization
	}
	if c.Group == "" {
		c.Group = resolved.Group
	}
	if c.Project == "" {
		c.Project = resolved.Project
	}
	if c.GitLabProject == "" {
		c.GitLabProject = defaultGitLabProject(*c)
	}
	if c.BuildEntry == "" {
		c.BuildEntry = "./cmd/" + c.Name
	}
	if c.BuildOutput == "" {
		c.BuildOutput = "build/" + c.Name
	}
	if c.Port == 0 {
		c.Port = 8080
	}
	if c.HealthPath == "" {
		c.HealthPath = "/health"
	}
	if c.Namespace == "" {
		c.Namespace = defaultNamespace(c.Group, c.Project)
		c.NamespaceSrc = "auto_project"
	}
	if c.NamespaceSrc == "" {
		c.NamespaceSrc = "manual"
	}
	if c.Replicas == 0 {
		c.Replicas = 1
	}
	if c.Container == "" {
		c.Container = c.Name
	}
	if c.DockerMode == "" {
		c.DockerMode = "existing"
	}
	if c.DockerPath == "" {
		c.DockerPath = "Dockerfile"
	}
	if c.CIMode == "" {
		c.CIMode = "include"
	}
	if c.PromSource == "" {
		c.PromSource = "node200-k8s"
	}
	return nil
}

func parseSimpleYAML(raw string) map[string]string {
	out := map[string]string{}
	section := ""
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, " \t\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if indent == 0 && value == "" {
			section = key
			continue
		}
		if indent == 0 {
			section = ""
			out[key] = value
			continue
		}
		if section != "" {
			out[section+"."+key] = value
		}
	}
	return out
}

func intFromString(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func dockerfileTemplate(c onboardServiceConfig) string {
	switch c.Language {
	case "node":
		return fmt.Sprintf(`FROM m.daocloud.io/docker.io/library/node:20-alpine

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY

WORKDIR /app
COPY package*.json ./
RUN npm ci --omit=dev
COPY . .

EXPOSE %d

CMD ["npm", "start"]
`, c.Port)
	case "python":
		return fmt.Sprintf(`FROM m.daocloud.io/docker.io/library/python:3.12-alpine

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY

WORKDIR /app
COPY . .
RUN if [ -f requirements.txt ]; then pip install --no-cache-dir -r requirements.txt; fi

EXPOSE %d

CMD ["python", "app.py"]
`, c.Port)
	}
	return fmt.Sprintf(`FROM m.daocloud.io/docker.io/library/alpine:3.20

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY

COPY %s /usr/local/bin/%s

EXPOSE %d

ENTRYPOINT ["/usr/local/bin/%s"]
`, c.BuildOutput, c.Container, c.Port, c.Container)
}

func gitlabCIIncludeTemplate(c onboardServiceConfig) string {
	return fmt.Sprintf(`include:
  - project: tpo/devex/opspilot/opspilot-core
    ref: main
    file: /ci/templates/buildkit-gitops.%s.yml

variables:
  APP_NAME: "%s"
  ARGOCD_APP_NAME: "%s"
  BUILD_ENTRY: "%s"
  BUILD_OUTPUT: "%s"
  DOCKERFILE_PATH: "%s"
  GITOPS_APP_PATH: "%s"
  GITOPS_APP_FILE: "apps/%s-application.yaml"
  GITOPS_CONTAINER_NAME: "%s"
  DEPLOY_NAMESPACE: "%s"
`, c.Language, c.Name, argoAppName(c), c.BuildEntry, c.BuildOutput, c.DockerPath, gitOpsAppPath(c), argoAppName(c), c.Container, c.Namespace)
}

func namespaceTemplate(c onboardServiceConfig) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    opspilot.io/managed: "true"
    opspilot.io/organization: %s
    opspilot.io/group: %s
    opspilot.io/project: %s
`, c.Namespace, c.Organization, c.Group, c.Project)
}

func deploymentTemplate(c onboardServiceConfig) string {
	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
spec:
  replicas: %d
  selector:
    matchLabels:
      app.kubernetes.io/name: %s
  template:
    metadata:
      labels:
        app.kubernetes.io/name: %s
    spec:
      containers:
        - name: %s
          image: placeholder
          imagePullPolicy: IfNotPresent
          ports:
            - name: http
              containerPort: %d
          readinessProbe:
            httpGet:
              path: %s
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: %s
              port: http
            initialDelaySeconds: 15
            periodSeconds: 20
`, c.Name, c.Namespace, c.Name, c.Replicas, c.Name, c.Name, c.Container, c.Port, c.HealthPath, c.HealthPath)
}

func serviceTemplate(c onboardServiceConfig) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    app.kubernetes.io/name: %s
  ports:
    - name: http
      port: %d
      targetPort: http
`, c.Name, c.Namespace, c.Name, c.Port)
}

func kustomizationTemplate() string {
	return `resources:
  - namespace.yaml
  - deployment.yaml
  - service.yaml
`
}

func releaseMapping(c onboardServiceConfig) string {
	image := "192.168.48.206:5050/" + c.GitLabProject + "/" + c.Name
	gitops := gitOpsAppPath(c) + "/deployment.yaml"
	return fmt.Sprintf("%s=namespace:%s,deployment:%s,container:%s,source:%s,image:%s,gitlab:%s,gitops:%s,argocd:%s",
		c.Name, c.Namespace, c.Name, c.Container, c.PromSource, image, c.GitLabProject, gitops, argoAppName(c))
}
