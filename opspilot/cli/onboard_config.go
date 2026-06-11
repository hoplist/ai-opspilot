package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

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
	cfg.ConfigSources = configSourcesFromValues(values)
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
	c.ConfigSources = normalizeConfigSources(*c, c.ConfigSources)
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

func configSourcesFromValues(values map[string]string) []onboardConfigSourceConfig {
	names := map[string]bool{}
	for key := range values {
		if !strings.HasPrefix(key, "configSources.") {
			continue
		}
		rest := strings.TrimPrefix(key, "configSources.")
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
	items := []onboardConfigSourceConfig{}
	for _, name := range ordered {
		prefix := "configSources." + name + "."
		items = append(items, onboardConfigSourceConfig{
			Name:        name,
			Type:        values[prefix+"type"],
			Required:    boolFromString(values[prefix+"required"], false),
			AppID:       values[prefix+"appId"],
			Env:         values[prefix+"env"],
			Cluster:     values[prefix+"cluster"],
			Namespaces:  splitCSV(values[prefix+"namespaces"]),
			Meta:        values[prefix+"meta"],
			ConfigMap:   values[prefix+"configMap"],
			TokenSecret: values[prefix+"tokenSecret"],
			InjectMode:  values[prefix+"inject"],
			EnvFlag:     values[prefix+"envFlag"],
			MetaFlag:    values[prefix+"metaFlag"],
			MountPath:   values[prefix+"mountPath"],
			Reason:      values[prefix+"reason"],
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
