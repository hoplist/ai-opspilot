package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
)

type repoPolicyItem struct {
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Status  string `json:"status"`
	Level   string `json:"level"`
	Message string `json:"message,omitempty"`
	Fixable bool   `json:"fixable"`
	Action  string `json:"action,omitempty"`
}

type repoPreflightResult struct {
	Service     string               `json:"service"`
	Project     string               `json:"project"`
	Language    string               `json:"language"`
	Namespace   string               `json:"namespace"`
	Ready       bool                 `json:"ready"`
	Autofixable bool                 `json:"autofixable"`
	Items       []repoPolicyItem     `json:"items"`
	Gaps        []string             `json:"gaps"`
	Next        []string             `json:"next"`
	Config      onboardServiceConfig `json:"config"`
}

type repoAutofixResult struct {
	Service        string               `json:"service"`
	Project        string               `json:"project"`
	Mode           string               `json:"mode"`
	Files          []onboardWriteResult `json:"files"`
	ReleaseMapping string               `json:"release_mapping"`
	Preflight      repoPreflightResult  `json:"preflight"`
}

type codePrecheckSummary struct {
	Blockers int `json:"blockers"`
	Warnings int `json:"warnings"`
	Passed   int `json:"passed"`
}

type codePrecheckItem struct {
	ID             string `json:"id"`
	Severity       string `json:"severity"`
	Category       string `json:"category"`
	Path           string `json:"path"`
	Line           int    `json:"line"`
	Message        string `json:"message"`
	Snippet        string `json:"snippet,omitempty"`
	Skill          string `json:"skill"`
	Recommendation string `json:"recommendation"`
}

type codePrecheckResult struct {
	Service      string              `json:"service"`
	Project      string              `json:"project"`
	Status       string              `json:"status"`
	Ready        bool                `json:"ready"`
	Summary      codePrecheckSummary `json:"summary"`
	Items        []codePrecheckItem  `json:"items"`
	EvidencePath string              `json:"evidence_path,omitempty"`
	Skills       []string            `json:"skills"`
	Next         []string            `json:"next,omitempty"`
}

func repoCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected repo command: preflight, precheck, or autofix")
	}
	switch args[0] {
	case "preflight":
		return repoPreflightCommand(opts, args[1:], out)
	case "precheck":
		return repoPrecheckCommand(opts, args[1:], out)
	case "autofix":
		return repoAutofixCommand(opts, args[1:], out)
	default:
		return fmt.Errorf("unknown repo command: %s", args[0])
	}
}

func repoPreflightCommand(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("repo preflight", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	project := fs.String("project", "", "GitLab project path, for example tpo/devex/skillshub/skillshub-api")
	catalog := fs.String("namespace-catalog", "opspilot.namespaces.yaml", "namespace catalog path")
	_ = fs.Parse(args)
	result, err := withRepo(*repo, func() (repoPreflightResult, error) {
		return repoPreflight(*project, *catalog)
	})
	if err != nil {
		return err
	}
	writeErr := writeRepoPreflight(out, opts.output, result)
	if writeErr != nil {
		return writeErr
	}
	if !result.Ready {
		return fmt.Errorf("repository failed OpsPilot governance preflight")
	}
	return nil
}

func repoPrecheckCommand(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("repo precheck", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	project := fs.String("project", "", "GitLab project path, for example tpo/devex/skillshub/skillshub-api")
	write := fs.Bool("write", false, "write .opspilot/evidence/code-precheck.json")
	warnOnly := fs.Bool("warn-only", false, "do not fail when blocker findings exist")
	_ = fs.Parse(args)
	result, err := withRepo(*repo, func() (codePrecheckResult, error) {
		result, err := codePrecheck(*project)
		if err != nil {
			return codePrecheckResult{}, err
		}
		if *write {
			if err := writeCodePrecheckEvidence(result); err != nil {
				return codePrecheckResult{}, err
			}
			result.EvidencePath = codePrecheckEvidencePath()
		}
		return result, nil
	})
	if err != nil {
		return err
	}
	writeErr := writeCodePrecheck(out, opts.output, result)
	if writeErr != nil {
		return writeErr
	}
	if !*warnOnly && !result.Ready {
		return fmt.Errorf("repository failed OpsPilot code precheck")
	}
	return nil
}

func repoAutofixCommand(opts globalOptions, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("repo autofix", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	project := fs.String("project", "", "GitLab project path, for example tpo/devex/skillshub/skillshub-api")
	catalog := fs.String("namespace-catalog", "opspilot.namespaces.yaml", "namespace catalog path")
	write := fs.Bool("write", false, "write generated files")
	force := fs.Bool("force", false, "overwrite existing generated files")
	_ = fs.Parse(args)
	result, err := withRepo(*repo, func() (repoAutofixResult, error) {
		preflight, err := repoPreflight(*project, *catalog)
		if err != nil {
			return repoAutofixResult{}, err
		}
		cfg := preflight.Config
		if shouldGenerateDockerfile(preflight) {
			cfg.DockerMode = "generate"
		}
		files := append([]generatedFile{{path: "opspilot.service.yaml", body: serviceConfigTemplate(cfg)}}, onboardFiles(cfg)...)
		results := make([]onboardWriteResult, 0, len(files))
		for _, file := range files {
			action := "planned"
			if *write {
				action, err = writeGeneratedFile(file.path, file.body, *force)
				if err != nil {
					return repoAutofixResult{}, err
				}
			}
			results = append(results, onboardWriteResult{Path: file.path, Action: action})
		}
		return repoAutofixResult{
			Service:        cfg.Name,
			Project:        cfg.GitLabProject,
			Mode:           writeMode(*write),
			Files:          results,
			ReleaseMapping: releaseMapping(cfg),
			Preflight:      preflight,
		}, nil
	})
	if err != nil {
		return err
	}
	return writeOutput(out, opts.output, result, func(w io.Writer) error {
		fmt.Fprintf(w, "Repo autofix: %s mode=%s\n", result.Service, result.Mode)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ACTION\tPATH")
		for _, file := range result.Files {
			fmt.Fprintf(tw, "%s\t%s\n", file.Action, file.Path)
		}
		return tw.Flush()
	})
}

func repoPreflight(project, catalogPath string) (repoPreflightResult, error) {
	detected, err := detectOnboardRepository(project, catalogPath)
	if err != nil {
		return repoPreflightResult{}, err
	}
	cfg := detected.Config
	if err := cfg.defaults(); err != nil {
		return repoPreflightResult{}, err
	}
	items := []repoPolicyItem{
		checkRepoDockerfile(cfg),
		checkRepoCI(cfg),
		checkRepoFile("namespace", filepath.Join("deploy", "k8s", "namespace.yaml"), "generate deploy/k8s/namespace.yaml from ownership"),
		checkRepoFile("limitrange", filepath.Join("deploy", "k8s", "limitrange.yaml"), "generate deploy/k8s/limitrange.yaml for namespace defaults"),
		checkRepoFile("resourcequota", filepath.Join("deploy", "k8s", "resourcequota.yaml"), "generate deploy/k8s/resourcequota.yaml for namespace quota"),
		checkRepoDeployment(cfg),
		checkRepoFile("service", filepath.Join("deploy", "k8s", "service.yaml"), "generate deploy/k8s/service.yaml"),
		checkRepoFile("kustomization", filepath.Join("deploy", "k8s", "kustomization.yaml"), "generate deploy/k8s/kustomization.yaml"),
		checkRepoQuality(),
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

func writeRepoPreflight(out io.Writer, output string, result repoPreflightResult) error {
	return writeOutput(out, output, result, func(w io.Writer) error {
		fmt.Fprintf(w, "Repo preflight: %s ready=%t autofixable=%t\n", result.Service, result.Ready, result.Autofixable)
		fmt.Fprintf(w, "Project: %s namespace=%s language=%s\n", result.Project, result.Namespace, result.Language)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "STATUS\tLEVEL\tCHECK\tFIX\tMESSAGE")
		for _, item := range result.Items {
			fix := ""
			if item.Fixable {
				fix = "auto"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", item.Status, item.Level, item.Name, fix, item.Message)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		if len(result.Next) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(uniqueStrings(result.Next), "; "))
		}
		return nil
	})
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

func checkRepoCI(cfg onboardServiceConfig) repoPolicyItem {
	body, err := os.ReadFile(".gitlab-ci.yml")
	if err != nil {
		if os.IsNotExist(err) {
			return repoPolicyItem{Name: "gitlab_ci", Path: ".gitlab-ci.yml", Status: "fail", Level: "blocker", Message: "missing", Fixable: true, Action: "run repo autofix --write to generate platform CI include"}
		}
		return repoPolicyItem{Name: "gitlab_ci", Path: ".gitlab-ci.yml", Status: "fail", Level: "blocker", Message: err.Error(), Fixable: false, Action: "fix .gitlab-ci.yml filesystem error"}
	}
	text := string(body)
	template := "/ci/templates/buildkit-gitops." + cfg.Language + ".yml"
	if strings.Contains(text, template) {
		return repoPolicyItem{Name: "gitlab_ci", Path: ".gitlab-ci.yml", Status: "pass", Level: "info", Message: "platform template include detected"}
	}
	if strings.Contains(text, "buildctl-daemonless.sh") || strings.Contains(text, "BUILDKIT_IMAGE") {
		return repoPolicyItem{Name: "gitlab_ci", Path: ".gitlab-ci.yml", Status: "warn", Level: "warning", Message: "direct BuildKit CI detected; platform include is preferred", Fixable: true, Action: "run repo autofix --write --force to replace CI with platform include"}
	}
	return repoPolicyItem{Name: "gitlab_ci", Path: ".gitlab-ci.yml", Status: "fail", Level: "blocker", Message: "platform BuildKit/GitOps template not detected", Fixable: true, Action: "run repo autofix --write --force to replace CI with platform include"}
}

func checkRepoFile(name, path, action string) repoPolicyItem {
	if _, err := os.Stat(path); err == nil {
		return repoPolicyItem{Name: name, Path: path, Status: "pass", Level: "info", Message: "present"}
	} else if err != nil && !os.IsNotExist(err) {
		return repoPolicyItem{Name: name, Path: path, Status: "fail", Level: "blocker", Message: err.Error(), Fixable: false, Action: "fix manifest filesystem error"}
	}
	return repoPolicyItem{Name: name, Path: path, Status: "fail", Level: "blocker", Message: "missing", Fixable: true, Action: action}
}

func checkRepoQuality() repoPolicyItem {
	path := filepath.Join(".opspilot", "quality.yaml")
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

func checkRepoDeployment(cfg onboardServiceConfig) repoPolicyItem {
	path := filepath.Join("deploy", "k8s", "deployment.yaml")
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

func codePrecheck(project string) (codePrecheckResult, error) {
	detected, err := detectOnboardRepository(project, "opspilot.namespaces.yaml")
	if err != nil {
		return codePrecheckResult{}, err
	}
	cfg := detected.Config
	if err := cfg.defaults(); err != nil {
		return codePrecheckResult{}, err
	}
	items, err := scanCodePrecheckItems()
	if err != nil {
		return codePrecheckResult{}, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		if severityRank(items[i].Severity) != severityRank(items[j].Severity) {
			return severityRank(items[i].Severity) < severityRank(items[j].Severity)
		}
		if items[i].Path != items[j].Path {
			return items[i].Path < items[j].Path
		}
		return items[i].Line < items[j].Line
	})
	result := codePrecheckResult{
		Service: cfg.Name,
		Project: cfg.GitLabProject,
		Status:  "pass",
		Ready:   true,
		Items:   items,
		Skills: []string{
			"code-reviewer",
			"security-reviewer",
			"secure-code-guardian",
			"database-optimizer",
			"debugging-wizard",
		},
	}
	for _, item := range items {
		switch item.Severity {
		case "blocker":
			result.Summary.Blockers++
		case "warning":
			result.Summary.Warnings++
		default:
			result.Summary.Passed++
		}
	}
	switch {
	case result.Summary.Blockers > 0:
		result.Ready = false
		result.Status = "blocker"
		result.Next = []string{
			"ask OpsPilot to explain code precheck blockers",
			"fix blocker findings before BuildKit packaging",
		}
	case result.Summary.Warnings > 0:
		result.Status = "warn"
		result.Next = []string{"review warning findings after release or ask OpsPilot for a fix plan"}
	default:
		result.Next = []string{"continue to language tests and BuildKit packaging"}
	}
	return result, nil
}

func writeCodePrecheck(out io.Writer, output string, result codePrecheckResult) error {
	return writeOutput(out, output, result, func(w io.Writer) error {
		fmt.Fprintf(w, "Code precheck: %s status=%s ready=%t blockers=%d warnings=%d\n",
			result.Service, result.Status, result.Ready, result.Summary.Blockers, result.Summary.Warnings)
		if result.EvidencePath != "" {
			fmt.Fprintf(w, "Evidence: %s\n", result.EvidencePath)
		}
		if len(result.Items) > 0 {
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "SEVERITY\tCATEGORY\tFILE\tLINE\tSKILL\tMESSAGE")
			for _, item := range result.Items {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n", item.Severity, item.Category, item.Path, item.Line, item.Skill, item.Message)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
		}
		if len(result.Next) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(result.Next, "; "))
		}
		return nil
	})
}

func writeCodePrecheckEvidence(result codePrecheckResult) error {
	path := codePrecheckEvidencePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	result.EvidencePath = path
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

func codePrecheckEvidencePath() string {
	return filepath.Join(".opspilot", "evidence", "code-precheck.json")
}

func scanCodePrecheckItems() ([]codePrecheckItem, error) {
	items := []codePrecheckItem{}
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if shouldSkipCodePrecheckDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		if !shouldScanCodePrecheckFile(path) {
			return nil
		}
		body, ok := readSmallTextFile(path)
		if !ok {
			return nil
		}
		items = append(items, scanCodePrecheckText(filepath.ToSlash(path), string(body))...)
		return nil
	})
	return dedupeCodePrecheckItems(items), err
}

func shouldSkipCodePrecheckDir(name string) bool {
	switch name {
	case ".git", ".opspilot", "node_modules", "vendor", "dist", "build", "target", ".next", ".venv", "venv", "__pycache__", "coverage", ".pytest_cache", "ci", "docs", "gitops-manifests-work":
		return true
	default:
		return false
	}
}

func shouldScanCodePrecheckFile(path string) bool {
	slashPath := filepath.ToSlash(path)
	if strings.HasPrefix(slashPath, "deploy/") ||
		strings.HasPrefix(slashPath, "ci/") ||
		strings.HasPrefix(slashPath, "docs/") ||
		strings.Contains(slashPath, "/test/") ||
		strings.Contains(slashPath, "/tests/") ||
		strings.Contains(slashPath, "/fixtures/") {
		return false
	}
	base := strings.ToLower(filepath.Base(path))
	if base == ".gitlab-ci.yml" ||
		strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, ".test.js") ||
		strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, ".spec.js") ||
		strings.HasSuffix(base, ".spec.ts") {
		return false
	}
	switch base {
	case "dockerfile", ".env", ".env.example", "application.yml", "application.yaml", "application.properties":
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".js", ".jsx", ".ts", ".tsx", ".py", ".java", ".kt", ".php", ".cs", ".sql", ".yml", ".yaml", ".properties", ".toml":
		return true
	default:
		return false
	}
}

func scanCodePrecheckText(path, text string) []codePrecheckItem {
	items := []codePrecheckItem{}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		items = append(items, codePrecheckLineFindings(path, lineNo, trimmed)...)
		if n1 := codePrecheckNPlusOne(path, lineNo, lines, i); n1 != nil {
			items = append(items, *n1)
		}
	}
	return items
}

func codePrecheckLineFindings(path string, lineNo int, line string) []codePrecheckItem {
	items := []codePrecheckItem{}
	lower := strings.ToLower(line)
	normalizedSQL := normalizeSQLLine(line)
	if isCodePrecheckRuleDefinitionLine(lower) {
		return items
	}
	switch {
	case looksLikeSecretLeak(line):
		items = append(items, codePrecheckFinding("secret_leak", "blocker", "security", path, lineNo, "possible hardcoded secret or token", line, "security-reviewer", "Move the value to GitLab/Kubernetes Secret and rotate it if it is real."))
	case containsDangerousShellLine(lower):
		items = append(items, codePrecheckFinding("dangerous_shell", "blocker", "security", path, lineNo, "dangerous shell execution detected", line, "security-reviewer", "Remove shell execution or replace it with a bounded, reviewed command without user input."))
	case containsDestructiveSQL(normalizedSQL):
		items = append(items, codePrecheckFinding("db_destructive_sql", "blocker", "database", path, lineNo, "destructive SQL detected", line, "database-optimizer", "Move destructive operations to an explicit migration/admin workflow with safeguards."))
	case containsUnguardedWriteSQL(normalizedSQL):
		items = append(items, codePrecheckFinding("db_unguarded_write", "blocker", "database", path, lineNo, "UPDATE/DELETE without a visible WHERE guard", line, "database-optimizer", "Add a guarded WHERE condition and verify affected rows before committing."))
	case containsQueryHandlerWrite(lower):
		items = append(items, codePrecheckFinding("api_query_writes_data", "blocker", "code", path, lineNo, "query-style handler appears to write data", line, "code-reviewer", "Separate read and write paths or change the route/handler semantics."))
	case containsUnboundedFileWrite(lower):
		items = append(items, codePrecheckFinding("unbounded_file_write", "blocker", "storage", path, lineNo, "possible unbounded write to logs/uploads/runtime path", line, "code-reviewer", "Add file size, type, retention, and path validation before writing."))
	case containsFullTableRead(normalizedSQL):
		items = append(items, codePrecheckFinding("db_full_table_read", "warning", "database", path, lineNo, "possible full-table read without pagination or filtering", line, "database-optimizer", "Add WHERE, LIMIT, or pagination and verify indexes."))
	case containsSelectStar(normalizedSQL):
		items = append(items, codePrecheckFinding("db_select_star", "warning", "database", path, lineNo, "SELECT * needs review", line, "database-optimizer", "Select only needed columns and confirm the query is bounded."))
	case containsRawSQLConstruction(line):
		items = append(items, codePrecheckFinding("raw_sql_construction", "warning", "security", path, lineNo, "raw SQL string construction needs review", line, "secure-code-guardian", "Use parameterized queries or ORM placeholders."))
	case containsMissingTimeoutHint(lower):
		items = append(items, codePrecheckFinding("missing_timeout_hint", "warning", "reliability", path, lineNo, "outbound client call may need an explicit timeout", line, "code-reviewer", "Configure request/database/client timeout to avoid stuck workers."))
	}
	return items
}

func isCodePrecheckRuleDefinitionLine(lower string) bool {
	return strings.Contains(lower, "codeprecheckfinding(") ||
		strings.Contains(lower, "regexp.mustcompile(") ||
		strings.Contains(lower, "containsany(") ||
		strings.Contains(lower, "strings.contains(lower,") ||
		strings.Contains(lower, "containsdangerousshellline(") ||
		strings.Contains(lower, "add('") ||
		strings.Contains(lower, "add(\"")
}

func codePrecheckFinding(id, severity, category, path string, line int, message, snippet, skill, recommendation string) codePrecheckItem {
	return codePrecheckItem{
		ID:             id,
		Severity:       severity,
		Category:       category,
		Path:           path,
		Line:           line,
		Message:        message,
		Snippet:        truncateSnippet(snippet),
		Skill:          skill,
		Recommendation: recommendation,
	}
}

func codePrecheckNPlusOne(path string, lineNo int, lines []string, index int) *codePrecheckItem {
	lower := strings.ToLower(strings.TrimSpace(lines[index]))
	if !strings.HasPrefix(lower, "for ") && !strings.HasPrefix(lower, "for(") && !strings.HasPrefix(lower, "while ") {
		return nil
	}
	end := index + 8
	if end > len(lines) {
		end = len(lines)
	}
	window := strings.ToLower(strings.Join(lines[index:end], "\n"))
	if containsAny(window, []string{".query(", ".find(", ".findall(", ".filter(", "select ", "jdbc", "gorm.", "db."}) {
		item := codePrecheckFinding("possible_n_plus_one", "warning", "database", path, lineNo, "loop contains database-like access", lines[index], "database-optimizer", "Batch the query, prefetch relations, or move database access outside the loop.")
		return &item
	}
	return nil
}

func normalizeSQLLine(line string) string {
	return strings.Join(strings.Fields(strings.ToLower(line)), " ")
}

func looksLikeSecretLeak(line string) bool {
	lower := strings.ToLower(line)
	if !containsAny(lower, []string{"password", "passwd", "secret", "token", "access_key", "accesskey", "api_key", "apikey", "private_key"}) {
		return false
	}
	if !strings.Contains(line, "=") && !strings.Contains(line, ":") {
		return false
	}
	if containsAny(lower, []string{"${", "$env", "env.", "env(", "getenv", "os.getenv", "secretref", "valuefrom", "example", "placeholder", "changeme", "your_", "<", "xxx", "token_or_api", "missing", "source_skill_path", "image_pull_secret", "pull_secret", "settings.", "qualitysettings."}) {
		return false
	}
	value := valueAfterAssignment(line)
	if value == "" || strings.ContainsAny(value, "()[]{}") {
		return false
	}
	return looksLikeSecretLiteral(value) || strings.Contains(lower, "private_key")
}

func valueAfterAssignment(line string) string {
	idx := strings.IndexAny(line, "=:")
	if idx < 0 {
		return ""
	}
	value := strings.TrimSpace(line[idx+1:])
	value = strings.Trim(value, `"', `)
	return value
}

func looksLikeSecretLiteral(value string) bool {
	value = strings.Trim(value, `"'`)
	if len(value) < 16 || strings.Contains(value, " ") {
		return false
	}
	if strings.HasPrefix(value, "glpat-") || strings.HasPrefix(value, "sk-") {
		return true
	}
	return regexp.MustCompile(`^[A-Za-z0-9_./+=:-]{16,}$`).MatchString(value)
}

func containsDangerousShellLine(lower string) bool {
	return strings.Contains(lower, "rm -rf /") ||
		((strings.Contains(lower, "curl ") || strings.Contains(lower, "wget ")) && (strings.Contains(lower, "| sh") || strings.Contains(lower, "| bash"))) ||
		strings.Contains(lower, "mkfs.") ||
		strings.Contains(lower, ":(){ :|:& };:")
}

func containsDestructiveSQL(sql string) bool {
	return regexp.MustCompile(`\b(drop|truncate)\s+(table|database|schema)\b`).MatchString(sql)
}

func containsUnguardedWriteSQL(sql string) bool {
	if !regexp.MustCompile(`\b(update\s+[a-zA-Z0-9_."` + "`" + `]+\s+set|delete\s+from\s+[a-zA-Z0-9_."` + "`" + `]+)\b`).MatchString(sql) {
		return false
	}
	return !strings.Contains(sql, " where ")
}

func containsQueryHandlerWrite(lower string) bool {
	return containsAny(lower, []string{"query", "search", "list", "get"}) &&
		containsAny(lower, []string{".save(", ".create(", ".insert(", ".update(", ".delete(", "delete from", "update "})
}

func containsUnboundedFileWrite(lower string) bool {
	return containsAny(lower, []string{"writefile", "create(", "openfile", "write("}) &&
		containsAny(lower, []string{"/logs", "/upload", "/uploads", "/runtime", "log_dir", "upload_dir", "runtime_dir"})
}

func containsFullTableRead(sql string) bool {
	if !regexp.MustCompile(`\b(select|findall|find_all|all\(\))\b`).MatchString(sql) {
		return false
	}
	if containsAny(sql, []string{" where ", " limit ", " offset ", " page", "paginate", "take(", "skip("}) {
		return false
	}
	return regexp.MustCompile(`\bfrom\s+[a-zA-Z0-9_]+`).MatchString(sql) || containsAny(sql, []string{"findall(", "find_all(", ".all()"})
}

func containsSelectStar(sql string) bool {
	return strings.Contains(sql, "select *") && !containsAny(sql, []string{" limit ", " where "})
}

func containsRawSQLConstruction(line string) bool {
	lower := strings.ToLower(line)
	if !containsAny(lower, []string{"select ", "update ", "delete ", "insert "}) {
		return false
	}
	return strings.Contains(line, "fmt.Sprintf") ||
		strings.Contains(line, "f\"") ||
		strings.Contains(line, "${") ||
		strings.Contains(line, "+")
}

func containsMissingTimeoutHint(lower string) bool {
	if containsAny(lower, []string{"timeout", "withtimeout", "context.withtimeout"}) {
		return false
	}
	return containsAny(lower, []string{"http.get(", "http.post(", "requests.get(", "requests.post(", "new resttemplate", "webclient.create("})
}

func truncateSnippet(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= 160 {
		return value
	}
	return value[:160]
}

func dedupeCodePrecheckItems(items []codePrecheckItem) []codePrecheckItem {
	seen := map[string]bool{}
	out := []codePrecheckItem{}
	for _, item := range items {
		key := fmt.Sprintf("%s:%s:%d", item.ID, item.Path, item.Line)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func severityRank(value string) int {
	switch value {
	case "blocker":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
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

func containsAny(text string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
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
