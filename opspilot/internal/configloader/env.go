package configloader

import (
	"strconv"
	"strings"
)

func (c Config) ServiceCatalogRaw() string {
	entries := []string{}
	for _, item := range c.Services {
		if item.Name == "" {
			continue
		}
		attrs := []string{}
		add := func(key, value string) {
			if strings.TrimSpace(value) != "" {
				attrs = append(attrs, key+":"+strings.TrimSpace(value))
			}
		}
		addList := func(key string, values []string) {
			if len(values) > 0 {
				attrs = append(attrs, key+":"+strings.Join(cleanList(values), "|"))
			}
		}
		add("environment", item.Environment)
		add("group", item.Group)
		add("project", item.Project)
		add("owner", item.Owner)
		add("repo", item.Repo)
		addList("domains", item.Domains)
		add("namespace", item.Runtime.Namespace)
		add("deployment", item.Runtime.Deployment)
		add("container", item.Runtime.Container)
		add("source", item.Runtime.Cluster)
		add("image", item.Runtime.Image)
		add("port", item.Runtime.Port)
		addList("app_indexes", item.Logs.AppIndexes)
		addList("message_fields", item.Logs.MessageFields)
		add("gateway", item.Gateway.Datasource)
		add("apisix_index", item.Gateway.APISIXIndex)
		add("gitlab", item.Release.GitLab)
		add("gitops", item.Release.GitOps)
		add("argocd", item.Release.ArgoCD)
		addList("config_sources", item.ConfigSources)
		addList("middleware", item.Middleware)
		addList("dependencies", item.Dependencies)
		addList("storage", item.Storage)
		if item.Correlation.RequireURI != nil {
			add("require_uri", boolString(*item.Correlation.RequireURI))
		}
		addList("path_prefixes", item.Correlation.PathPrefixes)
		if item.Correlation.DefaultWindowSeconds > 0 {
			add("default_window_seconds", intString(item.Correlation.DefaultWindowSeconds))
		}
		entries = append(entries, item.Name+"="+strings.Join(attrs, ","))
	}
	return strings.Join(entries, ";")
}

func (c Config) CredentialCatalogRaw() string {
	entries := []string{}
	for _, item := range c.Credentials {
		if item.Name == "" {
			continue
		}
		attrs := []string{"name=" + item.Name}
		add := func(key, value string) {
			if strings.TrimSpace(value) != "" {
				attrs = append(attrs, key+":"+strings.TrimSpace(value))
			}
		}
		addList := func(key string, values []string) {
			if len(values) > 0 {
				attrs = append(attrs, key+":"+strings.Join(cleanList(values), "|"))
			}
		}
		add("type", item.Type)
		add("class", item.Class)
		add("environment", item.Environment)
		add("scope", item.Scope)
		add("storage", item.Storage)
		add("namespace", item.Namespace)
		add("username", item.Username)
		if item.Password != "" {
			add("password_set", "true")
		}
		addList("used_by", item.UsedBy)
		addList("permissions", item.Permissions)
		add("owner", item.Owner)
		add("rotation", item.Rotation)
		entries = append(entries, strings.Join(attrs, ","))
	}
	return strings.Join(entries, ";")
}

func (c Config) ClusterCatalogRaw() string {
	entries := []string{}
	for _, item := range c.Clusters {
		if item.Name == "" {
			continue
		}
		attrs := []string{}
		add := func(key, value string) {
			if strings.TrimSpace(value) != "" {
				attrs = append(attrs, key+":"+strings.TrimSpace(value))
			}
		}
		add("environment", item.Environment)
		add("kubernetes", item.KubernetesMode)
		add("kubernetes_ref", item.KubernetesRef)
		add("kubeconfig", item.KubeconfigPath)
		add("context", item.KubeContext)
		add("prometheus", item.Prometheus)
		add("logs", item.Logs)
		add("gitops_project", item.GitOpsProject)
		add("path", item.GitOpsPath)
		add("argocd_ns", item.ArgoNamespace)
		add("registry", item.Registry)
		entries = append(entries, item.Name+"="+strings.Join(attrs, ","))
	}
	return strings.Join(entries, ";")
}

func (c Config) NodeAgentsRaw() string {
	entries := []string{}
	for _, item := range c.Agents {
		if item.Name == "" || item.URL == "" {
			continue
		}
		entries = append(entries, item.Name+"="+strings.TrimRight(item.URL, "/"))
	}
	return strings.Join(entries, ",")
}

func (c Config) DefaultNodeAgent() string {
	for _, item := range c.Agents {
		if item.Name != "" && item.Default {
			return item.Name
		}
	}
	if len(c.Agents) > 0 {
		return c.Agents[0].Name
	}
	return ""
}

func (c Config) NodeAgentTokensRaw() string {
	entries := []string{}
	for _, item := range c.Agents {
		if item.Name == "" || item.Credential == nil || item.Credential.Password == "" {
			continue
		}
		entries = append(entries, item.Name+"="+item.Credential.Password)
	}
	return strings.Join(entries, ",")
}

func (c Config) PrometheusDatasourcesRaw() string {
	entries := []string{}
	for _, item := range c.Datasources {
		if item.Name == "" || item.URL == "" || strings.ToLower(item.Kind) != "prometheus" {
			continue
		}
		entries = append(entries, item.Name+"="+strings.TrimRight(item.URL, "/"))
	}
	return strings.Join(entries, ",")
}

func (c Config) DefaultPrometheusSource() string {
	for _, item := range c.Datasources {
		if item.Name != "" && item.URL != "" && strings.ToLower(item.Kind) == "prometheus" {
			return item.Name
		}
	}
	return ""
}

type LogSearchDefaults struct {
	URL             string
	Index           string
	APISIXIndex     string
	ServiceIndex    string
	ServiceURIField string
	Username        string
	Password        string
}

func (c Config) LogSearchDefaults() LogSearchDefaults {
	for _, item := range c.Datasources {
		kind := strings.ToLower(item.Kind)
		if item.Name == "" || item.URL == "" || (kind != "elasticsearch" && kind != "opensearch" && kind != "elk") {
			continue
		}
		out := LogSearchDefaults{
			URL:         strings.TrimRight(item.URL, "/"),
			APISIXIndex: item.Indexes.APISIX,
		}
		if len(item.Indexes.AppDefault) > 0 {
			out.ServiceIndex = item.Indexes.AppDefault[0]
			out.Index = item.Indexes.AppDefault[0]
		} else if len(item.Indexes.App) > 0 {
			out.ServiceIndex = item.Indexes.App[0]
			out.Index = item.Indexes.App[0]
		}
		if item.Fields != nil {
			out.ServiceURIField = firstNonEmpty(item.Fields["service_uri"], item.Fields["message"], item.Fields["log"])
		}
		if item.Credential != nil {
			out.Username, out.Password = item.Credential.Username, item.Credential.Password
		}
		return out
	}
	return LogSearchDefaults{}
}

func (c Config) CorrelationRoutesRaw() string {
	entries := []string{}
	for _, rule := range c.Rules {
		entries = append(entries, ruleEntries(rule)...)
	}
	for _, service := range c.Services {
		if len(service.Domains) == 0 || len(service.Logs.AppIndexes) == 0 {
			continue
		}
		prefixes := service.Correlation.PathPrefixes
		if len(prefixes) == 0 {
			prefixes = []string{""}
		}
		for _, domain := range service.Domains {
			for _, prefix := range prefixes {
				entries = append(entries, joinRouteFields([]string{
					firstNonEmpty(service.Name, domain+prefix),
					domain,
					prefix,
					service.Logs.AppIndexes[0],
					firstNonEmpty(firstList(service.Logs.MessageFields), "msg"),
					"",
					"",
					"",
				}))
			}
		}
	}
	return strings.Join(entries, ";")
}

func ruleEntries(rule Rule) []string {
	if rule.Host == "" || rule.ServiceIndex == "" {
		return nil
	}
	prefixes := rule.PathPrefixes
	if len(prefixes) == 0 {
		prefixes = []string{""}
	}
	out := []string{}
	for _, prefix := range prefixes {
		out = append(out, joinRouteFields([]string{
			rule.Name,
			rule.Host,
			prefix,
			rule.ServiceIndex,
			rule.ServiceURIField,
			rule.ServiceEventField,
			rule.ServiceEventTemplate,
			rule.ServiceFallbackQuery,
		}))
	}
	return out
}

func cleanList(values []string) []string {
	out := []string{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, strings.TrimSpace(value))
		}
	}
	return out
}

func firstList(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func joinRouteFields(fields []string) string {
	for len(fields) < 8 {
		fields = append(fields, "")
	}
	return strings.Join(fields[:8], "|")
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func intString(value int) string {
	return strconv.Itoa(value)
}
