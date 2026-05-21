package main

import (
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
	Language      string `json:"language"`
	BuildEntry    string `json:"build_entry"`
	BuildOutput   string `json:"build_output"`
	Port          int    `json:"port"`
	HealthPath    string `json:"health_path"`
	Namespace     string `json:"namespace"`
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
	if len(args) == 0 || args[0] != "service" {
		return fmt.Errorf("expected: onboard service")
	}
	fs := flag.NewFlagSet("onboard service", flag.ExitOnError)
	configPath := fs.String("config", "opspilot.service.yaml", "service onboarding config")
	write := fs.Bool("write", false, "write generated files")
	force := fs.Bool("force", false, "overwrite existing generated files")
	_ = fs.Parse(args[1:])
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
		generatedFile{path: filepath.Join("deploy", "k8s", "deployment.yaml"), body: deploymentTemplate(cfg)},
		generatedFile{path: filepath.Join("deploy", "k8s", "service.yaml"), body: serviceTemplate(cfg)},
		generatedFile{path: filepath.Join("deploy", "k8s", "kustomization.yaml"), body: kustomizationTemplate()},
		generatedFile{path: "opspilot.release-service.txt", body: releaseMapping(cfg) + "\n"},
	)
	return files
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

func readOnboardServiceConfig(path string) (onboardServiceConfig, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return onboardServiceConfig{}, err
	}
	values := parseSimpleYAML(string(body))
	return onboardServiceConfig{
		Name:          values["name"],
		GitLabProject: values["gitlabProject"],
		Language:      values["language"],
		BuildEntry:    values["build.entry"],
		BuildOutput:   values["build.output"],
		Port:          intFromString(values["runtime.port"], 0),
		HealthPath:    values["runtime.healthPath"],
		Namespace:     values["deploy.namespace"],
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
	if c.GitLabProject == "" {
		c.GitLabProject = "platform/" + c.Name
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
		c.Namespace = c.Name
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
  - project: platform/opspilot
    ref: main
    file: /ci/templates/buildkit-gitops.%s.yml

variables:
  APP_NAME: "%s"
  BUILD_ENTRY: "%s"
  BUILD_OUTPUT: "%s"
  DOCKERFILE_PATH: "%s"
  GITOPS_APP_PATH: "clusters/test/apps/%s"
  GITOPS_APP_FILE: "apps/%s-application.yaml"
  GITOPS_CONTAINER_NAME: "%s"
  DEPLOY_NAMESPACE: "%s"
`, c.Language, c.Name, c.BuildEntry, c.BuildOutput, c.DockerPath, c.Name, c.Name, c.Container, c.Namespace)
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
      imagePullSecrets:
        - name: gitlab-registry-pull
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
  - deployment.yaml
  - service.yaml
`
}

func releaseMapping(c onboardServiceConfig) string {
	image := "192.168.48.206:5050/" + c.GitLabProject + "/" + c.Name
	gitops := "clusters/test/apps/" + c.Name + "/deployment.yaml"
	return fmt.Sprintf("%s=namespace:%s,deployment:%s,container:%s,source:%s,image:%s,gitlab:%s,gitops:%s,argocd:%s",
		c.Name, c.Namespace, c.Name, c.Container, c.PromSource, image, c.GitLabProject, gitops, c.Name)
}
