package configloader

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const Version = "v1"

type Metadata struct {
	Name string `json:"name" yaml:"name"`
}

type Config struct {
	Version     string       `json:"version"`
	Source      string       `json:"source"`
	Directory   string       `json:"directory,omitempty"`
	Commit      string       `json:"commit,omitempty"`
	LoadedAt    string       `json:"loaded_at"`
	Valid       bool         `json:"valid"`
	Files       []string     `json:"files,omitempty"`
	Warnings    []string     `json:"warnings,omitempty"`
	Errors      []string     `json:"errors,omitempty"`
	Services    []Service    `json:"services,omitempty"`
	Datasources []Datasource `json:"datasources,omitempty"`
	Credentials []Credential `json:"credentials,omitempty"`
	Clusters    []Cluster    `json:"clusters,omitempty"`
	Topology    []Region     `json:"topology,omitempty"`
	Rules       []Rule       `json:"correlation_rules,omitempty"`
}

type Summary struct {
	Version     string         `json:"version"`
	Source      string         `json:"source"`
	Directory   string         `json:"directory,omitempty"`
	Commit      string         `json:"commit,omitempty"`
	LoadedAt    string         `json:"loaded_at"`
	Valid       bool           `json:"valid"`
	Files       []string       `json:"files,omitempty"`
	Warnings    []string       `json:"warnings,omitempty"`
	Errors      []string       `json:"errors,omitempty"`
	Counts      map[string]int `json:"counts"`
	Credentials []Credential   `json:"credentials,omitempty"`
}

type Service struct {
	Name          string             `json:"name" yaml:"name"`
	Environment   string             `json:"environment,omitempty" yaml:"environment"`
	Group         string             `json:"group,omitempty" yaml:"group"`
	Project       string             `json:"project,omitempty" yaml:"project"`
	Owner         string             `json:"owner,omitempty" yaml:"owner"`
	Repo          string             `json:"repo,omitempty" yaml:"repo"`
	Domains       []string           `json:"domains,omitempty" yaml:"domains"`
	Runtime       RuntimeSpec        `json:"runtime,omitempty" yaml:"runtime"`
	Logs          ServiceLogSpec     `json:"logs,omitempty" yaml:"logs"`
	Gateway       GatewaySpec        `json:"gateway,omitempty" yaml:"gateway"`
	Release       ReleaseSpec        `json:"release,omitempty" yaml:"release"`
	ConfigSources []string           `json:"config_sources,omitempty" yaml:"config_sources"`
	Middleware    []string           `json:"middleware,omitempty" yaml:"middleware"`
	Dependencies  []string           `json:"dependencies,omitempty" yaml:"dependencies"`
	Storage       []string           `json:"storage,omitempty" yaml:"storage"`
	Correlation   ServiceCorrelation `json:"correlation,omitempty" yaml:"correlation"`
	Source        string             `json:"source,omitempty" yaml:"-"`
}

type RuntimeSpec struct {
	Cluster    string `json:"cluster,omitempty" yaml:"cluster"`
	Namespace  string `json:"namespace,omitempty" yaml:"namespace"`
	Deployment string `json:"deployment,omitempty" yaml:"deployment"`
	Container  string `json:"container,omitempty" yaml:"container"`
	Image      string `json:"image,omitempty" yaml:"image"`
	Port       string `json:"port,omitempty" yaml:"port"`
}

type ServiceLogSpec struct {
	AppIndexes    []string `json:"app_indexes,omitempty" yaml:"app_indexes"`
	MessageFields []string `json:"message_fields,omitempty" yaml:"message_fields"`
}

type GatewaySpec struct {
	Datasource  string `json:"datasource,omitempty" yaml:"datasource"`
	APISIXIndex string `json:"apisix_index,omitempty" yaml:"apisix_index"`
}

type ReleaseSpec struct {
	GitLab string `json:"gitlab_project,omitempty" yaml:"gitlab_project"`
	GitOps string `json:"gitops_path,omitempty" yaml:"gitops_path"`
	ArgoCD string `json:"argocd_app,omitempty" yaml:"argocd_app"`
}

type ServiceCorrelation struct {
	RequireURI           *bool    `json:"require_uri,omitempty" yaml:"require_uri"`
	PathPrefixes         []string `json:"path_prefixes,omitempty" yaml:"path_prefixes"`
	DefaultWindowSeconds int      `json:"default_window_seconds,omitempty" yaml:"default_window_seconds"`
}

type Datasource struct {
	Name          string             `json:"name" yaml:"name"`
	Kind          string             `json:"kind" yaml:"kind"`
	Environment   string             `json:"environment,omitempty" yaml:"environment"`
	Region        string             `json:"region,omitempty" yaml:"region"`
	URL           string             `json:"url,omitempty" yaml:"url"`
	CredentialRef string             `json:"credential_ref,omitempty" yaml:"credential_ref"`
	Indexes       DatasourceIndexes  `json:"indexes,omitempty" yaml:"indexes"`
	Fields        map[string]string  `json:"fields,omitempty" yaml:"fields"`
	Options       map[string]string  `json:"options,omitempty" yaml:"options"`
	Source        string             `json:"source,omitempty" yaml:"-"`
	Credential    *CredentialRuntime `json:"-" yaml:"-"`
}

type CredentialRuntime struct {
	Username string
	Password string
}

type DatasourceIndexes struct {
	APISIX     string   `json:"apisix,omitempty" yaml:"apisix"`
	AppDefault []string `json:"app_default,omitempty" yaml:"app_default"`
	App        []string `json:"app,omitempty" yaml:"app"`
}

type Credential struct {
	Name        string   `json:"name" yaml:"name"`
	Type        string   `json:"type,omitempty" yaml:"type"`
	Class       string   `json:"class,omitempty" yaml:"class"`
	Environment string   `json:"environment,omitempty" yaml:"environment"`
	Scope       string   `json:"scope,omitempty" yaml:"scope"`
	Storage     string   `json:"storage,omitempty" yaml:"storage"`
	Namespace   string   `json:"namespace,omitempty" yaml:"namespace"`
	Username    string   `json:"username,omitempty" yaml:"username"`
	Password    string   `json:"-" yaml:"password"`
	PasswordSet bool     `json:"password_set,omitempty" yaml:"-"`
	UsedBy      []string `json:"used_by,omitempty" yaml:"used_by"`
	Permissions []string `json:"permissions,omitempty" yaml:"permissions"`
	Owner       string   `json:"owner,omitempty" yaml:"owner"`
	Rotation    string   `json:"rotation,omitempty" yaml:"rotation"`
	Note        string   `json:"note,omitempty" yaml:"note"`
	Source      string   `json:"source,omitempty" yaml:"-"`
}

type Cluster struct {
	Name           string `json:"name" yaml:"name"`
	Environment    string `json:"environment,omitempty" yaml:"environment"`
	KubernetesMode string `json:"kubernetes_mode,omitempty" yaml:"kubernetes_mode"`
	KubernetesRef  string `json:"kubernetes_ref,omitempty" yaml:"kubernetes_ref"`
	KubeconfigPath string `json:"kubeconfig_path,omitempty" yaml:"kubeconfig_path"`
	KubeContext    string `json:"kube_context,omitempty" yaml:"kube_context"`
	Prometheus     string `json:"prometheus,omitempty" yaml:"prometheus"`
	Logs           string `json:"logs,omitempty" yaml:"logs"`
	GitOpsProject  string `json:"gitops_project,omitempty" yaml:"gitops_project"`
	GitOpsPath     string `json:"gitops_path,omitempty" yaml:"gitops_path"`
	ArgoNamespace  string `json:"argocd_namespace,omitempty" yaml:"argocd_namespace"`
	Registry       string `json:"registry,omitempty" yaml:"registry"`
	Source         string `json:"source,omitempty" yaml:"-"`
}

type Region struct {
	Name      string   `json:"name" yaml:"name"`
	Zone      string   `json:"zone,omitempty" yaml:"zone"`
	Neighbors []string `json:"neighbors,omitempty" yaml:"neighbors"`
	Source    string   `json:"source,omitempty" yaml:"-"`
}

type Rule struct {
	Name                 string   `json:"name" yaml:"name"`
	Host                 string   `json:"host,omitempty" yaml:"host"`
	PathPrefixes         []string `json:"path_prefixes,omitempty" yaml:"path_prefixes"`
	Service              string   `json:"service,omitempty" yaml:"service"`
	ServiceIndex         string   `json:"service_index,omitempty" yaml:"service_index"`
	ServiceURIField      string   `json:"service_uri_field,omitempty" yaml:"service_uri_field"`
	ServiceEventField    string   `json:"service_event_field,omitempty" yaml:"service_event_field"`
	ServiceEventTemplate string   `json:"service_event_template,omitempty" yaml:"service_event_template"`
	ServiceFallbackQuery string   `json:"service_fallback_query,omitempty" yaml:"service_fallback_query"`
	RequireURI           *bool    `json:"require_uri,omitempty" yaml:"require_uri"`
	Source               string   `json:"source,omitempty" yaml:"-"`
}

type documentHeader struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
}

type serviceDocument struct {
	APIVersion string      `yaml:"apiVersion"`
	Kind       string      `yaml:"kind"`
	Metadata   Metadata    `yaml:"metadata"`
	Spec       ServiceSpec `yaml:"spec"`
}

type ServiceSpec Service

type datasourceDocument struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	Metadata   Metadata       `yaml:"metadata"`
	Spec       DatasourceSpec `yaml:"spec"`
}

type DatasourceSpec Datasource

type credentialDocument struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	Metadata   Metadata       `yaml:"metadata"`
	Spec       CredentialSpec `yaml:"spec"`
}

type CredentialSpec Credential

type clusterDocument struct {
	APIVersion string      `yaml:"apiVersion"`
	Kind       string      `yaml:"kind"`
	Metadata   Metadata    `yaml:"metadata"`
	Spec       ClusterSpec `yaml:"spec"`
}

type ClusterSpec Cluster

type bulkDocument struct {
	Version     string       `yaml:"version"`
	Services    []Service    `yaml:"services"`
	Datasources []Datasource `yaml:"datasources"`
	Credentials []Credential `yaml:"credentials"`
	Clusters    []Cluster    `yaml:"clusters"`
	Topology    []Region     `yaml:"topology"`
	Rules       []Rule       `yaml:"correlation_rules"`
}

func Load(dir string) Config {
	cfg := Config{
		Version:  Version,
		Source:   "env",
		LoadedAt: time.Now().UTC().Format(time.RFC3339),
		Valid:    true,
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		cfg.Warnings = append(cfg.Warnings, "OPSPILOT_CONFIG_DIR is not set; using legacy environment configuration")
		return cfg
	}
	cfg.Source = "file"
	cfg.Directory = dir
	files, err := yamlFiles(dir)
	if err != nil {
		cfg.Valid = false
		cfg.Errors = append(cfg.Errors, err.Error())
		return cfg
	}
	cfg.Files = relativeFiles(dir, files)
	cfg.Commit = readFirstExisting(
		filepath.Join(dir, ".git", "refs", "heads", "main"),
		filepath.Join(dir, "REVISION"),
		filepath.Join(dir, "revision"),
	)
	for _, file := range files {
		loadFile(file, &cfg)
	}
	dedupe(&cfg)
	attachCredentials(&cfg)
	validate(&cfg)
	if len(cfg.Errors) > 0 {
		cfg.Valid = false
	}
	return cfg
}

func (c Config) Summary() Summary {
	return Summary{
		Version:   c.Version,
		Source:    c.Source,
		Directory: c.Directory,
		Commit:    c.Commit,
		LoadedAt:  c.LoadedAt,
		Valid:     c.Valid,
		Files:     c.Files,
		Warnings:  c.Warnings,
		Errors:    c.Errors,
		Counts: map[string]int{
			"services":          len(c.Services),
			"datasources":       len(c.Datasources),
			"credentials":       len(c.Credentials),
			"clusters":          len(c.Clusters),
			"topology_regions":  len(c.Topology),
			"correlation_rules": len(c.Rules),
		},
		Credentials: redactCredentials(c.Credentials),
	}
}

func yamlFiles(dir string) ([]string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("config dir not available: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("config path is not a directory: %s", dir)
	}
	files := []string{}
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".git") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".yaml" || ext == ".yml" {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func loadFile(path string, cfg *Config) {
	f, err := os.Open(path)
	if err != nil {
		cfg.Errors = append(cfg.Errors, fmt.Sprintf("%s: %v", path, err))
		return
	}
	defer f.Close()
	decoder := yaml.NewDecoder(f)
	for {
		var node yaml.Node
		err := decoder.Decode(&node)
		if err == io.EOF {
			return
		}
		if err != nil {
			cfg.Errors = append(cfg.Errors, fmt.Sprintf("%s: %v", path, err))
			return
		}
		if len(node.Content) == 0 {
			continue
		}
		parseDocument(path, &node, cfg)
	}
}

func parseDocument(path string, node *yaml.Node, cfg *Config) {
	var header documentHeader
	if err := node.Decode(&header); err != nil {
		cfg.Errors = append(cfg.Errors, fmt.Sprintf("%s: %v", path, err))
		return
	}
	switch strings.ToLower(strings.TrimSpace(header.Kind)) {
	case "service":
		var doc serviceDocument
		if err := node.Decode(&doc); err != nil {
			cfg.Errors = append(cfg.Errors, fmt.Sprintf("%s: %v", path, err))
			return
		}
		item := Service(doc.Spec)
		item.Name = firstNonEmpty(item.Name, doc.Metadata.Name)
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Services = append(cfg.Services, item)
	case "datasource":
		var doc datasourceDocument
		if err := node.Decode(&doc); err != nil {
			cfg.Errors = append(cfg.Errors, fmt.Sprintf("%s: %v", path, err))
			return
		}
		item := Datasource(doc.Spec)
		item.Name = firstNonEmpty(item.Name, doc.Metadata.Name)
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Datasources = append(cfg.Datasources, item)
	case "credential":
		var doc credentialDocument
		if err := node.Decode(&doc); err != nil {
			cfg.Errors = append(cfg.Errors, fmt.Sprintf("%s: %v", path, err))
			return
		}
		item := Credential(doc.Spec)
		item.Name = firstNonEmpty(item.Name, doc.Metadata.Name)
		item.PasswordSet = item.Password != ""
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Credentials = append(cfg.Credentials, item)
	case "cluster":
		var doc clusterDocument
		if err := node.Decode(&doc); err != nil {
			cfg.Errors = append(cfg.Errors, fmt.Sprintf("%s: %v", path, err))
			return
		}
		item := Cluster(doc.Spec)
		item.Name = firstNonEmpty(item.Name, doc.Metadata.Name)
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Clusters = append(cfg.Clusters, item)
	case "":
		parseBulkDocument(path, node, cfg)
	default:
		cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("%s: unsupported kind %q", path, header.Kind))
	}
}

func parseBulkDocument(path string, node *yaml.Node, cfg *Config) {
	var doc bulkDocument
	if err := node.Decode(&doc); err != nil {
		cfg.Errors = append(cfg.Errors, fmt.Sprintf("%s: %v", path, err))
		return
	}
	for _, item := range doc.Services {
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Services = append(cfg.Services, item)
	}
	for _, item := range doc.Datasources {
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Datasources = append(cfg.Datasources, item)
	}
	for _, item := range doc.Credentials {
		item.PasswordSet = item.Password != ""
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Credentials = append(cfg.Credentials, item)
	}
	for _, item := range doc.Clusters {
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Clusters = append(cfg.Clusters, item)
	}
	for _, item := range doc.Topology {
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Topology = append(cfg.Topology, item)
	}
	for _, item := range doc.Rules {
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Rules = append(cfg.Rules, item)
	}
}

func validate(cfg *Config) {
	if cfg.Source == "file" && len(cfg.Files) == 0 {
		cfg.Warnings = append(cfg.Warnings, "config dir contains no YAML files")
	}
	for _, item := range cfg.Services {
		if item.Name == "" {
			cfg.Errors = append(cfg.Errors, "service entry missing name")
		}
		if len(item.Domains) == 0 && item.Runtime.Deployment == "" && len(item.Logs.AppIndexes) == 0 {
			cfg.Warnings = append(cfg.Warnings, "service "+item.Name+" has no domains, deployment, or app log indexes")
		}
	}
	for _, item := range cfg.Datasources {
		if item.Name == "" {
			cfg.Errors = append(cfg.Errors, "datasource entry missing name")
		}
		if item.Kind == "" {
			cfg.Errors = append(cfg.Errors, "datasource "+item.Name+" missing kind")
		}
		if item.URL == "" {
			cfg.Warnings = append(cfg.Warnings, "datasource "+item.Name+" has no URL")
		}
		if item.CredentialRef != "" && credentialByName(cfg.Credentials, item.CredentialRef) == nil {
			cfg.Warnings = append(cfg.Warnings, "datasource "+item.Name+" references missing credential "+item.CredentialRef)
		}
	}
	for _, item := range cfg.Credentials {
		if item.Name == "" {
			cfg.Errors = append(cfg.Errors, "credential entry missing name")
		}
	}
	for _, item := range cfg.Clusters {
		if item.Name == "" {
			cfg.Errors = append(cfg.Errors, "cluster entry missing name")
		}
	}
}

func attachCredentials(cfg *Config) {
	for idx := range cfg.Datasources {
		ref := cfg.Datasources[idx].CredentialRef
		if ref == "" {
			continue
		}
		cred := credentialByName(cfg.Credentials, ref)
		if cred == nil {
			continue
		}
		cfg.Datasources[idx].Credential = &CredentialRuntime{
			Username: cred.Username,
			Password: cred.Password,
		}
	}
}

func credentialByName(items []Credential, name string) *Credential {
	for idx := range items {
		if items[idx].Name == name {
			return &items[idx]
		}
	}
	return nil
}

func dedupe(cfg *Config) {
	cfg.Services = dedupeByName(cfg.Services, func(item Service) string { return item.Name })
	cfg.Datasources = dedupeByName(cfg.Datasources, func(item Datasource) string { return item.Name })
	cfg.Credentials = dedupeByName(cfg.Credentials, func(item Credential) string { return item.Name })
	cfg.Clusters = dedupeByName(cfg.Clusters, func(item Cluster) string { return item.Name })
	cfg.Topology = dedupeByName(cfg.Topology, func(item Region) string { return item.Name })
	cfg.Rules = dedupeByName(cfg.Rules, func(item Rule) string { return item.Name })
}

func dedupeByName[T any](items []T, name func(T) string) []T {
	order := []string{}
	byName := map[string]T{}
	unnamed := []T{}
	for _, item := range items {
		key := name(item)
		if key == "" {
			unnamed = append(unnamed, item)
			continue
		}
		if _, exists := byName[key]; !exists {
			order = append(order, key)
		}
		byName[key] = item
	}
	out := append([]T{}, unnamed...)
	for _, key := range order {
		out = append(out, byName[key])
	}
	return out
}

func redactCredentials(items []Credential) []Credential {
	out := make([]Credential, 0, len(items))
	for _, item := range items {
		item.PasswordSet = item.Password != ""
		item.Password = ""
		out = append(out, item)
	}
	return out
}

func relativeFiles(root string, files []string) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		rel, err := filepath.Rel(root, file)
		if err != nil {
			out = append(out, filepath.ToSlash(file))
			continue
		}
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}

func readFirstExisting(paths ...string) string {
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(body))
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
