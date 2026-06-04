package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type onboardServiceConfig struct {
	Name           string                      `json:"name"`
	GitLabProject  string                      `json:"gitlab_project"`
	Organization   string                      `json:"organization,omitempty"`
	Group          string                      `json:"group,omitempty"`
	Project        string                      `json:"project,omitempty"`
	Language       string                      `json:"language"`
	BuildEntry     string                      `json:"build_entry"`
	BuildOutput    string                      `json:"build_output"`
	Port           int                         `json:"port"`
	HealthPath     string                      `json:"health_path"`
	Namespace      string                      `json:"namespace"`
	NamespaceSrc   string                      `json:"namespace_source,omitempty"`
	Replicas       int                         `json:"replicas"`
	Container      string                      `json:"container"`
	DockerMode     string                      `json:"dockerfile_mode"`
	DockerPath     string                      `json:"dockerfile_path"`
	CIMode         string                      `json:"ci_mode"`
	PromSource     string                      `json:"prometheus_source"`
	Resources      onboardResourcesConfig      `json:"resources"`
	NamespaceGuard onboardNamespaceGuardConfig `json:"namespace_guard"`
	Middleware     []onboardMiddlewareConfig   `json:"middleware,omitempty"`
	Storage        []onboardStorageConfig      `json:"storage,omitempty"`
}

type onboardResourcesConfig struct {
	Profile       string `json:"profile"`
	RequestCPU    string `json:"request_cpu"`
	RequestMemory string `json:"request_memory"`
	LimitCPU      string `json:"limit_cpu"`
	LimitMemory   string `json:"limit_memory"`
}

type onboardNamespaceGuardConfig struct {
	LimitRange     bool   `json:"limit_range"`
	ResourceQuota  bool   `json:"resource_quota"`
	RequestsCPU    string `json:"requests_cpu"`
	RequestsMemory string `json:"requests_memory"`
	LimitsCPU      string `json:"limits_cpu"`
	LimitsMemory   string `json:"limits_memory"`
	Pods           string `json:"pods"`
}

type onboardMiddlewareConfig struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Display    string   `json:"display"`
	Mode       string   `json:"mode"`
	Allocation string   `json:"allocation"`
	Provision  string   `json:"provision,omitempty"`
	Resource   string   `json:"resource"`
	Secret     string   `json:"secret"`
	Env        []string `json:"env"`
	Reason     string   `json:"reason,omitempty"`
	Evidence   []string `json:"evidence,omitempty"`
}

type onboardStorageConfig struct {
	Name          string   `json:"name"`
	Purpose       string   `json:"purpose"`
	Mode          string   `json:"mode"`
	MountPath     string   `json:"mount_path"`
	HostPath      string   `json:"host_path,omitempty"`
	SizeHint      string   `json:"size_hint,omitempty"`
	SizeLimit     string   `json:"size_limit,omitempty"`
	RetentionDays int      `json:"retention_days,omitempty"`
	ReadOnly      bool     `json:"read_only,omitempty"`
	Reason        string   `json:"reason,omitempty"`
	Evidence      []string `json:"evidence,omitempty"`
}

type middlewareCatalogEntry struct {
	Kind       string
	Display    string
	Mode       string
	Allocation string
	Env        []string
	Tokens     []string
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
	GitOpsPlan     onboardGitOpsPlan    `json:"gitops_plan"`
}

type onboardRepoResult struct {
	Service         string               `json:"service"`
	Environment     string               `json:"environment"`
	Repo            string               `json:"repo"`
	Project         string               `json:"project"`
	Mode            string               `json:"mode"`
	Ready           bool                 `json:"ready"`
	Language        string               `json:"language"`
	Namespace       string               `json:"namespace"`
	Port            int                  `json:"port"`
	Config          onboardServiceConfig `json:"config"`
	Files           []onboardWriteResult `json:"files"`
	Preflight       *onboardCheckResult  `json:"preflight,omitempty"`
	Gaps            []string             `json:"gaps,omitempty"`
	Next            []string             `json:"next,omitempty"`
	ReleaseMapping  string               `json:"release_mapping"`
	GitOpsPlan      onboardGitOpsPlan    `json:"gitops_plan"`
	CredentialPlans []string             `json:"credential_plans,omitempty"`
}

type onboardGitOpsPlan struct {
	Cluster         string   `json:"cluster"`
	Path            string   `json:"path"`
	DeploymentPath  string   `json:"deployment_path"`
	ApplicationName string   `json:"application_name"`
	Namespace       string   `json:"namespace"`
	Image           string   `json:"image"`
	StandardFlow    []string `json:"standard_flow"`
}

func onboardCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected: onboard repo, service, check, detect, or generate")
	}
	switch args[0] {
	case "repo":
		return onboardRepoCommand(opts, args[1:], out)
	case "service":
		return onboardServiceCommand(args[1:], out)
	case "check":
		return onboardCheckCommand(args[1:], out)
	case "detect":
		return onboardDetectCommand(args[1:], out)
	case "generate":
		return onboardGenerateCommand(args[1:], out)
	default:
		return fmt.Errorf("expected: onboard repo, service, check, detect, or generate")
	}
}

func onboardRepoCommand(opts globalOptions, args []string, out io.Writer) error {
	positionalProject := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalProject = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("onboard repo", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	project := fs.String("project", "", "GitLab project path, for example tpo/devex/skillshub/skillshub-api")
	catalog := fs.String("namespace-catalog", "opspilot.namespaces.yaml", "namespace catalog path")
	envName := fs.String("env", "test", "target environment")
	write := fs.Bool("write", false, "write generated files")
	force := fs.Bool("force", false, "overwrite existing generated files")
	_ = fs.Parse(args)
	if *project == "" {
		*project = positionalProject
	}
	if *project == "" && fs.NArg() > 0 {
		*project = fs.Arg(0)
	}
	if *project == "" {
		return fmt.Errorf("onboard repo requires a GitLab project path")
	}
	result, err := withRepo(*repo, func() (onboardRepoResult, error) {
		detected, err := detectOnboardRepository(*project, *catalog)
		if err != nil {
			return onboardRepoResult{}, err
		}
		cfg := detected.Config
		if err := cfg.defaults(); err != nil {
			return onboardRepoResult{}, err
		}
		files := append([]generatedFile{{path: "opspilot.service.yaml", body: serviceConfigTemplate(cfg)}}, onboardFiles(cfg)...)
		results := make([]onboardWriteResult, 0, len(files))
		for _, file := range files {
			action := "planned"
			if *write {
				action, err = writeGeneratedFile(file.path, file.body, *force)
				if err != nil {
					return onboardRepoResult{}, err
				}
			}
			results = append(results, onboardWriteResult{Path: file.path, Action: action})
		}
		ready := detected.Ready
		gaps := append([]string{}, detected.Gaps...)
		next := append([]string{}, detected.Next...)
		var preflight *onboardCheckResult
		if *write {
			check := checkOnboardRepository(cfg)
			preflight = &check
			ready = check.Ready
			gaps = append([]string{}, check.Missing...)
			next = append([]string{}, check.Next...)
		} else {
			next = append([]string{"rerun with --write to generate platform files"}, next...)
		}
		if ready {
			next = append(next,
				"push the repository to GitLab",
				"wait for GitLab Runner -> BuildKit -> GitOps -> Argo CD",
				"run opspilot inspect service "+cfg.Name+" --output human",
			)
		}
		return onboardRepoResult{
			Service:         cfg.Name,
			Environment:     *envName,
			Repo:            *repo,
			Project:         cfg.GitLabProject,
			Mode:            writeMode(*write),
			Ready:           ready,
			Language:        cfg.Language,
			Namespace:       cfg.Namespace,
			Port:            cfg.Port,
			Config:          cfg,
			Files:           results,
			Preflight:       preflight,
			Gaps:            uniqueStrings(gaps),
			Next:            uniqueStrings(next),
			ReleaseMapping:  releaseMapping(cfg),
			GitOpsPlan:      gitOpsPlan(cfg),
			CredentialPlans: middlewareCredentialPlans(cfg),
		}, nil
	})
	if err != nil {
		return err
	}
	writeErr := writeOutput(out, opts.output, result, func(w io.Writer) error {
		fmt.Fprintf(w, "Onboard: %s env=%s ready=%t mode=%s\n", result.Service, result.Environment, result.Ready, result.Mode)
		fmt.Fprintf(w, "Repo: %s\n", result.Repo)
		fmt.Fprintf(w, "Project: %s\n", result.Project)
		fmt.Fprintf(w, "Detected: language=%s port=%d namespace=%s\n", result.Language, result.Port, result.Namespace)
		if len(result.Gaps) > 0 {
			fmt.Fprintf(w, "Gaps: %s\n", strings.Join(result.Gaps, ", "))
		}
		if len(result.Files) > 0 {
			fmt.Fprintln(w, "Files:")
			for _, file := range result.Files {
				fmt.Fprintf(w, "  %s\t%s\n", file.Action, file.Path)
			}
		}
		if result.ReleaseMapping != "" {
			fmt.Fprintf(w, "Release mapping: %s\n", result.ReleaseMapping)
		}
		fmt.Fprintf(w, "GitOps: path=%s app=%s image=%s\n", result.GitOpsPlan.Path, result.GitOpsPlan.ApplicationName, result.GitOpsPlan.Image)
		if len(result.CredentialPlans) > 0 {
			fmt.Fprintf(w, "Credential plans: %s\n", strings.Join(result.CredentialPlans, "; "))
		}
		if len(result.Next) > 0 {
			fmt.Fprintf(w, "Next: %s\n", strings.Join(result.Next, "; "))
		}
		return nil
	})
	if writeErr != nil {
		return writeErr
	}
	if result.Mode == "write" && !result.Ready {
		return fmt.Errorf("onboard repo generated files but repository is not release-ready")
	}
	return nil
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
	defaultOrganization      = "tpo"
	defaultGroup             = "devex"
	defaultNamespacePrefix   = "cicd"
	defaultResourceProfile   = "small"
	defaultCITemplateProject = "platform/opspilot"
	defaultHostPathRoot      = "/data/opspilot/hostpath"
)

var resourceProfiles = map[string]onboardResourcesConfig{
	"small": {
		Profile:       "small",
		RequestCPU:    "50m",
		RequestMemory: "64Mi",
		LimitCPU:      "500m",
		LimitMemory:   "256Mi",
	},
	"medium": {
		Profile:       "medium",
		RequestCPU:    "100m",
		RequestMemory: "128Mi",
		LimitCPU:      "1",
		LimitMemory:   "512Mi",
	},
	"large": {
		Profile:       "large",
		RequestCPU:    "500m",
		RequestMemory: "512Mi",
		LimitCPU:      "2",
		LimitMemory:   "2Gi",
	},
}

var defaultNamespaceGuard = onboardNamespaceGuardConfig{
	LimitRange:     true,
	ResourceQuota:  true,
	RequestsCPU:    "4",
	RequestsMemory: "8Gi",
	LimitsCPU:      "8",
	LimitsMemory:   "16Gi",
	Pods:           "50",
}

var projectSuffixes = map[string]bool{
	"admin":   true,
	"api":     true,
	"core":    true,
	"job":     true,
	"service": true,
	"web":     true,
	"worker":  true,
}

var middlewareCatalog = []middlewareCatalogEntry{
	{
		Kind:       "mysql",
		Display:    "MySQL database",
		Mode:       "shared-database",
		Allocation: "database-user",
		Env:        []string{"DATABASE_URL"},
		Tokens: []string{
			"go-sql-driver/mysql", "mysql2", "mysqlclient", "pymysql", "mysql-connector",
			"jdbc:mysql", "mysql_", "mysql://",
		},
	},
	{
		Kind:       "postgres",
		Display:    "PostgreSQL database",
		Mode:       "shared-database",
		Allocation: "database-user-schema",
		Env:        []string{"DATABASE_URL"},
		Tokens: []string{
			"lib/pq", "pgx", "psycopg", "asyncpg", "node-postgres", "postgresql",
			"jdbc:postgresql", "postgres://", "postgres_", "pghost",
		},
	},
	{
		Kind:       "redis",
		Display:    "Redis cache",
		Mode:       "shared-cache",
		Allocation: "key-prefix",
		Env:        []string{"REDIS_URL"},
		Tokens: []string{
			"go-redis", "ioredis", "redis-py", "redis_url", "redis.host",
			"redis_host", "redis://",
		},
	},
	{
		Kind:       "rabbitmq",
		Display:    "RabbitMQ message queue",
		Mode:       "shared-broker",
		Allocation: "vhost-user",
		Env:        []string{"AMQP_URL"},
		Tokens: []string{
			"rabbitmq", "amqplib", "pika", "spring.rabbitmq", "amqp_url", "rabbitmq_url",
			"amqp://",
		},
	},
	{
		Kind:       "s3",
		Display:    "S3 compatible object storage",
		Mode:       "shared-object-storage",
		Allocation: "bucket-access-key",
		Env:        []string{"S3_ENDPOINT", "S3_BUCKET", "S3_ACCESS_KEY", "S3_SECRET_KEY"},
		Tokens: []string{
			"minio", "boto3", "@aws-sdk/client-s3", "aws-sdk", "s3_endpoint", "s3_bucket",
			"aws_access_key_id",
		},
	},
	{
		Kind:       "opensearch",
		Display:    "OpenSearch/Elasticsearch search",
		Mode:       "shared-search",
		Allocation: "index-prefix",
		Env:        []string{"OPENSEARCH_URL"},
		Tokens: []string{
			"opensearch", "elasticsearch", "elastic_client", "elasticsearch_url",
			"opensearch_url",
		},
	},
	{
		Kind:       "kafka",
		Display:    "Kafka streaming",
		Mode:       "shared-streaming",
		Allocation: "topic-prefix-acl",
		Env:        []string{"KAFKA_BROKERS"},
		Tokens: []string{
			"kafka", "sarama", "confluent-kafka", "kafka_brokers", "spring.kafka",
		},
	},
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
		generatedFile{path: filepath.Join("deploy", "k8s", "limitrange.yaml"), body: limitRangeTemplate(cfg)},
		generatedFile{path: filepath.Join("deploy", "k8s", "resourcequota.yaml"), body: resourceQuotaTemplate(cfg)},
		generatedFile{path: filepath.Join("deploy", "k8s", "serviceaccount.yaml"), body: serviceAccountTemplate(cfg)},
		generatedFile{path: filepath.Join("deploy", "k8s", "deployment.yaml"), body: deploymentTemplate(cfg)},
		generatedFile{path: filepath.Join("deploy", "k8s", "service.yaml"), body: serviceTemplate(cfg)},
		generatedFile{path: filepath.Join("deploy", "k8s", "kustomization.yaml"), body: kustomizationTemplate(cfg)},
		generatedFile{path: filepath.Join(".opspilot", "quality.yaml"), body: qualityTemplate(cfg)},
		generatedFile{path: "opspilot.release-service.txt", body: releaseMapping(cfg) + "\n"},
	)
	for _, item := range cfg.Middleware {
		if middlewareAutoProvisioned(item) {
			files = append(files, generatedFile{
				path: filepath.Join("deploy", "k8s", "middleware-"+item.Name+".yaml"),
				body: middlewareTemplate(cfg, item),
			})
		}
	}
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
	base := fmt.Sprintf(`name: %s
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

resources:
  profile: %s
  requests:
    cpu: %s
    memory: %s
  limits:
    cpu: %s
    memory: %s

namespaceGuard:
  limitRange: %t
  resourceQuota: %t
  quota:
    requestsCpu: %s
    requestsMemory: %s
    limitsCpu: %s
    limitsMemory: %s
    pods: %s

dockerfile:
  mode: %s
  path: %s

ci:
  mode: %s

%s
%s
release:
  prometheusSource: %s
`, c.Name, c.GitLabProject, c.Organization, c.Group, c.Project, c.Language, c.BuildEntry, c.BuildOutput, c.Port, c.HealthPath, c.Namespace, firstNonEmpty(c.NamespaceSrc, "manual"), c.Replicas, c.Container, c.Resources.Profile, c.Resources.RequestCPU, c.Resources.RequestMemory, c.Resources.LimitCPU, c.Resources.LimitMemory, c.NamespaceGuard.LimitRange, c.NamespaceGuard.ResourceQuota, c.NamespaceGuard.RequestsCPU, c.NamespaceGuard.RequestsMemory, c.NamespaceGuard.LimitsCPU, c.NamespaceGuard.LimitsMemory, c.NamespaceGuard.Pods, c.DockerMode, c.DockerPath, c.CIMode, middlewareConfigTemplate(c.Middleware), storageConfigTemplate(c.Storage), c.PromSource)
	return base
}

func middlewareConfigTemplate(items []onboardMiddlewareConfig) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("middleware:\n")
	for _, item := range items {
		b.WriteString(fmt.Sprintf("  %s:\n", item.Name))
		b.WriteString(fmt.Sprintf("    kind: %s\n", item.Kind))
		b.WriteString(fmt.Sprintf("    display: %s\n", item.Display))
		b.WriteString(fmt.Sprintf("    mode: %s\n", item.Mode))
		b.WriteString(fmt.Sprintf("    allocation: %s\n", item.Allocation))
		b.WriteString(fmt.Sprintf("    provision: %s\n", firstNonEmpty(item.Provision, "external")))
		b.WriteString(fmt.Sprintf("    resource: %s\n", item.Resource))
		b.WriteString(fmt.Sprintf("    secret: %s\n", item.Secret))
		b.WriteString(fmt.Sprintf("    env: %s\n", strings.Join(item.Env, ",")))
		if item.Reason != "" {
			b.WriteString(fmt.Sprintf("    reason: %s\n", item.Reason))
		}
	}
	return b.String()
}

func storageConfigTemplate(items []onboardStorageConfig) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("storage:\n")
	for _, item := range items {
		b.WriteString(fmt.Sprintf("  %s:\n", item.Name))
		b.WriteString(fmt.Sprintf("    purpose: %s\n", item.Purpose))
		b.WriteString(fmt.Sprintf("    mode: %s\n", item.Mode))
		b.WriteString(fmt.Sprintf("    mountPath: %s\n", item.MountPath))
		if item.HostPath != "" {
			b.WriteString(fmt.Sprintf("    hostPath: %s\n", item.HostPath))
		}
		if item.SizeHint != "" {
			b.WriteString(fmt.Sprintf("    sizeHint: %s\n", item.SizeHint))
		}
		if item.SizeLimit != "" {
			b.WriteString(fmt.Sprintf("    sizeLimit: %s\n", item.SizeLimit))
		}
		if item.RetentionDays > 0 {
			b.WriteString(fmt.Sprintf("    retentionDays: %d\n", item.RetentionDays))
		}
		if item.ReadOnly {
			b.WriteString("    readOnly: true\n")
		}
		if item.Reason != "" {
			b.WriteString(fmt.Sprintf("    reason: %s\n", item.Reason))
		}
	}
	return b.String()
}

func readOnboardServiceConfig(path string) (onboardServiceConfig, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return onboardServiceConfig{}, err
	}
	values := parseSimpleYAML(string(body))
	cfg := onboardServiceConfig{
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
		Resources: onboardResourcesConfig{
			Profile:       values["resources.profile"],
			RequestCPU:    values["resources.requests.cpu"],
			RequestMemory: values["resources.requests.memory"],
			LimitCPU:      values["resources.limits.cpu"],
			LimitMemory:   values["resources.limits.memory"],
		},
		NamespaceGuard: onboardNamespaceGuardConfig{
			LimitRange:     boolFromString(values["namespaceGuard.limitRange"], false),
			ResourceQuota:  boolFromString(values["namespaceGuard.resourceQuota"], false),
			RequestsCPU:    values["namespaceGuard.quota.requestsCpu"],
			RequestsMemory: values["namespaceGuard.quota.requestsMemory"],
			LimitsCPU:      values["namespaceGuard.quota.limitsCpu"],
			LimitsMemory:   values["namespaceGuard.quota.limitsMemory"],
			Pods:           values["namespaceGuard.quota.pods"],
		},
	}
	cfg.Middleware = middlewareFromValues(values)
	cfg.Storage = storageFromValues(values)
	return cfg, nil
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
		c.Port = defaultPortForLanguage(c.Language)
	}
	if c.HealthPath == "" {
		c.HealthPath = defaultHealthPathForLanguage(c.Language)
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
	c.Resources = defaultResources(c.Resources)
	c.NamespaceGuard = defaultNamespaceGuardConfig(c.NamespaceGuard)
	c.Middleware = normalizeMiddlewareRequirements(*c, c.Middleware)
	c.Storage = normalizeStorageRequirements(*c, c.Storage)
	return nil
}

func parseSimpleYAML(raw string) map[string]string {
	out := map[string]string{}
	type frame struct {
		indent int
		key    string
	}
	stack := []frame{}
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
		for len(stack) > 0 && indent <= stack[len(stack)-1].indent {
			stack = stack[:len(stack)-1]
		}
		if value == "" {
			stack = append(stack, frame{indent: indent, key: key})
			continue
		}
		parts := make([]string, 0, len(stack)+1)
		for _, part := range stack {
			parts = append(parts, part.key)
		}
		parts = append(parts, key)
		out[strings.Join(parts, ".")] = value
	}
	return out
}

func middlewareFromValues(values map[string]string) []onboardMiddlewareConfig {
	names := map[string]bool{}
	for key := range values {
		if !strings.HasPrefix(key, "middleware.") {
			continue
		}
		rest := strings.TrimPrefix(key, "middleware.")
		name, _, ok := strings.Cut(rest, ".")
		if ok && name != "" {
			names[name] = true
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	items := []onboardMiddlewareConfig{}
	for _, name := range ordered {
		prefix := "middleware." + name + "."
		items = append(items, onboardMiddlewareConfig{
			Name:       name,
			Kind:       values[prefix+"kind"],
			Display:    values[prefix+"display"],
			Mode:       values[prefix+"mode"],
			Allocation: values[prefix+"allocation"],
			Provision:  values[prefix+"provision"],
			Resource:   values[prefix+"resource"],
			Secret:     values[prefix+"secret"],
			Env:        splitCSV(values[prefix+"env"]),
			Reason:     values[prefix+"reason"],
		})
	}
	return items
}

func storageFromValues(values map[string]string) []onboardStorageConfig {
	names := map[string]bool{}
	for key := range values {
		if !strings.HasPrefix(key, "storage.") {
			continue
		}
		rest := strings.TrimPrefix(key, "storage.")
		name, _, ok := strings.Cut(rest, ".")
		if ok && name != "" {
			names[name] = true
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	items := []onboardStorageConfig{}
	for _, name := range ordered {
		prefix := "storage." + name + "."
		items = append(items, onboardStorageConfig{
			Name:          name,
			Purpose:       values[prefix+"purpose"],
			Mode:          values[prefix+"mode"],
			MountPath:     values[prefix+"mountPath"],
			HostPath:      values[prefix+"hostPath"],
			SizeHint:      values[prefix+"sizeHint"],
			SizeLimit:     values[prefix+"sizeLimit"],
			RetentionDays: intFromString(values[prefix+"retentionDays"], 0),
			ReadOnly:      boolFromString(values[prefix+"readOnly"], false),
			Reason:        values[prefix+"reason"],
		})
	}
	return items
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

func boolFromString(raw string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	case "0", "false", "no", "off", "disabled":
		return false
	default:
		return fallback
	}
}

func defaultResources(current onboardResourcesConfig) onboardResourcesConfig {
	profile := strings.TrimSpace(current.Profile)
	if profile == "" {
		profile = defaultResourceProfile
	}
	base, ok := resourceProfiles[profile]
	if !ok {
		base = resourceProfiles[defaultResourceProfile]
		base.Profile = profile
	}
	if current.RequestCPU != "" {
		base.RequestCPU = current.RequestCPU
	}
	if current.RequestMemory != "" {
		base.RequestMemory = current.RequestMemory
	}
	if current.LimitCPU != "" {
		base.LimitCPU = current.LimitCPU
	}
	if current.LimitMemory != "" {
		base.LimitMemory = current.LimitMemory
	}
	return base
}

func defaultNamespaceGuardConfig(current onboardNamespaceGuardConfig) onboardNamespaceGuardConfig {
	base := defaultNamespaceGuard
	base.LimitRange = true
	base.ResourceQuota = true
	if current.RequestsCPU != "" {
		base.RequestsCPU = current.RequestsCPU
	}
	if current.RequestsMemory != "" {
		base.RequestsMemory = current.RequestsMemory
	}
	if current.LimitsCPU != "" {
		base.LimitsCPU = current.LimitsCPU
	}
	if current.LimitsMemory != "" {
		base.LimitsMemory = current.LimitsMemory
	}
	if current.Pods != "" {
		base.Pods = current.Pods
	}
	return base
}

func detectMiddlewareRequirements(c onboardServiceConfig) []onboardMiddlewareConfig {
	signals := collectRepoSignals()
	items := []onboardMiddlewareConfig{}
	for _, entry := range middlewareCatalog {
		evidence := middlewareEvidence(signals, entry.Tokens)
		if len(evidence) == 0 {
			continue
		}
		item := defaultMiddlewareRequirement(c, entry)
		item.Evidence = evidence
		item.Reason = fmt.Sprintf("detected %s dependency; use %s and allocate %s", entry.Display, entry.Mode, entry.Allocation)
		items = append(items, item)
	}
	return items
}

func detectStorageRequirements(c onboardServiceConfig) []onboardStorageConfig {
	signals := collectRepoSignals()
	items := []onboardStorageConfig{}
	if evidence := storageEvidence(signals, []string{"LOG_DIR", "LOG_PATH", "log.dir", "logging.file", "/logs", "logs/"}); len(evidence) > 0 {
		item := defaultStorageRequirement(c, "logs")
		item.MountPath = firstDetectedPath(evidence, []string{"LOG_DIR", "LOG_PATH"}, item.MountPath)
		item.Evidence = evidence
		item.Reason = "detected log path; use platform-managed hostPath with retention metadata"
		items = append(items, item)
	}
	if evidence := storageEvidence(signals, []string{"upload", "uploads", "runtime", "files", "conversations"}); len(evidence) > 0 {
		item := defaultStorageRequirement(c, "runtime")
		item.MountPath = firstDetectedPath(evidence, []string{"UPLOAD_DIR", "UPLOAD_PATH", "RUNTIME_DIR", "FILES_DIR"}, item.MountPath)
		item.Evidence = evidence
		item.Reason = "detected runtime/upload file path; use platform-managed hostPath"
		items = append(items, item)
	}
	if evidence := storageEvidence(signals, []string{"CACHE_DIR", "cache.dir", "/cache", "tmp/cache", "temp"}); len(evidence) > 0 {
		item := defaultStorageRequirement(c, "cache")
		item.MountPath = firstDetectedPath(evidence, []string{"CACHE_DIR", "TMP_DIR", "TEMP_DIR"}, item.MountPath)
		item.Evidence = evidence
		item.Reason = "detected cache/temp path; use bounded emptyDir"
		items = append(items, item)
	}
	return normalizeStorageRequirements(c, items)
}

type repoSignal struct {
	Path string
	Text string
}

func collectRepoSignals() []repoSignal {
	signals := []repoSignal{}
	seen := map[string]bool{}
	maxFiles := 200
	_ = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || len(signals) >= maxFiles {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if shouldSkipScanDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		if !shouldScanDependencyFile(path) || seen[path] {
			return nil
		}
		seen[path] = true
		if body, ok := readSmallTextFile(path); ok {
			signals = append(signals, repoSignal{Path: filepath.ToSlash(path), Text: string(body)})
		}
		return nil
	})
	return signals
}

func shouldSkipScanDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "dist", "build", "target", ".next", ".venv", "venv", "__pycache__":
		return true
	default:
		return false
	}
}

func shouldScanDependencyFile(path string) bool {
	slashPath := filepath.ToSlash(path)
	if strings.HasPrefix(slashPath, "deploy/k8s/") {
		return false
	}
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "opspilot.service.yaml", "opspilot.namespaces.yaml", "opspilot.release-service.txt", ".gitlab-ci.yml":
		return false
	}
	switch base {
	case "go.mod", "package.json", "requirements.txt", "pyproject.toml", "pom.xml",
		".env", ".env.example", "application.yml", "application.yaml",
		"bootstrap.yml", "bootstrap.yaml", "config.yml", "config.yaml",
		"application.properties":
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".js", ".ts", ".py", ".java", ".yml", ".yaml", ".properties", ".toml":
		return true
	default:
		return false
	}
}

func readSmallTextFile(path string) ([]byte, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > 256*1024 {
		return nil, false
	}
	body, err := os.ReadFile(path)
	if err != nil || strings.ContainsRune(string(body), '\x00') {
		return nil, false
	}
	return body, true
}

func middlewareEvidence(signals []repoSignal, tokens []string) []string {
	evidence := []string{}
	for _, signal := range signals {
		lower := strings.ToLower(signal.Text)
		for _, token := range tokens {
			if token == "" || !strings.Contains(lower, strings.ToLower(token)) {
				continue
			}
			evidence = append(evidence, fmt.Sprintf("%s contains %s", signal.Path, token))
			if len(evidence) >= 3 {
				return evidence
			}
		}
	}
	return evidence
}

func storageEvidence(signals []repoSignal, tokens []string) []string {
	evidence := []string{}
	for _, signal := range signals {
		for _, line := range strings.Split(signal.Text, "\n") {
			lower := strings.ToLower(line)
			for _, token := range tokens {
				if token == "" || !strings.Contains(lower, strings.ToLower(token)) {
					continue
				}
				snippet := strings.Join(strings.Fields(line), " ")
				if len(snippet) > 120 {
					snippet = snippet[:120]
				}
				if snippet == "" {
					snippet = token
				}
				evidence = append(evidence, fmt.Sprintf("%s contains %s: %s", signal.Path, token, snippet))
				if len(evidence) >= 3 {
					return evidence
				}
			}
		}
	}
	return evidence
}

func normalizeMiddlewareRequirements(c onboardServiceConfig, items []onboardMiddlewareConfig) []onboardMiddlewareConfig {
	normalized := []onboardMiddlewareConfig{}
	seen := map[string]bool{}
	for _, item := range items {
		kind := firstNonEmpty(item.Kind, item.Name)
		entry, ok := middlewareCatalogByKind(kind)
		if !ok {
			entry = middlewareCatalogEntry{
				Kind:       sanitizeDNSLabel(kind),
				Display:    firstNonEmpty(item.Display, kind),
				Mode:       firstNonEmpty(item.Mode, "shared"),
				Allocation: firstNonEmpty(item.Allocation, "logical-resource"),
				Env:        item.Env,
			}
		}
		defaults := defaultMiddlewareRequirement(c, entry)
		defaults.Name = firstNonEmpty(item.Name, defaults.Name)
		defaults.Kind = firstNonEmpty(item.Kind, defaults.Kind)
		defaults.Display = firstNonEmpty(item.Display, defaults.Display)
		defaults.Mode = firstNonEmpty(item.Mode, defaults.Mode)
		defaults.Allocation = firstNonEmpty(item.Allocation, defaults.Allocation)
		defaults.Provision = firstNonEmpty(item.Provision, defaults.Provision)
		defaults.Resource = firstNonEmpty(item.Resource, defaults.Resource)
		defaults.Secret = firstNonEmpty(item.Secret, defaults.Secret)
		if len(item.Env) > 0 {
			defaults.Env = item.Env
		}
		defaults.Reason = item.Reason
		defaults.Evidence = item.Evidence
		key := defaults.Name
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		normalized = append(normalized, defaults)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return middlewareCatalogRank(normalized[i].Kind) < middlewareCatalogRank(normalized[j].Kind)
	})
	return normalized
}

func normalizeStorageRequirements(c onboardServiceConfig, items []onboardStorageConfig) []onboardStorageConfig {
	normalized := []onboardStorageConfig{}
	seen := map[string]bool{}
	for _, item := range items {
		name := sanitizeDNSLabel(firstNonEmpty(item.Name, item.Purpose))
		if name == "" {
			continue
		}
		purpose := sanitizeDNSLabel(firstNonEmpty(item.Purpose, name))
		mode := canonicalStorageMode(firstNonEmpty(item.Mode, defaultStorageMode(purpose)))
		normalizedItem := onboardStorageConfig{
			Name:          name,
			Purpose:       purpose,
			Mode:          mode,
			MountPath:     firstNonEmpty(item.MountPath, defaultStorageMountPath(purpose)),
			HostPath:      item.HostPath,
			SizeHint:      item.SizeHint,
			SizeLimit:     item.SizeLimit,
			RetentionDays: item.RetentionDays,
			ReadOnly:      item.ReadOnly,
			Reason:        item.Reason,
			Evidence:      item.Evidence,
		}
		switch mode {
		case "emptyDir":
			normalizedItem.HostPath = ""
			normalizedItem.SizeHint = ""
			if normalizedItem.SizeLimit == "" {
				normalizedItem.SizeLimit = defaultStorageSizeLimit(purpose)
			}
		default:
			normalizedItem.Mode = "hostPath"
			if !isPlatformHostPath(normalizedItem.HostPath) {
				normalizedItem.HostPath = platformHostPath(c, name)
			}
			if normalizedItem.SizeHint == "" {
				normalizedItem.SizeHint = defaultStorageSizeHint(purpose)
			}
			if purpose == "logs" && normalizedItem.RetentionDays == 0 {
				normalizedItem.RetentionDays = 7
			}
		}
		if normalizedItem.MountPath == "" || seen[normalizedItem.Name] {
			continue
		}
		seen[normalizedItem.Name] = true
		normalized = append(normalized, normalizedItem)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return storageRank(normalized[i].Purpose, normalized[i].Name) < storageRank(normalized[j].Purpose, normalized[j].Name)
	})
	return normalized
}

func defaultStorageRequirement(c onboardServiceConfig, purpose string) onboardStorageConfig {
	purpose = sanitizeDNSLabel(purpose)
	if purpose == "" {
		purpose = "data"
	}
	mode := defaultStorageMode(purpose)
	item := onboardStorageConfig{
		Name:      purpose,
		Purpose:   purpose,
		Mode:      mode,
		MountPath: defaultStorageMountPath(purpose),
	}
	if mode == "emptyDir" {
		item.SizeLimit = defaultStorageSizeLimit(purpose)
	} else {
		item.HostPath = platformHostPath(c, purpose)
		item.SizeHint = defaultStorageSizeHint(purpose)
		if purpose == "logs" {
			item.RetentionDays = 7
		}
	}
	return item
}

func defaultStorageMode(purpose string) string {
	switch sanitizeDNSLabel(purpose) {
	case "cache", "tmp", "temp":
		return "emptyDir"
	default:
		return "hostPath"
	}
}

func canonicalStorageMode(mode string) string {
	switch strings.ToLower(strings.ReplaceAll(mode, "-", "")) {
	case "emptydir":
		return "emptyDir"
	default:
		return "hostPath"
	}
}

func defaultStorageMountPath(purpose string) string {
	switch sanitizeDNSLabel(purpose) {
	case "logs":
		return "/app/logs"
	case "runtime", "uploads", "upload", "files", "data":
		return "/app/runtime"
	case "cache", "tmp", "temp":
		return "/tmp/cache"
	default:
		return "/app/" + sanitizeDNSLabel(purpose)
	}
}

func defaultStorageSizeHint(purpose string) string {
	switch sanitizeDNSLabel(purpose) {
	case "runtime", "uploads", "upload", "files", "data":
		return "20Gi"
	default:
		return "10Gi"
	}
}

func defaultStorageSizeLimit(purpose string) string {
	switch sanitizeDNSLabel(purpose) {
	case "cache", "tmp", "temp":
		return "1Gi"
	default:
		return "1Gi"
	}
}

func storageRank(purpose, name string) int {
	switch sanitizeDNSLabel(firstNonEmpty(purpose, name)) {
	case "logs":
		return 0
	case "runtime", "uploads", "upload", "files", "data":
		return 1
	case "cache", "tmp", "temp":
		return 2
	default:
		return 10
	}
}

func platformHostPath(c onboardServiceConfig, name string) string {
	namespace := sanitizeDNSLabel(firstNonEmpty(c.Namespace, defaultNamespace(c.Group, c.Project)))
	service := sanitizeDNSLabel(firstNonEmpty(c.Name, c.Container, "service"))
	volume := sanitizeDNSLabel(firstNonEmpty(name, "data"))
	return strings.TrimRight(defaultHostPathRoot, "/") + "/" + namespace + "/" + service + "/" + volume
}

func platformHostPathRoot(c onboardServiceConfig) string {
	namespace := sanitizeDNSLabel(firstNonEmpty(c.Namespace, defaultNamespace(c.Group, c.Project)))
	service := sanitizeDNSLabel(firstNonEmpty(c.Name, c.Container, "service"))
	return strings.TrimRight(defaultHostPathRoot, "/") + "/" + namespace + "/" + service
}

func isPlatformHostPath(value string) bool {
	value = strings.TrimSpace(value)
	root := strings.TrimRight(defaultHostPathRoot, "/") + "/"
	return strings.HasPrefix(value, root)
}

func firstDetectedPath(evidence []string, keys []string, fallback string) string {
	for _, item := range evidence {
		for _, key := range keys {
			if path := extractPathAfterKey(item, key); path != "" {
				return path
			}
		}
	}
	return fallback
}

func extractPathAfterKey(text, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	lower := strings.ToLower(text)
	markers := []string{
		strings.ToLower(key) + "=",
		strings.ToLower(key) + ":",
		strings.ToLower(key) + " =",
		strings.ToLower(key) + " :",
	}
	for _, marker := range markers {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		raw := text[idx+len(marker):]
		raw = strings.TrimLeft(raw, " \t\"'")
		if strings.HasPrefix(raw, "=") || strings.HasPrefix(raw, ":") {
			raw = strings.TrimLeft(raw[1:], " \t\"'")
		}
		end := len(raw)
		for i, r := range raw {
			if r == ' ' || r == '\t' || r == ',' || r == ';' || r == '"' || r == '\'' || r == '#' || r == '\r' {
				end = i
				break
			}
		}
		candidate := strings.TrimRight(strings.TrimSpace(raw[:end]), "/")
		if strings.HasPrefix(candidate, "/") {
			return candidate
		}
	}
	return ""
}

func defaultMiddlewareRequirement(c onboardServiceConfig, entry middlewareCatalogEntry) onboardMiddlewareConfig {
	name := sanitizeDNSLabel(entry.Kind)
	item := onboardMiddlewareConfig{
		Name:       name,
		Kind:       entry.Kind,
		Display:    entry.Display,
		Mode:       entry.Mode,
		Allocation: entry.Allocation,
		Resource:   middlewareResourceName(c, entry.Kind),
		Secret:     sanitizeDNSLabel(c.Name + "-" + entry.Kind + "-conn"),
		Env:        append([]string{}, entry.Env...),
	}
	if middlewareKindAutoProvisioned(entry.Kind) {
		item.Provision = "auto"
	} else {
		item.Provision = "external"
	}
	return item
}

func middlewareCatalogByKind(kind string) (middlewareCatalogEntry, bool) {
	kind = sanitizeDNSLabel(kind)
	for _, entry := range middlewareCatalog {
		if entry.Kind == kind {
			return entry, true
		}
	}
	return middlewareCatalogEntry{}, false
}

func middlewareCatalogRank(kind string) int {
	for i, entry := range middlewareCatalog {
		if entry.Kind == kind {
			return i
		}
	}
	return len(middlewareCatalog) + 1
}

func middlewareResourceName(c onboardServiceConfig, kind string) string {
	parts := []string{
		firstNonEmpty(c.Group, defaultGroup),
		firstNonEmpty(c.Project, projectNameFromService(c.Name)),
		c.Name,
		kind,
	}
	return strings.ReplaceAll(sanitizeDNSLabel(strings.Join(parts, "-")), "-", "_")
}

func splitCSV(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
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
	case "frontend":
		return fmt.Sprintf(`FROM m.daocloud.io/docker.io/library/node:20-alpine AS build

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY

WORKDIR /app
COPY package*.json ./
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi
COPY . .
RUN npm run build

FROM m.daocloud.io/docker.io/library/nginx:1.27-alpine
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE %d
`, c.Port)
	case "java":
		return fmt.Sprintf(`FROM m.daocloud.io/docker.io/library/maven:3.9.9-eclipse-temurin-21-alpine AS build

ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG NO_PROXY

WORKDIR /app
COPY pom.xml .
RUN mvn -B -q -DskipTests dependency:go-offline || true
COPY src ./src
RUN mvn -B -DskipTests package

FROM m.daocloud.io/docker.io/library/eclipse-temurin:21-jre-alpine
WORKDIR /app
COPY --from=build /app/target/*.jar /app/app.jar

EXPOSE %d

ENTRYPOINT ["java", "-jar", "/app/app.jar"]
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
  - project: %s
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
`, defaultCITemplateProject, c.Language, c.Name, argoAppName(c), c.BuildEntry, c.BuildOutput, c.DockerPath, gitOpsAppPath(c), argoAppName(c), c.Container, c.Namespace)
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

func limitRangeTemplate(c onboardServiceConfig) string {
	return fmt.Sprintf(`apiVersion: v1
kind: LimitRange
metadata:
  name: %s-defaults
  namespace: %s
spec:
  limits:
    - type: Container
      defaultRequest:
        cpu: %s
        memory: %s
      default:
        cpu: %s
        memory: %s
`, c.Namespace, c.Namespace, c.Resources.RequestCPU, c.Resources.RequestMemory, c.Resources.LimitCPU, c.Resources.LimitMemory)
}

func resourceQuotaTemplate(c onboardServiceConfig) string {
	return fmt.Sprintf(`apiVersion: v1
kind: ResourceQuota
metadata:
  name: %s-quota
  namespace: %s
spec:
  hard:
    requests.cpu: %s
    requests.memory: %s
    limits.cpu: %s
    limits.memory: %s
    pods: "%s"
`, c.Namespace, c.Namespace, c.NamespaceGuard.RequestsCPU, c.NamespaceGuard.RequestsMemory, c.NamespaceGuard.LimitsCPU, c.NamespaceGuard.LimitsMemory, c.NamespaceGuard.Pods)
}

func serviceAccountTemplate(c onboardServiceConfig) string {
	return fmt.Sprintf(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: %s
  namespace: %s
imagePullSecrets:
  - name: gitlab-registry-pull
`, serviceAccountName(c), c.Namespace)
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
%s
    spec:
      serviceAccountName: %s
      imagePullSecrets:
        - name: gitlab-registry-pull
      containers:
        - name: %s
          image: placeholder
          imagePullPolicy: IfNotPresent
          ports:
            - name: http
              containerPort: %d
          resources:
            requests:
              cpu: %s
              memory: %s
            limits:
              cpu: %s
              memory: %s
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
%s%s%s`, c.Name, c.Namespace, c.Name, c.Replicas, c.Name, c.Name, storagePodAnnotationsTemplate(c), serviceAccountName(c), c.Container, c.Port, c.Resources.RequestCPU, c.Resources.RequestMemory, c.Resources.LimitCPU, c.Resources.LimitMemory, c.HealthPath, c.HealthPath, middlewareEnvFromTemplate(c.Middleware), storageVolumeMountsTemplate(c.Storage), storageVolumesTemplate(c.Storage))
}

func serviceAccountName(c onboardServiceConfig) string {
	return sanitizeDNSLabel(firstNonEmpty(c.Container, c.Name, "app"))
}

func storagePodAnnotationsTemplate(c onboardServiceConfig) string {
	if len(c.Storage) == 0 {
		return ""
	}
	softLimit := storageSoftLimitSummary(c.Storage)
	if softLimit == "" {
		softLimit = "none"
	}
	return fmt.Sprintf(`      annotations:
        opspilot.io/storage-managed: "true"
        opspilot.io/storage-hostpath-root: "%s"
        opspilot.io/storage-soft-limit: "%s"
`, platformHostPathRoot(c), softLimit)
}

func storageSoftLimitSummary(items []onboardStorageConfig) string {
	parts := []string{}
	for _, item := range items {
		limit := firstNonEmpty(item.SizeHint, item.SizeLimit)
		if limit == "" {
			continue
		}
		parts = append(parts, item.Name+"="+limit)
	}
	return strings.Join(parts, ",")
}

func middlewareEnvFromTemplate(items []onboardMiddlewareConfig) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("          envFrom:\n")
	seen := map[string]bool{}
	for _, item := range items {
		if item.Secret == "" || seen[item.Secret] {
			continue
		}
		seen[item.Secret] = true
		b.WriteString("            - secretRef:\n")
		b.WriteString(fmt.Sprintf("                name: %s\n", item.Secret))
	}
	return b.String()
}

func storageVolumeMountsTemplate(items []onboardStorageConfig) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("          volumeMounts:\n")
	for _, item := range items {
		b.WriteString(fmt.Sprintf("            - name: %s\n", item.Name))
		b.WriteString(fmt.Sprintf("              mountPath: %s\n", item.MountPath))
		if item.ReadOnly {
			b.WriteString("              readOnly: true\n")
		}
	}
	return b.String()
}

func storageVolumesTemplate(items []onboardStorageConfig) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("      volumes:\n")
	for _, item := range items {
		b.WriteString(fmt.Sprintf("        - name: %s\n", item.Name))
		switch item.Mode {
		case "emptyDir":
			b.WriteString("          emptyDir:\n")
			b.WriteString(fmt.Sprintf("            sizeLimit: %s\n", firstNonEmpty(item.SizeLimit, defaultStorageSizeLimit(item.Purpose))))
		default:
			b.WriteString("          hostPath:\n")
			b.WriteString(fmt.Sprintf("            path: %s\n", item.HostPath))
			b.WriteString("            type: DirectoryOrCreate\n")
		}
	}
	return b.String()
}

func middlewareTemplate(c onboardServiceConfig, item onboardMiddlewareConfig) string {
	switch sanitizeDNSLabel(item.Kind) {
	case "mysql":
		return mysqlMiddlewareTemplate(c, item)
	case "postgres":
		return postgresMiddlewareTemplate(c, item)
	case "redis":
		return redisMiddlewareTemplate(c, item)
	case "s3":
		return minioMiddlewareTemplate(c, item)
	default:
		return ""
	}
}

func mysqlMiddlewareTemplate(c onboardServiceConfig, item onboardMiddlewareConfig) string {
	name := middlewareWorkloadName(c, item)
	password := middlewareDefaultPassword(c)
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  MYSQL_DATABASE: %s
  MYSQL_USER: %s
  MYSQL_PASSWORD: %s
  MYSQL_ROOT_PASSWORD: %s
  DATABASE_URL: mysql://%s:%s@%s.%s.svc.cluster.local:3306/%s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    opspilot.io/middleware: mysql
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: %s
  template:
    metadata:
      labels:
        app.kubernetes.io/name: %s
        opspilot.io/middleware: mysql
    spec:
      containers:
        - name: mysql
          image: m.daocloud.io/docker.io/library/mysql:8.4
          imagePullPolicy: IfNotPresent
          envFrom:
            - secretRef:
                name: %s
          ports:
            - name: mysql
              containerPort: 3306
          resources:
            requests:
              cpu: 100m
              memory: 256Mi
            limits:
              cpu: 500m
              memory: 1Gi
          readinessProbe:
            tcpSocket:
              port: mysql
            initialDelaySeconds: 20
            periodSeconds: 10
          livenessProbe:
            tcpSocket:
              port: mysql
            initialDelaySeconds: 60
            periodSeconds: 20
---
apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    app.kubernetes.io/name: %s
  ports:
    - name: mysql
      port: 3306
      targetPort: mysql
`, item.Secret, c.Namespace, item.Resource, sanitizeDNSLabel(c.Name), password, password, sanitizeDNSLabel(c.Name), password, name, c.Namespace, item.Resource, name, c.Namespace, name, name, name, item.Secret, name, c.Namespace, name)
}

func postgresMiddlewareTemplate(c onboardServiceConfig, item onboardMiddlewareConfig) string {
	name := middlewareWorkloadName(c, item)
	password := middlewareDefaultPassword(c)
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  POSTGRES_DB: %s
  POSTGRES_USER: %s
  POSTGRES_PASSWORD: %s
  DATABASE_URL: postgres://%s:%s@%s.%s.svc.cluster.local:5432/%s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    opspilot.io/middleware: postgres
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: %s
  template:
    metadata:
      labels:
        app.kubernetes.io/name: %s
        opspilot.io/middleware: postgres
    spec:
      containers:
        - name: postgres
          image: m.daocloud.io/docker.io/library/postgres:16-alpine
          imagePullPolicy: IfNotPresent
          envFrom:
            - secretRef:
                name: %s
          ports:
            - name: postgres
              containerPort: 5432
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 512Mi
          readinessProbe:
            tcpSocket:
              port: postgres
            initialDelaySeconds: 10
            periodSeconds: 10
          livenessProbe:
            tcpSocket:
              port: postgres
            initialDelaySeconds: 30
            periodSeconds: 20
---
apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    app.kubernetes.io/name: %s
  ports:
    - name: postgres
      port: 5432
      targetPort: postgres
`, item.Secret, c.Namespace, item.Resource, sanitizeDNSLabel(c.Name), password, sanitizeDNSLabel(c.Name), password, name, c.Namespace, item.Resource, name, c.Namespace, name, name, name, item.Secret, name, c.Namespace, name)
}

func redisMiddlewareTemplate(c onboardServiceConfig, item onboardMiddlewareConfig) string {
	name := middlewareWorkloadName(c, item)
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  REDIS_URL: redis://%s.%s.svc.cluster.local:6379/0
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    opspilot.io/middleware: redis
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: %s
  template:
    metadata:
      labels:
        app.kubernetes.io/name: %s
        opspilot.io/middleware: redis
    spec:
      containers:
        - name: redis
          image: m.daocloud.io/docker.io/library/redis:7-alpine
          imagePullPolicy: IfNotPresent
          ports:
            - name: redis
              containerPort: 6379
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 250m
              memory: 256Mi
          readinessProbe:
            tcpSocket:
              port: redis
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            tcpSocket:
              port: redis
            initialDelaySeconds: 15
            periodSeconds: 20
---
apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    app.kubernetes.io/name: %s
  ports:
    - name: redis
      port: 6379
      targetPort: redis
`, item.Secret, c.Namespace, name, c.Namespace, name, c.Namespace, name, name, name, name, c.Namespace, name)
}

func minioMiddlewareTemplate(c onboardServiceConfig, item onboardMiddlewareConfig) string {
	name := middlewareWorkloadName(c, item)
	password := middlewareDefaultPassword(c)
	bucket := sanitizeDNSLabel(c.Name)
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  MINIO_ROOT_USER: %s
  MINIO_ROOT_PASSWORD: %s
  S3_ENDPOINT: http://%s.%s.svc.cluster.local:9000
  S3_BUCKET: %s
  S3_ACCESS_KEY: %s
  S3_SECRET_KEY: %s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    opspilot.io/middleware: s3
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: %s
  template:
    metadata:
      labels:
        app.kubernetes.io/name: %s
        opspilot.io/middleware: s3
    spec:
      containers:
        - name: minio
          image: m.daocloud.io/docker.io/minio/minio:RELEASE.2025-04-22T22-12-26Z
          imagePullPolicy: IfNotPresent
          args: ["server", "/data"]
          envFrom:
            - secretRef:
                name: %s
          ports:
            - name: s3
              containerPort: 9000
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 512Mi
          readinessProbe:
            httpGet:
              path: /minio/health/ready
              port: s3
            initialDelaySeconds: 10
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /minio/health/live
              port: s3
            initialDelaySeconds: 20
            periodSeconds: 20
---
apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    app.kubernetes.io/name: %s
  ports:
    - name: s3
      port: 9000
      targetPort: s3
`, item.Secret, c.Namespace, sanitizeDNSLabel(c.Name), password, name, c.Namespace, bucket, sanitizeDNSLabel(c.Name), password, name, c.Namespace, name, name, name, item.Secret, name, c.Namespace, name)
}

func middlewareAutoProvisioned(item onboardMiddlewareConfig) bool {
	return strings.EqualFold(firstNonEmpty(item.Provision, "external"), "auto") && middlewareKindAutoProvisioned(item.Kind)
}

func middlewareKindAutoProvisioned(kind string) bool {
	switch sanitizeDNSLabel(kind) {
	case "mysql", "postgres", "redis", "s3":
		return true
	default:
		return false
	}
}

func middlewareWorkloadName(c onboardServiceConfig, item onboardMiddlewareConfig) string {
	return sanitizeDNSLabel(c.Name + "-" + item.Name)
}

func middlewareDefaultPassword(c onboardServiceConfig) string {
	sum := sha1.Sum([]byte(c.GitLabProject + "/" + c.Name))
	return "opspilot-" + hex.EncodeToString(sum[:])[:12]
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

func qualityTemplate(c onboardServiceConfig) string {
	return fmt.Sprintf(`quality:
  enabled: true
  optional: true
  baseURL: http://%s.%s.svc.cluster.local:%d
  smoke:
    timeoutSeconds: 3
    latencyP95Ms: 1000
    endpoints:
      - name: health
        method: GET
        path: %s
        expectStatus: 200
`, c.Name, c.Namespace, c.Port, firstNonEmpty(c.HealthPath, "/health"))
}

func kustomizationTemplate(c onboardServiceConfig) string {
	var b strings.Builder
	b.WriteString(`resources:
  - namespace.yaml
  - limitrange.yaml
  - resourcequota.yaml
  - serviceaccount.yaml
  - deployment.yaml
  - service.yaml
`)
	for _, item := range c.Middleware {
		if middlewareAutoProvisioned(item) {
			b.WriteString(fmt.Sprintf("  - middleware-%s.yaml\n", item.Name))
		}
	}
	return b.String()
}

func releaseMapping(c onboardServiceConfig) string {
	image := imageName(c)
	gitops := gitOpsAppPath(c) + "/deployment.yaml"
	return fmt.Sprintf("%s=namespace:%s,deployment:%s,container:%s,source:%s,image:%s,gitlab:%s,gitops:%s,argocd:%s",
		c.Name, c.Namespace, c.Name, c.Container, c.PromSource, image, c.GitLabProject, gitops, argoAppName(c))
}

func gitOpsPlan(c onboardServiceConfig) onboardGitOpsPlan {
	path := gitOpsAppPath(c)
	return onboardGitOpsPlan{
		Cluster:         "node200-test",
		Path:            path,
		DeploymentPath:  path + "/deployment.yaml",
		ApplicationName: argoAppName(c),
		Namespace:       c.Namespace,
		Image:           imageName(c),
		StandardFlow: []string{
			"git push application repository",
			"GitLab Runner preflight and code-precheck",
			"BuildKit rootless image build",
			"push image to GitLab Registry",
			"update GitOps repository",
			"Argo CD reconciles node200",
			"OpsPilot inspects release and runtime evidence",
		},
	}
}

func imageName(c onboardServiceConfig) string {
	return "192.168.48.206:5050/" + c.GitLabProject + "/" + c.Name
}

func middlewareCredentialPlans(c onboardServiceConfig) []string {
	out := []string{}
	for _, item := range c.Middleware {
		if item.Kind == "" {
			continue
		}
		secret := firstNonEmpty(item.Secret, sanitizeDNSLabel(c.Name+"-"+item.Kind+"-credentials"))
		out = append(out, fmt.Sprintf("%s: secret=%s mode=%s allocation=%s keys=%s",
			item.Kind, secret, firstNonEmpty(item.Mode, "shared"), firstNonEmpty(item.Allocation, "service-scoped"), strings.Join(item.Env, "|")))
	}
	return out
}
