package k8s

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/catalog"
)

const defaultClusterKubeconfigDir = "/var/run/opspilot/clusters"

type RegistryConfig struct {
	CatalogRaw     string
	DefaultCluster string
	KubeconfigDir  string
}

type Registry struct {
	defaultClient *Client
	defaultName   string
	kubeconfigDir string
	clusters      map[string]catalog.Cluster
	warnings      []string
}

func NewRegistry(config RegistryConfig) *Registry {
	catalogData, warnings := catalog.ClustersFromEnv(config.CatalogRaw)
	clusters := map[string]catalog.Cluster{}
	names := []string{}
	for _, item := range catalogData.Items {
		if item.Name == "" {
			continue
		}
		clusters[item.Name] = item
		names = append(names, item.Name)
	}
	sort.Strings(names)
	defaultName := firstNonEmptyString(config.DefaultCluster, env("OPSPILOT_CLUSTER", ""))
	if defaultName == "" && len(names) > 0 {
		defaultName = names[0]
	}
	defaultName = firstNonEmptyString(defaultName, "default")
	defaultClient := NewClient()
	defaultClient.clusterName = defaultName
	return &Registry{
		defaultClient: defaultClient,
		defaultName:   defaultName,
		kubeconfigDir: firstNonEmptyString(config.KubeconfigDir, env("OPSPILOT_CLUSTER_KUBECONFIG_DIR", defaultClusterKubeconfigDir)),
		clusters:      clusters,
		warnings:      warnings,
	}
}

func (r *Registry) DefaultClient() *Client {
	if r == nil || r.defaultClient == nil {
		return NewClient()
	}
	client, _, err := r.ClientFor("")
	if err != nil {
		return r.defaultClient
	}
	return client
}

func (r *Registry) ClientFor(name string) (*Client, []string, error) {
	if r == nil {
		return NewClient(), nil, nil
	}
	warnings := append([]string{}, r.warnings...)
	name = strings.TrimSpace(name)
	if name == "" {
		name = r.defaultName
	}
	if item, ok := r.clusters[name]; ok {
		if !isKubernetesMode(item.KubernetesMode) {
			return nil, warnings, fmt.Errorf("cluster does not have a Kubernetes datasource: %s", name)
		}
		return r.clientFromCluster(item), warnings, nil
	}
	if name == r.defaultName || len(r.clusters) == 0 {
		return r.defaultClient, warnings, nil
	}
	return nil, warnings, fmt.Errorf("cluster is not registered: %s", name)
}

func (r *Registry) Health() map[string]any {
	if r == nil {
		return NewClient().Health()
	}
	names := make([]string, 0, len(r.clusters))
	for name := range r.clusters {
		names = append(names, name)
	}
	sort.Strings(names)
	return map[string]any{
		"default_cluster":   r.defaultName,
		"registered_count":  len(r.clusters),
		"registered_names":  names,
		"kubeconfig_dir":    r.kubeconfigDir,
		"default_client":    r.defaultClient.Health(),
		"catalog_warnings":  r.warnings,
		"remote_secret_dir": r.kubeconfigDir,
	}
}

func (r *Registry) clientFromCluster(item catalog.Cluster) *Client {
	mode := strings.TrimSpace(item.KubernetesMode)
	if mode == "" {
		mode = "kubectl"
	}
	path := strings.TrimSpace(item.KubeconfigPath)
	if path == "" && isRemoteMode(mode) {
		ref := firstNonEmptyString(item.KubernetesRef, item.Name)
		path = pathpkgJoin(r.kubeconfigDir, ref, "kubeconfig")
	}
	return NewClientWithOptions(ClientOptions{
		ClusterName:    item.Name,
		Mode:           mode,
		Kubectl:        env("OPSPILOT_KUBECTL", "kubectl"),
		Host:           env("KUBERNETES_SERVICE_HOST", ""),
		Port:           env("KUBERNETES_SERVICE_PORT", "443"),
		TokenPath:      env("OPSPILOT_SERVICEACCOUNT_TOKEN", "/var/run/secrets/kubernetes.io/serviceaccount/token"),
		CAPath:         env("OPSPILOT_SERVICEACCOUNT_CA", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"),
		KubeconfigPath: path,
		KubeContext:    item.KubeContext,
	})
}

func pathpkgJoin(elements ...string) string {
	return path.Join(elements...)
}

func isRemoteMode(mode string) bool {
	switch strings.TrimSpace(mode) {
	case "remote", "kubeconfig":
		return true
	default:
		return false
	}
}

func isKubernetesMode(mode string) bool {
	switch strings.TrimSpace(mode) {
	case "", "kubectl", "in-cluster", "remote", "kubeconfig":
		return true
	default:
		return false
	}
}
