package main

import (
	"crypto/sha1"
	"encoding/hex"
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
