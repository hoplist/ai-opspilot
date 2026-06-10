package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

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
		if err := cfg.defaults(); err != nil {
			return onboardResult{}, err
		}
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
			GitOpsPlan:     gitOpsPlan(cfg),
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
		checkFile("limitrange", filepath.Join("deploy", "k8s", "limitrange.yaml"), true, "Kubernetes LimitRange guardrail"),
		checkFile("resourcequota", filepath.Join("deploy", "k8s", "resourcequota.yaml"), true, "Kubernetes ResourceQuota guardrail"),
		checkFile("serviceaccount", filepath.Join("deploy", "k8s", "serviceaccount.yaml"), true, "Kubernetes ServiceAccount with image pull secret"),
		checkFile("deployment", filepath.Join("deploy", "k8s", "deployment.yaml"), true, "Kubernetes Deployment manifest"),
		checkFile("service", filepath.Join("deploy", "k8s", "service.yaml"), true, "Kubernetes Service manifest"),
		checkFile("kustomization", filepath.Join("deploy", "k8s", "kustomization.yaml"), true, "Kustomize entrypoint"),
		checkFile("quality_config", filepath.Join(".opspilot", "quality.yaml"), false, "optional OpsPilot API quality checks"),
		checkFile("release_mapping", "opspilot.release-service.txt", false, "OpsPilot release service mapping"),
	}
	items = append(items, checkOnboardDeploymentGuardrails(cfg)...)
	items = append(items, checkOnboardMiddleware(cfg)...)
	items = append(items, checkOnboardStorage(cfg)...)
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

func checkOnboardMiddleware(cfg onboardServiceConfig) []onboardCheckItem {
	if len(cfg.Middleware) == 0 {
		return []onboardCheckItem{{
			Name:     "middleware",
			OK:       true,
			Required: false,
			Message:  "none configured",
		}}
	}
	items := make([]onboardCheckItem, 0, len(cfg.Middleware))
	for _, item := range cfg.Middleware {
		items = append(items, onboardCheckItem{
			Name:     "middleware_" + item.Name,
			OK:       true,
			Required: false,
			Message:  fmt.Sprintf("%s uses %s allocation=%s provision=%s secret=%s", item.Display, item.Mode, item.Allocation, firstNonEmpty(item.Provision, "external"), item.Secret),
		})
	}
	return items
}

func checkOnboardStorage(cfg onboardServiceConfig) []onboardCheckItem {
	if len(cfg.Storage) == 0 {
		return []onboardCheckItem{{
			Name:     "storage",
			OK:       true,
			Required: false,
			Message:  "none configured",
		}}
	}
	items := make([]onboardCheckItem, 0, len(cfg.Storage))
	for _, item := range cfg.Storage {
		message := fmt.Sprintf("%s uses %s mounted at %s", item.Purpose, item.Mode, item.MountPath)
		if item.Mode == "hostPath" {
			message += " hostPath=" + item.HostPath
		}
		if item.SizeLimit != "" {
			message += " sizeLimit=" + item.SizeLimit
		}
		items = append(items, onboardCheckItem{
			Name:     "storage_" + item.Name,
			OK:       true,
			Required: false,
			Message:  message,
		})
	}
	return items
}

func checkOnboardDeploymentGuardrails(cfg onboardServiceConfig) []onboardCheckItem {
	path := filepath.Join("deploy", "k8s", "deployment.yaml")
	body, err := os.ReadFile(path)
	if err != nil {
		return []onboardCheckItem{}
	}
	text := string(body)
	items := []onboardCheckItem{
		{Name: "deployment_resources", Path: path, OK: hasDeploymentResources(text), Required: true, Message: "CPU/memory requests and limits"},
		{Name: "deployment_probes", Path: path, OK: strings.Contains(text, "readinessProbe:") && strings.Contains(text, "livenessProbe:"), Required: true, Message: "readiness/liveness probes"},
	}
	if storageIssues, storageBlocker := deploymentStoragePolicyIssues(text, cfg); len(storageIssues) > 0 {
		items = append(items, onboardCheckItem{
			Name:     "deployment_storage",
			Path:     path,
			OK:       false,
			Required: storageBlocker,
			Message:  strings.Join(storageIssues, "; "),
		})
	} else {
		items = append(items, onboardCheckItem{
			Name:     "deployment_storage",
			Path:     path,
			OK:       true,
			Required: false,
			Message:  "storage policy",
		})
	}
	for i := range items {
		if !items[i].OK && items[i].Name != "deployment_storage" {
			items[i].Message = "missing " + items[i].Message
		}
	}
	return items
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
	case "namespace", "limitrange", "resourcequota", "deployment", "service", "kustomization", "deployment_resources", "deployment_probes", "deployment_storage":
		return "generate deploy/k8s manifests with opspilot onboard service --write"
	case "release_mapping":
		return "copy opspilot.release-service.txt into OpsPilot release service config"
	case "quality_config":
		return "generate optional .opspilot/quality.yaml with opspilot onboard service --write"
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
