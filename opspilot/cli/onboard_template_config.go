package main

import (
	"fmt"
	"strings"
)

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
%s
release:
  prometheusSource: %s
`, c.Name, c.GitLabProject, c.Organization, c.Group, c.Project, c.Language, c.BuildEntry, c.BuildOutput, c.Port, c.HealthPath, c.Namespace, firstNonEmpty(c.NamespaceSrc, "manual"), c.Replicas, c.Container, c.Resources.Profile, c.Resources.RequestCPU, c.Resources.RequestMemory, c.Resources.LimitCPU, c.Resources.LimitMemory, c.NamespaceGuard.LimitRange, c.NamespaceGuard.ResourceQuota, c.NamespaceGuard.RequestsCPU, c.NamespaceGuard.RequestsMemory, c.NamespaceGuard.LimitsCPU, c.NamespaceGuard.LimitsMemory, c.NamespaceGuard.Pods, c.DockerMode, c.DockerPath, c.CIMode, middlewareConfigTemplate(c.Middleware), storageConfigTemplate(c.Storage), configSourcesConfigTemplate(c.ConfigSources), c.PromSource)
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

func configSourcesConfigTemplate(items []onboardConfigSourceConfig) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("configSources:\n")
	for _, item := range items {
		b.WriteString(fmt.Sprintf("  %s:\n", item.Name))
		b.WriteString(fmt.Sprintf("    type: %s\n", item.Type))
		b.WriteString(fmt.Sprintf("    required: %t\n", item.Required))
		if item.AppID != "" {
			b.WriteString(fmt.Sprintf("    appId: %s\n", item.AppID))
		}
		if item.Env != "" {
			b.WriteString(fmt.Sprintf("    env: %s\n", item.Env))
		}
		if item.Cluster != "" {
			b.WriteString(fmt.Sprintf("    cluster: %s\n", item.Cluster))
		}
		if len(item.Namespaces) > 0 {
			b.WriteString(fmt.Sprintf("    namespaces: %s\n", strings.Join(item.Namespaces, ",")))
		}
		if item.Meta != "" {
			b.WriteString(fmt.Sprintf("    meta: %s\n", item.Meta))
		}
		if item.ConfigMap != "" {
			b.WriteString(fmt.Sprintf("    configMap: %s\n", item.ConfigMap))
		}
		if item.TokenSecret != "" {
			b.WriteString(fmt.Sprintf("    tokenSecret: %s\n", item.TokenSecret))
		}
		if item.InjectMode != "" {
			b.WriteString(fmt.Sprintf("    inject: %s\n", item.InjectMode))
		}
		if item.EnvFlag != "" {
			b.WriteString(fmt.Sprintf("    envFlag: %s\n", item.EnvFlag))
		}
		if item.MetaFlag != "" {
			b.WriteString(fmt.Sprintf("    metaFlag: %s\n", item.MetaFlag))
		}
		if item.MountPath != "" {
			b.WriteString(fmt.Sprintf("    mountPath: %s\n", item.MountPath))
		}
		if item.Reason != "" {
			b.WriteString(fmt.Sprintf("    reason: %s\n", item.Reason))
		}
	}
	return b.String()
}
