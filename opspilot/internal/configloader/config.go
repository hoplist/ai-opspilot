package configloader

import (
	"fmt"
	"io"
	"net/netip"
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
	Version      string        `json:"version"`
	Source       string        `json:"source"`
	Directory    string        `json:"directory,omitempty"`
	Commit       string        `json:"commit,omitempty"`
	LoadedAt     string        `json:"loaded_at"`
	Valid        bool          `json:"valid"`
	Files        []string      `json:"files,omitempty"`
	Warnings     []string      `json:"warnings,omitempty"`
	Errors       []string      `json:"errors,omitempty"`
	Settings     Settings      `json:"settings,omitempty"`
	Services     []Service     `json:"services,omitempty"`
	Datasources  []Datasource  `json:"datasources,omitempty"`
	Credentials  []Credential  `json:"credentials,omitempty"`
	Clusters     []Cluster     `json:"clusters,omitempty"`
	Agents       []Agent       `json:"agents,omitempty"`
	NetworkZones []NetworkZone `json:"network_zones,omitempty"`
	AssetSources []AssetSource `json:"asset_sources,omitempty"`
	Assets       []Asset       `json:"assets,omitempty"`
	Flows        []Flow        `json:"flows,omitempty"`
	Inspections  []Inspection  `json:"inspections,omitempty"`
	Topology     []Region      `json:"topology,omitempty"`
	Rules        []Rule        `json:"correlation_rules,omitempty"`
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

type Settings struct {
	DefaultCluster string         `json:"default_cluster,omitempty" yaml:"default_cluster"`
	KubeconfigDir  string         `json:"kubeconfig_dir,omitempty" yaml:"kubeconfig_dir"`
	GitLabURL      string         `json:"gitlab_url,omitempty" yaml:"gitlab_url"`
	GitOpsProject  string         `json:"gitops_project,omitempty" yaml:"gitops_project"`
	GitOpsRef      string         `json:"gitops_ref,omitempty" yaml:"gitops_ref"`
	Quality        QualitySetting `json:"quality,omitempty" yaml:"quality"`
}

type QualitySetting struct {
	Enabled         *bool  `json:"enabled,omitempty" yaml:"enabled"`
	RunnerImage     string `json:"runner_image,omitempty" yaml:"runner_image"`
	ImagePullSecret string `json:"image_pull_secret,omitempty" yaml:"image_pull_secret"`
	Ref             string `json:"ref,omitempty" yaml:"ref"`
	TTLSeconds      int    `json:"ttl_seconds,omitempty" yaml:"ttl_seconds"`
	DeadlineSeconds int    `json:"deadline_seconds,omitempty" yaml:"deadline_seconds"`
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
	Cluster       string             `json:"cluster,omitempty" yaml:"cluster"`
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

type Agent struct {
	Name          string             `json:"name" yaml:"name"`
	Environment   string             `json:"environment,omitempty" yaml:"environment"`
	URL           string             `json:"url,omitempty" yaml:"url"`
	Default       bool               `json:"default,omitempty" yaml:"default"`
	CredentialRef string             `json:"credential_ref,omitempty" yaml:"credential_ref"`
	Source        string             `json:"source,omitempty" yaml:"-"`
	Credential    *CredentialRuntime `json:"-" yaml:"-"`
}

type NetworkZone struct {
	Name         string   `json:"name" yaml:"name"`
	Region       string   `json:"region,omitempty" yaml:"region"`
	Zone         string   `json:"zone,omitempty" yaml:"zone"`
	CIDRs        []string `json:"cidrs,omitempty" yaml:"cidrs"`
	EntryPoints  []string `json:"entrypoints,omitempty" yaml:"entrypoints"`
	Coverage     string   `json:"coverage,omitempty" yaml:"coverage"`
	ActionPolicy string   `json:"action_policy,omitempty" yaml:"action_policy"`
	Description  string   `json:"description,omitempty" yaml:"description"`
	Source       string   `json:"source,omitempty" yaml:"-"`
}

type AssetSource struct {
	Name          string             `json:"name" yaml:"name"`
	Kind          string             `json:"kind,omitempty" yaml:"kind"`
	Region        string             `json:"region,omitempty" yaml:"region"`
	NetworkZone   string             `json:"network_zone,omitempty" yaml:"network_zone"`
	URL           string             `json:"url,omitempty" yaml:"url"`
	CredentialRef string             `json:"credential_ref,omitempty" yaml:"credential_ref"`
	Enabled       *bool              `json:"enabled,omitempty" yaml:"enabled"`
	Coverage      string             `json:"coverage,omitempty" yaml:"coverage"`
	Note          string             `json:"note,omitempty" yaml:"note"`
	Source        string             `json:"source,omitempty" yaml:"-"`
	Credential    *CredentialRuntime `json:"-" yaml:"-"`
}

type Asset struct {
	Name            string            `json:"name" yaml:"name"`
	Hostname        string            `json:"hostname,omitempty" yaml:"hostname"`
	IPs             []string          `json:"ips,omitempty" yaml:"ips"`
	AssetType       string            `json:"asset_type,omitempty" yaml:"asset_type"`
	Region          string            `json:"region,omitempty" yaml:"region"`
	NetworkZone     string            `json:"network_zone,omitempty" yaml:"network_zone"`
	Status          string            `json:"status,omitempty" yaml:"status"`
	Owner           string            `json:"owner,omitempty" yaml:"owner"`
	Sources         []string          `json:"sources,omitempty" yaml:"sources"`
	ExpectedSources []string          `json:"expected_sources,omitempty" yaml:"expected_sources"`
	Labels          map[string]string `json:"labels,omitempty" yaml:"labels"`
	Source          string            `json:"source,omitempty" yaml:"-"`
}

type Flow struct {
	Name        string      `json:"name" yaml:"name"`
	Type        string      `json:"type,omitempty" yaml:"type"`
	Cluster     string      `json:"cluster,omitempty" yaml:"cluster"`
	Environment string      `json:"environment,omitempty" yaml:"environment"`
	Region      string      `json:"region,omitempty" yaml:"region"`
	Service     string      `json:"service,omitempty" yaml:"service"`
	Window      FlowWindow  `json:"window,omitempty" yaml:"window"`
	MatchKeys   []string    `json:"match_keys,omitempty" yaml:"match_keys"`
	Stages      []FlowStage `json:"stages,omitempty" yaml:"stages"`
	Source      string      `json:"source,omitempty" yaml:"-"`
}

type FlowWindow struct {
	Default string `json:"default,omitempty" yaml:"default"`
	Max     string `json:"max,omitempty" yaml:"max"`
}

type FlowStage struct {
	Name             string            `json:"name" yaml:"name"`
	Type             string            `json:"type,omitempty" yaml:"type"`
	Service          string            `json:"service,omitempty" yaml:"service"`
	Namespace        string            `json:"namespace,omitempty" yaml:"namespace"`
	Workload         string            `json:"workload,omitempty" yaml:"workload"`
	DefaultContainer string            `json:"default_container,omitempty" yaml:"default_container"`
	Containers       []FlowContainer   `json:"containers,omitempty" yaml:"containers"`
	Datasource       string            `json:"datasource,omitempty" yaml:"datasource"`
	Topic            string            `json:"topic,omitempty" yaml:"topic"`
	ConsumerGroup    string            `json:"consumer_group,omitempty" yaml:"consumer_group"`
	Database         string            `json:"database,omitempty" yaml:"database"`
	Table            string            `json:"table,omitempty" yaml:"table"`
	Endpoint         string            `json:"endpoint,omitempty" yaml:"endpoint"`
	Evidence         map[string]any    `json:"evidence,omitempty" yaml:"evidence"`
	Options          map[string]string `json:"options,omitempty" yaml:"options"`
}

type FlowContainer struct {
	Name string `json:"name" yaml:"name"`
	Role string `json:"role,omitempty" yaml:"role"`
}

type Inspection struct {
	Name        string            `json:"name" yaml:"name"`
	Cluster     string            `json:"cluster,omitempty" yaml:"cluster"`
	Environment string            `json:"environment,omitempty" yaml:"environment"`
	Region      string            `json:"region,omitempty" yaml:"region"`
	Schedule    string            `json:"schedule,omitempty" yaml:"schedule"`
	Scope       InspectionScope   `json:"scope,omitempty" yaml:"scope"`
	Checks      []InspectionCheck `json:"checks,omitempty" yaml:"checks"`
	Source      string            `json:"source,omitempty" yaml:"-"`
}

type InspectionScope struct {
	Namespaces []string `json:"namespaces,omitempty" yaml:"namespaces"`
	Services   []string `json:"services,omitempty" yaml:"services"`
	Flows      []string `json:"flows,omitempty" yaml:"flows"`
}

type InspectionCheck struct {
	Name       string         `json:"name" yaml:"name"`
	Type       string         `json:"type,omitempty" yaml:"type"`
	Enabled    *bool          `json:"enabled,omitempty" yaml:"enabled"`
	Datasource string         `json:"datasource,omitempty" yaml:"datasource"`
	Flows      []string       `json:"flows,omitempty" yaml:"flows"`
	Thresholds map[string]any `json:"thresholds,omitempty" yaml:"thresholds"`
	Options    map[string]any `json:"options,omitempty" yaml:"options"`
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

type agentDocument struct {
	APIVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind"`
	Metadata   Metadata  `yaml:"metadata"`
	Spec       AgentSpec `yaml:"spec"`
}

type AgentSpec Agent

type networkZoneDocument struct {
	APIVersion string          `yaml:"apiVersion"`
	Kind       string          `yaml:"kind"`
	Metadata   Metadata        `yaml:"metadata"`
	Spec       NetworkZoneSpec `yaml:"spec"`
}

type NetworkZoneSpec NetworkZone

type assetSourceDocument struct {
	APIVersion string          `yaml:"apiVersion"`
	Kind       string          `yaml:"kind"`
	Metadata   Metadata        `yaml:"metadata"`
	Spec       AssetSourceSpec `yaml:"spec"`
}

type AssetSourceSpec AssetSource

type assetDocument struct {
	APIVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind"`
	Metadata   Metadata  `yaml:"metadata"`
	Spec       AssetSpec `yaml:"spec"`
}

type AssetSpec Asset

type bulkDocument struct {
	Version      string        `yaml:"version"`
	Settings     Settings      `yaml:"settings"`
	Services     []Service     `yaml:"services"`
	Datasources  []Datasource  `yaml:"datasources"`
	Credentials  []Credential  `yaml:"credentials"`
	Clusters     []Cluster     `yaml:"clusters"`
	Agents       []Agent       `yaml:"agents"`
	NetworkZones []NetworkZone `yaml:"network_zones"`
	AssetSources []AssetSource `yaml:"asset_sources"`
	Assets       []Asset       `yaml:"assets"`
	Flows        []Flow        `yaml:"flows"`
	Inspections  []Inspection  `yaml:"inspections"`
	Topology     []Region      `yaml:"topology"`
	Rules        []Rule        `yaml:"correlation_rules"`
}

type flowDocument struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       FlowSpec `yaml:"spec"`
}

type FlowSpec Flow

type inspectionDocument struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	Metadata   Metadata       `yaml:"metadata"`
	Spec       InspectionSpec `yaml:"spec"`
}

type InspectionSpec Inspection

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
	walkRoot := resolvedConfigRoot(dir)
	files, err := yamlFiles(walkRoot)
	if err != nil {
		cfg.Valid = false
		cfg.Errors = append(cfg.Errors, err.Error())
		return cfg
	}
	cfg.Files = relativeFiles(walkRoot, files)
	cfg.Commit = readFirstExisting(
		filepath.Join(walkRoot, ".git", "refs", "heads", "main"),
		filepath.Join(walkRoot, "REVISION"),
		filepath.Join(walkRoot, "revision"),
		filepath.Join(dir, "REVISION"),
		filepath.Join(dir, "revision"),
	)
	if cfg.Commit == "" {
		cfg.Commit = commitFromGitSyncWorktree(walkRoot)
	}
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

func resolvedConfigRoot(dir string) string {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return dir
	}
	return resolved
}

func commitFromGitSyncWorktree(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/.worktrees/")
	if len(parts) < 2 {
		return ""
	}
	commit := strings.Trim(strings.Split(parts[len(parts)-1], "/")[0], " ")
	if commit == "" || strings.Contains(commit, ".") {
		return ""
	}
	return commit
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
			"agents":            len(c.Agents),
			"network_zones":     len(c.NetworkZones),
			"asset_sources":     len(c.AssetSources),
			"assets":            len(c.Assets),
			"flows":             len(c.Flows),
			"inspections":       len(c.Inspections),
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
		if strings.HasPrefix(entry.Name(), ".") {
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
	case "agent":
		var doc agentDocument
		if err := node.Decode(&doc); err != nil {
			cfg.Errors = append(cfg.Errors, fmt.Sprintf("%s: %v", path, err))
			return
		}
		item := Agent(doc.Spec)
		item.Name = firstNonEmpty(item.Name, doc.Metadata.Name)
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Agents = append(cfg.Agents, item)
	case "networkzone", "network_zone":
		var doc networkZoneDocument
		if err := node.Decode(&doc); err != nil {
			cfg.Errors = append(cfg.Errors, fmt.Sprintf("%s: %v", path, err))
			return
		}
		item := NetworkZone(doc.Spec)
		item.Name = firstNonEmpty(item.Name, doc.Metadata.Name)
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.NetworkZones = append(cfg.NetworkZones, item)
	case "assetsource", "asset_source":
		var doc assetSourceDocument
		if err := node.Decode(&doc); err != nil {
			cfg.Errors = append(cfg.Errors, fmt.Sprintf("%s: %v", path, err))
			return
		}
		item := AssetSource(doc.Spec)
		item.Name = firstNonEmpty(item.Name, doc.Metadata.Name)
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.AssetSources = append(cfg.AssetSources, item)
	case "asset":
		var doc assetDocument
		if err := node.Decode(&doc); err != nil {
			cfg.Errors = append(cfg.Errors, fmt.Sprintf("%s: %v", path, err))
			return
		}
		item := Asset(doc.Spec)
		item.Name = firstNonEmpty(item.Name, doc.Metadata.Name)
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Assets = append(cfg.Assets, item)
	case "flow":
		var doc flowDocument
		if err := node.Decode(&doc); err != nil {
			cfg.Errors = append(cfg.Errors, fmt.Sprintf("%s: %v", path, err))
			return
		}
		item := Flow(doc.Spec)
		item.Name = firstNonEmpty(item.Name, doc.Metadata.Name)
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Flows = append(cfg.Flows, item)
	case "inspection":
		var doc inspectionDocument
		if err := node.Decode(&doc); err != nil {
			cfg.Errors = append(cfg.Errors, fmt.Sprintf("%s: %v", path, err))
			return
		}
		item := Inspection(doc.Spec)
		item.Name = firstNonEmpty(item.Name, doc.Metadata.Name)
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Inspections = append(cfg.Inspections, item)
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
	cfg.Settings = mergeSettings(cfg.Settings, doc.Settings)
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
	for _, item := range doc.Agents {
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Agents = append(cfg.Agents, item)
	}
	for _, item := range doc.NetworkZones {
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.NetworkZones = append(cfg.NetworkZones, item)
	}
	for _, item := range doc.AssetSources {
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.AssetSources = append(cfg.AssetSources, item)
	}
	for _, item := range doc.Assets {
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Assets = append(cfg.Assets, item)
	}
	for _, item := range doc.Flows {
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Flows = append(cfg.Flows, item)
	}
	for _, item := range doc.Inspections {
		item.Source = "file:" + filepath.ToSlash(path)
		cfg.Inspections = append(cfg.Inspections, item)
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
	for _, item := range cfg.Agents {
		if item.Name == "" {
			cfg.Errors = append(cfg.Errors, "agent entry missing name")
		}
		if item.URL == "" {
			cfg.Warnings = append(cfg.Warnings, "agent "+item.Name+" has no URL")
		}
		if item.CredentialRef != "" && credentialByName(cfg.Credentials, item.CredentialRef) == nil {
			cfg.Warnings = append(cfg.Warnings, "agent "+item.Name+" references missing credential "+item.CredentialRef)
		}
	}
	for _, item := range cfg.NetworkZones {
		if item.Name == "" {
			cfg.Errors = append(cfg.Errors, "network zone entry missing name")
		}
		if len(item.CIDRs) == 0 && len(item.EntryPoints) == 0 {
			cfg.Warnings = append(cfg.Warnings, "network zone "+item.Name+" has no CIDRs or entrypoints")
		}
		for _, cidr := range item.CIDRs {
			if _, err := netip.ParsePrefix(strings.TrimSpace(cidr)); err != nil {
				cfg.Errors = append(cfg.Errors, "network zone "+item.Name+" invalid CIDR "+cidr)
			}
		}
		for _, ip := range item.EntryPoints {
			if _, err := netip.ParseAddr(strings.TrimSpace(ip)); err != nil {
				cfg.Errors = append(cfg.Errors, "network zone "+item.Name+" invalid entrypoint "+ip)
			}
		}
	}
	for _, item := range cfg.AssetSources {
		if item.Name == "" {
			cfg.Errors = append(cfg.Errors, "asset source entry missing name")
		}
		if item.Kind == "" {
			cfg.Errors = append(cfg.Errors, "asset source "+item.Name+" missing kind")
		}
		if item.CredentialRef != "" && credentialByName(cfg.Credentials, item.CredentialRef) == nil {
			cfg.Warnings = append(cfg.Warnings, "asset source "+item.Name+" references missing credential "+item.CredentialRef)
		}
	}
	for _, item := range cfg.Assets {
		if item.Name == "" {
			cfg.Errors = append(cfg.Errors, "asset entry missing name")
		}
		for _, ip := range item.IPs {
			if _, err := netip.ParseAddr(strings.TrimSpace(ip)); err != nil {
				cfg.Errors = append(cfg.Errors, "asset "+item.Name+" invalid IP "+ip)
			}
		}
	}
	for _, item := range cfg.Flows {
		if item.Name == "" {
			cfg.Errors = append(cfg.Errors, "flow entry missing name")
		}
		if item.Cluster == "" {
			cfg.Warnings = append(cfg.Warnings, "flow "+item.Name+" has no cluster")
		}
		if len(item.Stages) == 0 {
			cfg.Warnings = append(cfg.Warnings, "flow "+item.Name+" has no stages")
		}
		for _, stage := range item.Stages {
			if stage.Name == "" {
				cfg.Warnings = append(cfg.Warnings, "flow "+item.Name+" has a stage without name")
			}
			if stage.Type == "" {
				cfg.Warnings = append(cfg.Warnings, "flow "+item.Name+" stage "+stage.Name+" has no type")
			}
		}
	}
	for _, item := range cfg.Inspections {
		if item.Name == "" {
			cfg.Errors = append(cfg.Errors, "inspection entry missing name")
		}
		if item.Cluster == "" {
			cfg.Warnings = append(cfg.Warnings, "inspection "+item.Name+" has no cluster")
		}
		if len(item.Checks) == 0 {
			cfg.Warnings = append(cfg.Warnings, "inspection "+item.Name+" has no checks")
		}
		for _, check := range item.Checks {
			if check.Name == "" {
				cfg.Warnings = append(cfg.Warnings, "inspection "+item.Name+" has a check without name")
			}
			if check.Type == "" {
				cfg.Warnings = append(cfg.Warnings, "inspection "+item.Name+" check "+check.Name+" has no type")
			}
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
	for idx := range cfg.Agents {
		ref := cfg.Agents[idx].CredentialRef
		if ref == "" {
			continue
		}
		cred := credentialByName(cfg.Credentials, ref)
		if cred == nil {
			continue
		}
		cfg.Agents[idx].Credential = &CredentialRuntime{
			Username: cred.Username,
			Password: cred.Password,
		}
	}
	for idx := range cfg.AssetSources {
		ref := cfg.AssetSources[idx].CredentialRef
		if ref == "" {
			continue
		}
		cred := credentialByName(cfg.Credentials, ref)
		if cred == nil {
			continue
		}
		cfg.AssetSources[idx].Credential = &CredentialRuntime{
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
	cfg.Agents = dedupeByName(cfg.Agents, func(item Agent) string { return item.Name })
	cfg.NetworkZones = dedupeByName(cfg.NetworkZones, func(item NetworkZone) string { return item.Name })
	cfg.AssetSources = dedupeByName(cfg.AssetSources, func(item AssetSource) string { return item.Name })
	cfg.Assets = dedupeByName(cfg.Assets, func(item Asset) string { return item.Name })
	cfg.Flows = dedupeByName(cfg.Flows, func(item Flow) string { return item.Name })
	cfg.Inspections = dedupeByName(cfg.Inspections, func(item Inspection) string { return item.Name })
	cfg.Topology = dedupeByName(cfg.Topology, func(item Region) string { return item.Name })
	cfg.Rules = dedupeByName(cfg.Rules, func(item Rule) string { return item.Name })
}

func mergeSettings(base, next Settings) Settings {
	if next.DefaultCluster != "" {
		base.DefaultCluster = next.DefaultCluster
	}
	if next.KubeconfigDir != "" {
		base.KubeconfigDir = next.KubeconfigDir
	}
	if next.GitLabURL != "" {
		base.GitLabURL = next.GitLabURL
	}
	if next.GitOpsProject != "" {
		base.GitOpsProject = next.GitOpsProject
	}
	if next.GitOpsRef != "" {
		base.GitOpsRef = next.GitOpsRef
	}
	if next.Quality.Enabled != nil {
		base.Quality.Enabled = next.Quality.Enabled
	}
	if next.Quality.RunnerImage != "" {
		base.Quality.RunnerImage = next.Quality.RunnerImage
	}
	if next.Quality.ImagePullSecret != "" {
		base.Quality.ImagePullSecret = next.Quality.ImagePullSecret
	}
	if next.Quality.Ref != "" {
		base.Quality.Ref = next.Quality.Ref
	}
	if next.Quality.TTLSeconds > 0 {
		base.Quality.TTLSeconds = next.Quality.TTLSeconds
	}
	if next.Quality.DeadlineSeconds > 0 {
		base.Quality.DeadlineSeconds = next.Quality.DeadlineSeconds
	}
	return base
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
