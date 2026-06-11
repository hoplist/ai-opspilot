package main

import (
	"fmt"
	"strings"
)

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
%s%s
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
%s%s%s`, c.Name, c.Namespace, c.Name, c.Replicas, c.Name, c.Name, storagePodAnnotationsTemplate(c), serviceAccountName(c), c.Container, configArgsTemplate(c.ConfigSources), configEnvTemplate(c.ConfigSources), c.Port, c.Resources.RequestCPU, c.Resources.RequestMemory, c.Resources.LimitCPU, c.Resources.LimitMemory, c.HealthPath, c.HealthPath, middlewareEnvFromTemplate(c.Middleware), containerVolumeMountsTemplate(c), podVolumesTemplate(c))
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

func configArgsTemplate(items []onboardConfigSourceConfig) string {
	for _, item := range items {
		if item.Type != "apollo" || item.InjectMode != "args" {
			continue
		}
		var b strings.Builder
		b.WriteString("          args:\n")
		if item.EnvFlag != "" && item.Env != "" {
			b.WriteString(fmt.Sprintf("            - %q\n", item.EnvFlag+"=$(APOLLO_ENV)"))
		}
		if item.MetaFlag != "" && item.Meta != "" {
			b.WriteString(fmt.Sprintf("            - %q\n", item.MetaFlag+"=$(APOLLO_META)"))
		}
		return b.String()
	}
	return ""
}

func configEnvTemplate(items []onboardConfigSourceConfig) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	wroteHeader := false
	for _, item := range items {
		if item.Type != "apollo" || item.InjectMode == "file" {
			continue
		}
		if !wroteHeader {
			b.WriteString("          env:\n")
			wroteHeader = true
		}
		writeConfigMapEnv := func(envName, key string) {
			b.WriteString(fmt.Sprintf("            - name: %s\n", envName))
			b.WriteString("              valueFrom:\n")
			b.WriteString("                configMapKeyRef:\n")
			b.WriteString(fmt.Sprintf("                  name: %s\n", item.ConfigMap))
			b.WriteString(fmt.Sprintf("                  key: %s\n", key))
		}
		writeConfigMapEnv("APOLLO_APP_ID", "APOLLO_APP_ID")
		writeConfigMapEnv("APOLLO_CLUSTER", "APOLLO_CLUSTER")
		writeConfigMapEnv("APOLLO_NAMESPACES", "APOLLO_NAMESPACES")
		if item.Env != "" {
			writeConfigMapEnv("APOLLO_ENV", "APOLLO_ENV")
		}
		if item.Meta != "" {
			writeConfigMapEnv("APOLLO_META", "APOLLO_META")
		}
		if item.TokenSecret != "" {
			b.WriteString("            - name: APOLLO_TOKEN\n")
			b.WriteString("              valueFrom:\n")
			b.WriteString("                secretKeyRef:\n")
			b.WriteString(fmt.Sprintf("                  name: %s\n", item.TokenSecret))
			b.WriteString("                  key: token\n")
			if !item.Required {
				b.WriteString("                  optional: true\n")
			}
		}
	}
	return b.String()
}

func configSourcesConfigMapTemplate(c onboardServiceConfig) string {
	if len(c.ConfigSources) == 0 {
		return ""
	}
	var b strings.Builder
	for i, item := range c.ConfigSources {
		if item.Type != "apollo" {
			continue
		}
		if i > 0 && b.Len() > 0 {
			b.WriteString("---\n")
		}
		b.WriteString(fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    opspilot.io/config-source: %s
data:
  APOLLO_APP_ID: %s
  APOLLO_CLUSTER: %s
  APOLLO_NAMESPACES: %s
`, item.ConfigMap, c.Namespace, c.Name, item.Type, yamlQuote(item.AppID), yamlQuote(item.Cluster), yamlQuote(strings.Join(item.Namespaces, ","))))
		if item.Env != "" {
			b.WriteString(fmt.Sprintf("  APOLLO_ENV: %s\n", yamlQuote(item.Env)))
		}
		if item.Meta != "" {
			b.WriteString(fmt.Sprintf("  APOLLO_META: %s\n", yamlQuote(item.Meta)))
		}
		if item.InjectMode == "file" {
			b.WriteString("  apollo.yaml: |\n")
			b.WriteString(fmt.Sprintf("    apollo:\n      appId: %s\n      cluster: %s\n      namespaces: %s\n", item.AppID, item.Cluster, strings.Join(item.Namespaces, ",")))
			if item.Env != "" {
				b.WriteString(fmt.Sprintf("      env: %s\n", item.Env))
			}
			if item.Meta != "" {
				b.WriteString(fmt.Sprintf("      meta: %s\n", item.Meta))
			}
		}
	}
	return b.String()
}

func yamlQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}

func containerVolumeMountsTemplate(c onboardServiceConfig) string {
	if len(c.Storage) == 0 && !hasFileConfigSource(c.ConfigSources) {
		return ""
	}
	var b strings.Builder
	b.WriteString("          volumeMounts:\n")
	for _, item := range c.ConfigSources {
		if item.InjectMode != "file" {
			continue
		}
		b.WriteString(fmt.Sprintf("            - name: %s\n", configSourceVolumeName(item)))
		b.WriteString(fmt.Sprintf("              mountPath: %s\n", item.MountPath))
		b.WriteString("              subPath: apollo.yaml\n")
		b.WriteString("              readOnly: true\n")
	}
	for _, item := range c.Storage {
		b.WriteString(fmt.Sprintf("            - name: %s\n", item.Name))
		b.WriteString(fmt.Sprintf("              mountPath: %s\n", item.MountPath))
		if item.ReadOnly {
			b.WriteString("              readOnly: true\n")
		}
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

func podVolumesTemplate(c onboardServiceConfig) string {
	if len(c.Storage) == 0 && !hasFileConfigSource(c.ConfigSources) {
		return ""
	}
	var b strings.Builder
	b.WriteString("      volumes:\n")
	for _, item := range c.ConfigSources {
		if item.InjectMode != "file" {
			continue
		}
		b.WriteString(fmt.Sprintf("        - name: %s\n", configSourceVolumeName(item)))
		b.WriteString("          configMap:\n")
		b.WriteString(fmt.Sprintf("            name: %s\n", item.ConfigMap))
	}
	for _, item := range c.Storage {
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

func hasFileConfigSource(items []onboardConfigSourceConfig) bool {
	for _, item := range items {
		if item.InjectMode == "file" {
			return true
		}
	}
	return false
}

func configSourceVolumeName(item onboardConfigSourceConfig) string {
	return sanitizeDNSLabel(item.Name + "-config")
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
`)
	if len(c.ConfigSources) > 0 {
		b.WriteString("  - configmap.yaml\n")
	}
	b.WriteString(`  - deployment.yaml
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
