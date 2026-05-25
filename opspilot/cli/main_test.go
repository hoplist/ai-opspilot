package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSchemaCommand(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"schema"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["name"] != "opspilot" {
		t.Fatalf("name = %v", payload["name"])
	}
}

func TestConsumeGlobalFlags(t *testing.T) {
	opts := globalOptions{backendURL: "default", output: "json"}
	args := consumeGlobalFlags([]string{"--backend-url", "http://x", "--output", "table", "schema"}, &opts)
	if opts.backendURL != "http://x" {
		t.Fatalf("backend = %s", opts.backendURL)
	}
	if opts.output != "table" {
		t.Fatalf("output = %s", opts.output)
	}
	if len(args) != 1 || args[0] != "schema" {
		t.Fatalf("args = %#v", args)
	}
}

func TestEvidenceRequestCommand(t *testing.T) {
	endpoint, values := evidenceCommand([]string{
		"request",
		"--host", "workflow.tpo.xzoa.com",
		"--uri", "/api/hr/queryUserScheduleList",
		"--service-index", "workflow-server*",
		"--service-uri-field", "msg",
	})
	if endpoint != "/api/evidence/request" {
		t.Fatalf("endpoint = %s", endpoint)
	}
	if values.Get("host") != "workflow.tpo.xzoa.com" {
		t.Fatalf("host = %s", values.Get("host"))
	}
	if values.Get("service_index") != "workflow-server*" {
		t.Fatalf("service_index = %s", values.Get("service_index"))
	}
}

func TestEvidenceRequestServiceOnlyCommand(t *testing.T) {
	_, values := evidenceCommand([]string{
		"request",
		"--uri", "/api/hr/queryUserScheduleList",
		"--service-index", "workflow-server*",
		"--service-only",
	})
	if values.Get("skip_apisix") != "true" {
		t.Fatalf("skip_apisix = %s", values.Get("skip_apisix"))
	}
	if values.Get("host") != "" {
		t.Fatalf("host = %s", values.Get("host"))
	}
}

func TestReleaseHistoryCommand(t *testing.T) {
	endpoint, values := releaseCommand([]string{"history", "--service", "opspilot-core", "--limit", "5"})
	if endpoint != "/api/release/history" {
		t.Fatalf("endpoint = %s", endpoint)
	}
	if values.Get("service") != "opspilot-core" || values.Get("limit") != "5" {
		t.Fatalf("values = %#v", values)
	}
}

func TestReleaseRollbackRequiresConfirm(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{"release", "rollback", "--service", "opspilot-core", "--to", "abc123"}, &out)
	if err == nil {
		t.Fatal("expected rollback without --confirm to fail")
	}
	if err.Error() != "release rollback requires --confirm" {
		t.Fatalf("err = %v", err)
	}
}

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
	for _, name := range []string{"namespace.yaml", "deployment.yaml", "service.yaml", "kustomization.yaml"} {
		if err := os.WriteFile(filepath.Join("deploy", "k8s", name), []byte("ok\n"), 0o644); err != nil {
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
		filepath.Join("deploy", "k8s", "deployment.yaml"),
		filepath.Join("deploy", "k8s", "service.yaml"),
		filepath.Join("deploy", "k8s", "kustomization.yaml"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing generated file %s: %v", path, err)
		}
	}
	deployment, err := os.ReadFile(filepath.Join("deploy", "k8s", "deployment.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(deployment, []byte("imagePullSecrets")) {
		t.Fatalf("generated deployment should rely on node/containerd registry auth, not imagePullSecrets: %s", string(deployment))
	}
}

func TestRepoPreflightDetectsMissingReleaseFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := run([]string{"repo", "preflight", "--repo", dir, "--project", "tpo/devex/demo/demo-api"}, &out)
	if err == nil {
		t.Fatal("expected repo preflight to fail")
	}
	var payload repoPreflightResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Ready || !payload.Autofixable {
		t.Fatalf("payload = %#v", payload)
	}
	if !containsString(payload.Gaps, "dockerfile") || !containsString(payload.Gaps, "gitlab_ci") {
		t.Fatalf("expected dockerfile and gitlab_ci gaps: %#v", payload.Gaps)
	}
}

func TestRepoAutofixWritesPlatformFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"repo", "autofix", "--repo", dir, "--project", "tpo/devex/demo/demo-api", "--write"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"opspilot.service.yaml",
		"Dockerfile",
		".gitlab-ci.yml",
		filepath.Join("deploy", "k8s", "namespace.yaml"),
		filepath.Join("deploy", "k8s", "deployment.yaml"),
		filepath.Join("deploy", "k8s", "service.yaml"),
		filepath.Join("deploy", "k8s", "kustomization.yaml"),
	} {
		if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
			t.Fatalf("missing generated file %s: %v", path, err)
		}
	}
	out.Reset()
	if err := run([]string{"repo", "preflight", "--repo", dir, "--project", "tpo/devex/demo/demo-api"}, &out); err != nil {
		t.Fatalf("preflight after autofix failed: %v\n%s", err, out.String())
	}
}

func TestRepoAutofixForceReplacesRiskyDockerfile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM alpine:latest\nRUN curl http://localhost/install.sh | sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"repo", "autofix", "--repo", dir, "--project", "tpo/devex/demo/demo-api", "--write", "--force"}, &out); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("localhost")) || bytes.Contains(body, []byte(":latest")) {
		t.Fatalf("risky Dockerfile was not replaced: %s", string(body))
	}
}
