package catalog

import "strings"

// Credential records metadata about a secret without exposing the secret value.
type Credential struct {
	Name        string   `json:"name"`
	Class       string   `json:"class,omitempty"`
	Environment string   `json:"environment,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	Storage     string   `json:"storage,omitempty"`
	Namespace   string   `json:"namespace,omitempty"`
	UsedBy      []string `json:"used_by,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	Owner       string   `json:"owner,omitempty"`
	Rotation    string   `json:"rotation,omitempty"`
	Source      string   `json:"source"`
}

// Cluster records a server-side datasource bundle for a Kubernetes cluster.
type Cluster struct {
	Name           string `json:"name"`
	Environment    string `json:"environment,omitempty"`
	KubernetesMode string `json:"kubernetes_mode,omitempty"`
	KubernetesRef  string `json:"kubernetes_ref,omitempty"`
	KubeconfigPath string `json:"kubeconfig_path,omitempty"`
	KubeContext    string `json:"kube_context,omitempty"`
	Prometheus     string `json:"prometheus,omitempty"`
	Logs           string `json:"logs,omitempty"`
	GitOpsProject  string `json:"gitops_project,omitempty"`
	GitOpsPath     string `json:"gitops_path,omitempty"`
	ArgoNamespace  string `json:"argocd_namespace,omitempty"`
	Registry       string `json:"registry,omitempty"`
	Source         string `json:"source"`
}

type CredentialCatalog struct {
	Version string       `json:"version"`
	Source  string       `json:"source"`
	Count   int          `json:"count"`
	Items   []Credential `json:"items"`
}

type ClusterCatalog struct {
	Version string    `json:"version"`
	Source  string    `json:"source"`
	Count   int       `json:"count"`
	Items   []Cluster `json:"items"`
}

type Service struct {
	Name          string   `json:"name"`
	Environment   string   `json:"environment,omitempty"`
	Group         string   `json:"group,omitempty"`
	Project       string   `json:"project,omitempty"`
	Owner         string   `json:"owner,omitempty"`
	Repo          string   `json:"repo,omitempty"`
	Namespace     string   `json:"namespace,omitempty"`
	Deployment    string   `json:"deployment,omitempty"`
	Container     string   `json:"container,omitempty"`
	Image         string   `json:"image,omitempty"`
	GitLab        string   `json:"gitlab_project,omitempty"`
	GitOps        string   `json:"gitops_path,omitempty"`
	ArgoCD        string   `json:"argocd_app,omitempty"`
	Port          string   `json:"port,omitempty"`
	ConfigSources []string `json:"config_sources,omitempty"`
	Middleware    []string `json:"middleware,omitempty"`
	Dependencies  []string `json:"dependencies,omitempty"`
	Storage       []string `json:"storage,omitempty"`
	ReleaseMapped bool     `json:"release_mapped"`
	Source        string   `json:"source"`
}

type ServiceSeed struct {
	Name       string
	Namespace  string
	Deployment string
	Container  string
	Source     string
	Image      string
	GitLab     string
	GitOps     string
	ArgoCD     string
}

type ServiceCatalog struct {
	Version string    `json:"version"`
	Source  string    `json:"source"`
	Count   int       `json:"count"`
	Items   []Service `json:"items"`
}

func CredentialsFromEnv(raw string) (CredentialCatalog, []string) {
	items, warnings := parseCredentials(raw)
	return CredentialCatalog{
		Version: "v1",
		Source:  sourceName(raw),
		Count:   len(items),
		Items:   items,
	}, warnings
}

func ClustersFromEnv(raw string) (ClusterCatalog, []string) {
	items, warnings := parseClusters(raw)
	return ClusterCatalog{
		Version: "v1",
		Source:  sourceName(raw),
		Count:   len(items),
		Items:   items,
	}, warnings
}

func ServicesFromEnv(raw string, seeds []ServiceSeed) (ServiceCatalog, []string) {
	items, warnings := parseServices(raw)
	byName := map[string]int{}
	for idx, item := range items {
		byName[item.Name] = idx
	}
	for _, seed := range seeds {
		if seed.Name == "" {
			continue
		}
		if idx, ok := byName[seed.Name]; ok {
			items[idx] = mergeSeed(items[idx], seed)
			continue
		}
		items = append(items, serviceFromSeed(seed))
	}
	return ServiceCatalog{
		Version: "v1",
		Source:  serviceSource(raw, seeds),
		Count:   len(items),
		Items:   items,
	}, warnings
}

func parseCredentials(raw string) ([]Credential, []string) {
	out := []Credential{}
	warnings := []string{}
	for _, entry := range splitEntries(raw) {
		attrs := parseAttrs(entry)
		name := firstNonEmpty(attrs["name"], attrs["secret"], attrs["id"])
		if name == "" {
			warnings = append(warnings, "credential catalog entry skipped: missing name")
			continue
		}
		out = append(out, Credential{
			Name:        name,
			Class:       attrs["class"],
			Environment: attrs["environment"],
			Scope:       attrs["scope"],
			Storage:     attrs["storage"],
			Namespace:   attrs["namespace"],
			UsedBy:      splitList(attrs["used_by"]),
			Permissions: splitList(attrs["permissions"]),
			Owner:       attrs["owner"],
			Rotation:    attrs["rotation"],
			Source:      "env",
		})
	}
	return out, warnings
}

func parseClusters(raw string) ([]Cluster, []string) {
	out := []Cluster{}
	warnings := []string{}
	for _, entry := range splitEntries(raw) {
		name, attrsRaw, ok := strings.Cut(entry, "=")
		attrs := map[string]string{}
		if ok {
			attrs = parseAttrs(attrsRaw)
			name = strings.TrimSpace(name)
		} else {
			attrs = parseAttrs(entry)
			name = attrs["name"]
		}
		if name == "" {
			warnings = append(warnings, "cluster catalog entry skipped: missing name")
			continue
		}
		out = append(out, Cluster{
			Name:           name,
			Environment:    attrs["environment"],
			KubernetesMode: firstNonEmpty(attrs["kubernetes"], attrs["kubernetes_mode"], attrs["k8s"]),
			KubernetesRef:  firstNonEmpty(attrs["secret"], attrs["service_account"], attrs["kubernetes_ref"], attrs["ref"]),
			KubeconfigPath: firstNonEmpty(attrs["kubeconfig"], attrs["kubeconfig_path"], attrs["kubeconfig_file"]),
			KubeContext:    firstNonEmpty(attrs["context"], attrs["kube_context"], attrs["kubeconfig_context"]),
			Prometheus:     attrs["prometheus"],
			Logs:           attrs["logs"],
			GitOpsProject:  attrs["gitops_project"],
			GitOpsPath:     firstNonEmpty(attrs["gitops_path"], attrs["path"]),
			ArgoNamespace:  firstNonEmpty(attrs["argocd_namespace"], attrs["argocd_ns"]),
			Registry:       attrs["registry"],
			Source:         "env",
		})
	}
	return out, warnings
}

func parseServices(raw string) ([]Service, []string) {
	out := []Service{}
	warnings := []string{}
	for _, entry := range splitEntries(raw) {
		name, attrsRaw, ok := strings.Cut(entry, "=")
		attrs := map[string]string{}
		if ok {
			attrs = parseAttrs(attrsRaw)
			name = strings.TrimSpace(name)
		} else {
			attrs = parseAttrs(entry)
			name = attrs["name"]
		}
		name = firstNonEmpty(name, attrs["service"])
		if name == "" {
			warnings = append(warnings, "service catalog entry skipped: missing name")
			continue
		}
		out = append(out, Service{
			Name:          name,
			Environment:   attrs["environment"],
			Group:         attrs["group"],
			Project:       attrs["project"],
			Owner:         attrs["owner"],
			Repo:          firstNonEmpty(attrs["repo"], attrs["repository"]),
			Namespace:     firstNonEmpty(attrs["namespace"], attrs["ns"]),
			Deployment:    firstNonEmpty(attrs["deployment"], attrs["deploy"]),
			Container:     attrs["container"],
			Image:         attrs["image"],
			GitLab:        firstNonEmpty(attrs["gitlab"], attrs["gitlab_project"]),
			GitOps:        firstNonEmpty(attrs["gitops"], attrs["gitops_path"]),
			ArgoCD:        firstNonEmpty(attrs["argocd"], attrs["argocd_app"]),
			Port:          attrs["port"],
			ConfigSources: splitList(firstNonEmpty(attrs["config_sources"], attrs["config"], attrs["apollo"])),
			Middleware:    splitList(attrs["middleware"]),
			Dependencies:  splitList(firstNonEmpty(attrs["dependencies"], attrs["depends_on"])),
			Storage:       splitList(attrs["storage"]),
			Source:        "env",
		})
	}
	return out, warnings
}

func mergeSeed(item Service, seed ServiceSeed) Service {
	item.Namespace = firstNonEmpty(item.Namespace, seed.Namespace)
	item.Deployment = firstNonEmpty(item.Deployment, seed.Deployment)
	item.Container = firstNonEmpty(item.Container, seed.Container)
	item.Image = firstNonEmpty(item.Image, seed.Image)
	item.GitLab = firstNonEmpty(item.GitLab, seed.GitLab)
	item.GitOps = firstNonEmpty(item.GitOps, seed.GitOps)
	item.ArgoCD = firstNonEmpty(item.ArgoCD, seed.ArgoCD)
	item.ReleaseMapped = true
	if item.Source == "" {
		item.Source = "release"
	}
	if item.Source == "env" {
		item.Source = "env+release"
	}
	return item
}

func serviceFromSeed(seed ServiceSeed) Service {
	return Service{
		Name:          seed.Name,
		Namespace:     seed.Namespace,
		Deployment:    seed.Deployment,
		Container:     seed.Container,
		Image:         seed.Image,
		GitLab:        seed.GitLab,
		GitOps:        seed.GitOps,
		ArgoCD:        seed.ArgoCD,
		ReleaseMapped: true,
		Source:        "release",
	}
}

func splitEntries(raw string) []string {
	out := []string{}
	for _, entry := range strings.Split(raw, ";") {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			out = append(out, entry)
		}
	}
	return out
}

func parseAttrs(raw string) map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(pair), ":")
		if !ok {
			key, value, ok = strings.Cut(strings.TrimSpace(pair), "=")
		}
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func splitList(raw string) []string {
	out := []string{}
	for _, item := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == '|' || r == '+'
	}) {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func sourceName(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "empty"
	}
	return "env"
}

func serviceSource(raw string, seeds []ServiceSeed) string {
	hasRaw := strings.TrimSpace(raw) != ""
	hasSeeds := len(seeds) > 0
	switch {
	case hasRaw && hasSeeds:
		return "env+release"
	case hasRaw:
		return "env"
	case hasSeeds:
		return "release"
	default:
		return "empty"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
