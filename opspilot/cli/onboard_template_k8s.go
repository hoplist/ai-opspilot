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
