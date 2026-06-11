package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnboardServicePlan(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opspilot.service.yaml")
	config := `name: skillshub-api
gitlabProject: tpo/devex/skillshub/skillshub-api
ownership:
  organization: tpo
  group: devex
  project: skillshub
language: go
build:
  entry: ./cmd/skillshub-api
  output: build/skillshub-api
runtime:
  port: 8080
  healthPath: /health
deploy:
  namespace: cicd-devex-skillshub
  replicas: 2
  container: skillshub-api
dockerfile:
  mode: existing
  path: Dockerfile
ci:
  mode: include
release:
  prometheusSource: node200-k8s
`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"onboard", "service", "--config", configPath}, &out); err != nil {
		t.Fatal(err)
	}
	var payload onboardResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Service != "skillshub-api" || payload.Mode != "plan" {
		t.Fatalf("payload = %#v", payload)
	}
	if !bytes.Contains(out.Bytes(), []byte("tpo/devex/skillshub/skillshub-api")) {
		t.Fatalf("release mapping missing project: %s", out.String())
	}
}

func TestOnboardServiceWriteSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	config := `name: demo-api
dockerfile:
  mode: generate
`
	if err := os.WriteFile("opspilot.service.yaml", []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("Dockerfile", []byte("FROM custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"onboard", "service", "--write"}, &out); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "FROM custom\n" {
		t.Fatalf("Dockerfile was overwritten: %s", string(body))
	}
	if _, err := os.Stat(filepath.Join("deploy", "k8s", "deployment.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join("deploy", "k8s", "namespace.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join("deploy", "k8s", "limitrange.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join("deploy", "k8s", "resourcequota.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestOnboardCheckDetectsReadyRepository(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	config := `name: demo-api
language: go
dockerfile:
  path: Dockerfile
deploy:
  namespace: cicd-devex-demo
`
	if err := os.WriteFile("opspilot.service.yaml", []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("Dockerfile", []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".gitlab-ci.yml", []byte("include:\n  - file: /ci/templates/buildkit-gitops.go.yml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join("deploy", "k8s"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := onboardServiceConfig{Name: "demo-api", Namespace: "cicd-devex-demo"}
	if err := cfg.defaults(); err != nil {
		t.Fatal(err)
	}
	generated := map[string]string{
		"namespace.yaml":      namespaceTemplate(cfg),
		"limitrange.yaml":     limitRangeTemplate(cfg),
		"resourcequota.yaml":  resourceQuotaTemplate(cfg),
		"serviceaccount.yaml": serviceAccountTemplate(cfg),
		"deployment.yaml":     deploymentTemplate(cfg),
		"service.yaml":        serviceTemplate(cfg),
		"kustomization.yaml":  kustomizationTemplate(cfg),
	}
	for name, body := range generated {
		if err := os.WriteFile(filepath.Join("deploy", "k8s", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := run([]string{"onboard", "check"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload onboardCheckResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Ready {
		t.Fatalf("expected ready check: %s", out.String())
	}
}

func TestOnboardCheckFailsWhenBuildKitMissing(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("opspilot.service.yaml", []byte("name: demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = run([]string{"onboard", "check"}, &out)
	if err == nil {
		t.Fatal("expected check to fail")
	}
	if !bytes.Contains(out.Bytes(), []byte("buildkit_ci")) {
		t.Fatalf("expected buildkit gap: %s", out.String())
	}
}

func TestOnboardCheckBlocksRawHostPath(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	config := `name: demo-api
language: go
dockerfile:
  path: Dockerfile
deploy:
  namespace: cicd-devex-demo
`
	if err := os.WriteFile("opspilot.service.yaml", []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("Dockerfile", []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".gitlab-ci.yml", []byte("include:\n  - file: /ci/templates/buildkit-gitops.go.yml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join("deploy", "k8s"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := onboardServiceConfig{Name: "demo-api", Namespace: "cicd-devex-demo"}
	if err := cfg.defaults(); err != nil {
		t.Fatal(err)
	}
	generated := map[string]string{
		"namespace.yaml":      namespaceTemplate(cfg),
		"limitrange.yaml":     limitRangeTemplate(cfg),
		"resourcequota.yaml":  resourceQuotaTemplate(cfg),
		"serviceaccount.yaml": serviceAccountTemplate(cfg),
		"deployment.yaml": deploymentTemplate(cfg) + `      volumes:
        - name: raw-logs
          hostPath:
            path: /data/logs/demo-api
            type: DirectoryOrCreate
`,
		"service.yaml":       serviceTemplate(cfg),
		"kustomization.yaml": kustomizationTemplate(cfg),
	}
	for name, body := range generated {
		if err := os.WriteFile(filepath.Join("deploy", "k8s", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	err = run([]string{"onboard", "check"}, &out)
	if err == nil {
		t.Fatal("expected onboard check to fail")
	}
	var payload onboardCheckResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !containsString(payload.Missing, "deployment_storage") || !bytes.Contains(out.Bytes(), []byte("outside /data/opspilot/hostpath")) {
		t.Fatalf("expected deployment_storage failure: %s", out.String())
	}
}

func TestOnboardDetectUsesNamespaceCatalog(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("go.mod", []byte("module example.com/skillshub-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join("cmd", "skillshub-api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("cmd", "skillshub-api", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("Dockerfile", []byte("FROM scratch\nEXPOSE 9090\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog := `namespaceMappings:
  tpo/devex/skillshub/*: cicd-devex-skillshub
`
	if err := os.WriteFile("opspilot.namespaces.yaml", []byte(catalog), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"onboard", "detect", "--project", "tpo/devex/skillshub/skillshub-api"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload onboardDetectResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Config.Namespace != "cicd-devex-skillshub" || payload.Config.NamespaceSrc != "catalog" || payload.Config.Port != 9090 || payload.Config.BuildEntry != "./cmd/skillshub-api" {
		t.Fatalf("payload = %#v", payload.Config)
	}
	if payload.Ready {
		t.Fatalf("detect should not be ready while release files are missing: %#v", payload.Gaps)
	}
}

func TestOnboardDetectsSharedMiddlewareIntent(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	goMod := `module example.com/orders-api

require (
	github.com/go-sql-driver/mysql v1.8.1
	github.com/redis/go-redis/v9 v9.7.0
)
`
	if err := os.WriteFile("go.mod", []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".env.example", []byte("MYSQL_DSN=\nREDIS_URL=\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"onboard", "detect", "--project", "tpo/devex/orders/orders-api"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload onboardDetectResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Config.Middleware) != 2 {
		t.Fatalf("middleware = %#v", payload.Config.Middleware)
	}
	if payload.Config.Middleware[0].Kind != "mysql" || payload.Config.Middleware[0].Mode != "shared-database" {
		t.Fatalf("mysql intent = %#v", payload.Config.Middleware[0])
	}
	if payload.Config.Middleware[1].Kind != "redis" || payload.Config.Middleware[1].Mode != "shared-cache" {
		t.Fatalf("redis intent = %#v", payload.Config.Middleware[1])
	}
	if payload.Config.Middleware[0].Secret != "orders-api-mysql-conn" {
		t.Fatalf("secret = %s", payload.Config.Middleware[0].Secret)
	}
}

func TestOnboardDetectsStorageIntent(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("go.mod", []byte("module example.com/demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := "LOG_DIR=/var/log/demo-api\nCACHE_DIR=/tmp/cache\nUPLOAD_DIR=/app/uploads\n"
	if err := os.WriteFile(".env.example", []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"onboard", "detect", "--project", "tpo/devex/demo/demo-api"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload onboardDetectResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Config.Storage) != 3 {
		t.Fatalf("storage = %#v", payload.Config.Storage)
	}
	byName := map[string]onboardStorageConfig{}
	for _, item := range payload.Config.Storage {
		byName[item.Name] = item
	}
	if byName["logs"].Mode != "hostPath" || byName["logs"].MountPath != "/var/log/demo-api" || !strings.HasPrefix(byName["logs"].HostPath, defaultHostPathRoot+"/") {
		t.Fatalf("logs storage = %#v", byName["logs"])
	}
	if byName["runtime"].Mode != "hostPath" || byName["runtime"].MountPath != "/app/uploads" {
		t.Fatalf("runtime storage = %#v", byName["runtime"])
	}
	if byName["cache"].Mode != "emptyDir" || byName["cache"].SizeLimit != "1Gi" {
		t.Fatalf("cache storage = %#v", byName["cache"])
	}
}

func TestOnboardGenerateAutoNamespacesByProject(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("go.mod", []byte("module example.com/demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"onboard", "generate", "--project", "tpo/devex/demo/demo-api", "--write"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload onboardResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("namespace:cicd-devex-demo")) {
		t.Fatalf("expected auto namespace in release mapping: %s", out.String())
	}
	body, err := os.ReadFile("opspilot.service.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("namespaceSource: auto_project")) {
		t.Fatalf("expected auto namespace source: %s", string(body))
	}
}

func TestOnboardRepoWritesAndChecks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"onboard", "repo", "tpo/devex/demo/demo-api", "--repo", dir, "--write"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload onboardRepoResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Service != "demo-api" || payload.Mode != "write" || !payload.Ready {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Namespace != "cicd-devex-demo" {
		t.Fatalf("namespace = %s", payload.Namespace)
	}
	if payload.GitOpsPlan.Path != "clusters/test/apps/devex/demo/demo-api" || payload.GitOpsPlan.ApplicationName != "devex-demo-demo-api" {
		t.Fatalf("gitops plan = %#v", payload.GitOpsPlan)
	}
	if !strings.Contains(payload.GitOpsPlan.Image, "192.168.48.206:5050/tpo/devex/demo/demo-api/demo-api") {
		t.Fatalf("image = %s", payload.GitOpsPlan.Image)
	}
	if _, err := os.Stat(filepath.Join(dir, "opspilot.service.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "deploy", "k8s", "deployment.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "deploy", "k8s", "limitrange.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "deploy", "k8s", "resourcequota.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestOnboardGenerateWritesMiddlewareIntent(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("package.json", []byte(`{"dependencies":{"mysql2":"^3.0.0","ioredis":"^5.0.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"onboard", "generate", "--project", "tpo/devex/orders/orders-api", "--write"}, &out); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("opspilot.service.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range [][]byte{
		[]byte("middleware:"),
		[]byte("mysql:"),
		[]byte("mode: shared-database"),
		[]byte("secret: orders-api-mysql-conn"),
		[]byte("redis:"),
		[]byte("mode: shared-cache"),
	} {
		if !bytes.Contains(body, expected) {
			t.Fatalf("generated config missing %s:\n%s", expected, string(body))
		}
	}
}

func TestOnboardGenerateWritesStorageVolumes(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("go.mod", []byte("module example.com/demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := "LOG_DIR=/var/log/demo-api\nCACHE_DIR=/tmp/cache\nUPLOAD_DIR=/app/uploads\n"
	if err := os.WriteFile(".env.example", []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"onboard", "generate", "--project", "tpo/devex/demo/demo-api", "--write"}, &out); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile("opspilot.service.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range [][]byte{
		[]byte("storage:"),
		[]byte("logs:"),
		[]byte("mode: hostPath"),
		[]byte(defaultHostPathRoot + "/cicd-devex-demo/demo-api/logs"),
		[]byte("cache:"),
		[]byte("mode: emptyDir"),
		[]byte("sizeLimit: 1Gi"),
	} {
		if !bytes.Contains(config, expected) {
			t.Fatalf("generated config missing %s:\n%s", expected, string(config))
		}
	}
	deployment, err := os.ReadFile(filepath.Join("deploy", "k8s", "deployment.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range [][]byte{
		[]byte(`opspilot.io/storage-managed: "true"`),
		[]byte("volumeMounts:"),
		[]byte("hostPath:"),
		[]byte(defaultHostPathRoot + "/cicd-devex-demo/demo-api/logs"),
		[]byte("emptyDir:"),
		[]byte("sizeLimit: 1Gi"),
		[]byte("mountPath: /var/log/demo-api"),
		[]byte("mountPath: /app/uploads"),
	} {
		if !bytes.Contains(deployment, expected) {
			t.Fatalf("generated deployment missing %s:\n%s", expected, string(deployment))
		}
	}
}

func TestOnboardServiceGeneratesApolloConfigSource(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	config := `name: task-server
gitlabProject: tpo/devex/gos/task-server
language: go
build:
  output: build/task-server
deploy:
  namespace: gos
dockerfile:
  mode: generate
ci:
  mode: include
configSources:
  apollo:
    type: apollo
    required: true
    appId: task-server
    env: prod
    cluster: default
    namespaces: application,gms
    meta: http://apolloconfig-server-inner.tpo.xzoa.com
    tokenSecret: task-server-apollo-token
    inject: args
`
	if err := os.WriteFile("opspilot.service.yaml", []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"onboard", "service", "--write"}, &out); err != nil {
		t.Fatal(err)
	}
	configMap, err := os.ReadFile(filepath.Join("deploy", "k8s", "configmap.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range [][]byte{
		[]byte("name: task-server-apollo-config"),
		[]byte(`APOLLO_APP_ID: "task-server"`),
		[]byte(`APOLLO_ENV: "prod"`),
		[]byte(`APOLLO_META: "http://apolloconfig-server-inner.tpo.xzoa.com"`),
	} {
		if !bytes.Contains(configMap, expected) {
			t.Fatalf("generated configmap missing %s:\n%s", expected, string(configMap))
		}
	}
	deployment, err := os.ReadFile(filepath.Join("deploy", "k8s", "deployment.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range [][]byte{
		[]byte(`- "--env=$(APOLLO_ENV)"`),
		[]byte(`- "--cfg=$(APOLLO_META)"`),
		[]byte("name: APOLLO_APP_ID"),
		[]byte("configMapKeyRef:"),
		[]byte("name: task-server-apollo-config"),
		[]byte("name: APOLLO_TOKEN"),
		[]byte("secretKeyRef:"),
		[]byte("name: task-server-apollo-token"),
	} {
		if !bytes.Contains(deployment, expected) {
			t.Fatalf("generated deployment missing %s:\n%s", expected, string(deployment))
		}
	}
	kustomization, err := os.ReadFile(filepath.Join("deploy", "k8s", "kustomization.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(kustomization, []byte("configmap.yaml")) {
		t.Fatalf("kustomization missing configmap.yaml: %s", string(kustomization))
	}
	out.Reset()
	if err := run([]string{"onboard", "check"}, &out); err != nil {
		t.Fatalf("onboard check failed: %v\n%s", err, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("config_source_apollo")) {
		t.Fatalf("onboard check did not report Apollo config source: %s", out.String())
	}
}

func TestOnboardDetectsApolloConfigSource(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("go.mod", []byte("module example.com/task-server\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	main := `package main

func main() {
	_ = "/go/bin/task --env=prod --cfg=http://apolloconfig-server-inner.tpo.xzoa.com"
}
`
	if err := os.WriteFile("main.go", []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"onboard", "detect", "--project", "tpo/devex/gos/task-server"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload onboardDetectResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Config.ConfigSources) != 1 {
		t.Fatalf("config sources = %#v", payload.Config.ConfigSources)
	}
	source := payload.Config.ConfigSources[0]
	if source.Type != "apollo" || source.InjectMode != "args" || source.Env != "prod" || source.Meta != "http://apolloconfig-server-inner.tpo.xzoa.com" {
		t.Fatalf("apollo source = %#v", source)
	}
	if !containsString(payload.Gaps, "configmap_missing") {
		t.Fatalf("expected configmap_missing gap: %#v", payload.Gaps)
	}
}

func TestOnboardGenerateWritesDetectedFiles(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("go.mod", []byte("module example.com/demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog := `namespaceMappings:
  tpo/devex/demo/*: cicd-devex-demo
`
	if err := os.WriteFile("opspilot.namespaces.yaml", []byte(catalog), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"onboard", "generate", "--project", "tpo/devex/demo/demo-api", "--write"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"opspilot.service.yaml",
		"Dockerfile",
		filepath.Join("deploy", "k8s", "namespace.yaml"),
		filepath.Join("deploy", "k8s", "limitrange.yaml"),
		filepath.Join("deploy", "k8s", "resourcequota.yaml"),
		filepath.Join("deploy", "k8s", "serviceaccount.yaml"),
		filepath.Join("deploy", "k8s", "deployment.yaml"),
		filepath.Join("deploy", "k8s", "service.yaml"),
		filepath.Join("deploy", "k8s", "kustomization.yaml"),
		filepath.Join(".opspilot", "quality.yaml"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing generated file %s: %v", path, err)
		}
	}
	deployment, err := os.ReadFile(filepath.Join("deploy", "k8s", "deployment.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(deployment, []byte("imagePullSecrets")) || !bytes.Contains(deployment, []byte("gitlab-registry-pull")) {
		t.Fatalf("generated deployment should include GitLab Registry pull configuration: %s", string(deployment))
	}
	if !bytes.Contains(deployment, []byte("resources:")) || !bytes.Contains(deployment, []byte("cpu: 50m")) || !bytes.Contains(deployment, []byte("memory: 256Mi")) {
		t.Fatalf("generated deployment missing default resource guardrails: %s", string(deployment))
	}
}

func TestDetectLanguageCoversGoldenDemoLanguages(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name: "frontend",
			files: map[string]string{
				"package.json": `{"scripts":{"build":"vite --host 0.0.0.0"},"dependencies":{"vite":"latest"}}`,
			},
			want: "frontend",
		},
		{
			name: "node",
			files: map[string]string{
				"package.json": `{"scripts":{"start":"node server.js"}}`,
			},
			want: "node",
		},
		{
			name: "python",
			files: map[string]string{
				"requirements.txt": "fastapi\nuvicorn\n",
			},
			want: "python",
		},
		{
			name: "java",
			files: map[string]string{
				"pom.xml": "<project></project>\n",
			},
			want: "java",
		},
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chdir(dir); err != nil {
				t.Fatal(err)
			}
			defer func() {
				_ = os.Chdir(wd)
			}()
			for path, body := range tc.files {
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := detectLanguage(); got != tc.want {
				t.Fatalf("detectLanguage() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestOnboardGenerateSupportsFrontendAndJava(t *testing.T) {
	cases := []struct {
		name           string
		project        string
		seedPath       string
		seedBody       string
		wantCI         string
		wantDockerfile []byte
		wantDeployment []byte
	}{
		{
			name:           "frontend",
			project:        "tpo/devex/frontend-demo/frontend-demo",
			seedPath:       "package.json",
			seedBody:       `{"scripts":{"build":"vite --host 0.0.0.0"},"dependencies":{"@vitejs/plugin-react":"latest","vite":"latest"}}`,
			wantCI:         "/ci/templates/buildkit-gitops.frontend.yml",
			wantDockerfile: []byte("nginx:1.27-alpine"),
			wantDeployment: []byte("containerPort: 80"),
		},
		{
			name:           "java",
			project:        "tpo/devex/java-demo/java-demo",
			seedPath:       "pom.xml",
			seedBody:       "<project></project>\n",
			wantCI:         "/ci/templates/buildkit-gitops.java.yml",
			wantDockerfile: []byte("maven:3.9.9-eclipse-temurin-21-alpine"),
			wantDeployment: []byte("containerPort: 8080"),
		},
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chdir(dir); err != nil {
				t.Fatal(err)
			}
			defer func() {
				_ = os.Chdir(wd)
			}()
			if err := os.WriteFile(tc.seedPath, []byte(tc.seedBody), 0o644); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := run([]string{"onboard", "generate", "--project", tc.project, "--write"}, &out); err != nil {
				t.Fatal(err)
			}
			ci, err := os.ReadFile(".gitlab-ci.yml")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(ci, []byte(tc.wantCI)) {
				t.Fatalf("generated CI missing %s: %s", tc.wantCI, string(ci))
			}
			dockerfile, err := os.ReadFile("Dockerfile")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(dockerfile, tc.wantDockerfile) {
				t.Fatalf("generated Dockerfile did not match %q: %s", tc.wantDockerfile, string(dockerfile))
			}
			deployment, err := os.ReadFile(filepath.Join("deploy", "k8s", "deployment.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(deployment, tc.wantDeployment) {
				t.Fatalf("generated deployment did not match %q: %s", tc.wantDeployment, string(deployment))
			}
		})
	}
}

func TestCITemplatesIncludeCodePrecheck(t *testing.T) {
	root := filepath.Join("..", "..", "ci", "templates")
	for _, name := range []string{
		"buildkit-gitops.go.yml",
		"buildkit-gitops.python.yml",
		"buildkit-gitops.node.yml",
		"buildkit-gitops.frontend.yml",
		"buildkit-gitops.java.yml",
	} {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, expected := range [][]byte{
			[]byte("  - code-precheck"),
			[]byte("code-precheck:"),
			[]byte(".opspilot/evidence/code-precheck.json"),
			[]byte("security-reviewer"),
			[]byte("database-optimizer"),
		} {
			if !bytes.Contains(body, expected) {
				t.Fatalf("%s missing %s", name, expected)
			}
		}
	}
	frontend, err := os.ReadFile(filepath.Join(root, "buildkit-gitops.frontend.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range [][]byte{
		[]byte("prebuild:image-smoke"),
		[]byte(".opspilot/evidence/frontend-image-smoke.json"),
		[]byte("vue_runtime_template_without_compiler"),
		[]byte("fix_options"),
		[]byte("automatic_quality_gate"),
		[]byte("human_approval_required"),
	} {
		if !bytes.Contains(frontend, expected) {
			t.Fatalf("frontend template missing %s", expected)
		}
	}
}

func TestPlatformGitLabCIIncludesCodePrecheck(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", ".gitlab-ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range [][]byte{
		[]byte("  - code-precheck"),
		[]byte("code-precheck:"),
		[]byte("repo precheck --repo . --project platform/opspilot --write"),
		[]byte(".opspilot/evidence/code-precheck.json"),
	} {
		if !bytes.Contains(body, expected) {
			t.Fatalf(".gitlab-ci.yml missing %s", expected)
		}
	}
}
