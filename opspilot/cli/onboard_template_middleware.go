package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
)

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
