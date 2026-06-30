package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	foundQualityWarning := false
	for _, item := range payload.Items {
		if item.Name == "quality_config" && item.Status == "warn" && item.Level == "warning" {
			foundQualityWarning = true
		}
	}
	if !foundQualityWarning {
		t.Fatalf("expected optional quality warning: %#v", payload.Items)
	}
}

func TestRepoUploadPlanDefaultsToSandbox(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/my-demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"repo", "upload-plan", "--repo", dir, "--name", "My Demo API"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload repoUploadPlanResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Mode != "plan" || !payload.Ready {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Target.GitLabProject != "tpo/sandbox/devex/my-demo-api" {
		t.Fatalf("target project = %s", payload.Target.GitLabProject)
	}
	if payload.Runtime.Namespace != "sandbox" || payload.Runtime.GitOpsPath != "clusters/test/apps/sandbox/my-demo-api" {
		t.Fatalf("runtime = %#v", payload.Runtime)
	}
	if payload.Runtime.ReleaseScope != "test-only" || payload.Language != "go" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestRepoPreflightSupportsExplicitMonorepoPaths(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "opspilot")
	deploy := filepath.Join(root, "deploy", "opspilot", "core")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(deploy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "go.mod"), []byte("module example.com/opspilot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Dockerfile"), []byte("FROM alpine:3.20\nEXPOSE 18080\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitlab-ci.yml"), []byte("buildctl-daemonless.sh build\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"namespace.yaml", "limitrange.yaml", "resourcequota.yaml", "serviceaccount.yaml", "service.yaml", "kustomization.yaml"} {
		if err := os.WriteFile(filepath.Join(deploy, name), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: placeholder\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	kustomization := `resources:
  - namespace.yaml
  - limitrange.yaml
  - resourcequota.yaml
  - serviceaccount.yaml
  - deployment.yaml
  - service.yaml
`
	if err := os.WriteFile(filepath.Join(deploy, "kustomization.yaml"), []byte(kustomization), 0o644); err != nil {
		t.Fatal(err)
	}
	deployment := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: opspilot
  namespace: cicd-devex-opspilot
spec:
  template:
    spec:
      containers:
        - name: opspilot
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 500m
              memory: 256Mi
          readinessProbe:
            httpGet:
              path: /health
              port: http
          livenessProbe:
            httpGet:
              path: /health
              port: http
`
	if err := os.WriteFile(filepath.Join(deploy, "deployment.yaml"), []byte(deployment), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{
		"repo", "preflight",
		"--repo", app,
		"--project", "platform/opspilot",
		"--ci-path", filepath.Join("..", ".gitlab-ci.yml"),
		"--deploy-path", filepath.Join("..", "deploy", "opspilot", "core"),
		"--namespace", "cicd-devex-opspilot",
		"--namespace-path", filepath.Join("..", "deploy", "opspilot", "core", "namespace.yaml"),
		"--limitrange-path", filepath.Join("..", "deploy", "opspilot", "core", "limitrange.yaml"),
		"--resourcequota-path", filepath.Join("..", "deploy", "opspilot", "core", "resourcequota.yaml"),
		"--serviceaccount-path", filepath.Join("..", "deploy", "opspilot", "core", "serviceaccount.yaml"),
		"--quality-path", filepath.Join("..", ".opspilot", "quality.yaml"),
	}, &out); err != nil {
		t.Fatalf("preflight with explicit monorepo paths failed: %v\n%s", err, out.String())
	}
	var payload repoPreflightResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, gap := range []string{"namespace", "deployment", "service", "kustomization"} {
		if containsString(payload.Gaps, gap) {
			t.Fatalf("did not expect %s gap with explicit paths: %#v", gap, payload.Gaps)
		}
	}
	foundCIPass := false
	for _, item := range payload.Items {
		if item.Name == "gitlab_ci" && item.Status == "pass" && item.Path == filepath.Join("..", ".gitlab-ci.yml") {
			foundCIPass = true
		}
	}
	if !foundCIPass {
		t.Fatalf("expected platform CI pass with explicit path: %#v", payload.Items)
	}
}

func TestRepoGovernanceAcceptsRecommendedAppPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "deploy", "k8s"), 0o755); err != nil {
		t.Fatal(err)
	}
	deployment := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo-api
  namespace: cicd-devex-demo
spec:
  template:
    spec:
      containers:
        - name: demo-api
          image: docker-hub.tpo.xzoa.com/devex/demo-api:abc1234
`
	if err := os.WriteFile(filepath.Join(dir, "deploy", "k8s", "deployment.yaml"), []byte(deployment), 0o644); err != nil {
		t.Fatal(err)
	}
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
	cfg := onboardServiceConfig{GitLabProject: "tpo/apps/devex/demo/demo-api"}
	items := checkRepoGovernance(cfg, repoLayoutOptions{}.defaults())
	if item := findRepoPolicyItem(items, "repo_class"); item.Status != "pass" || !strings.Contains(item.Message, "app") {
		t.Fatalf("repo_class item = %#v", item)
	}
	if item := findRepoPolicyItem(items, "immutable_image_tag"); item.Status != "pass" {
		t.Fatalf("immutable image item = %#v", item)
	}
}

func TestRepoGovernanceWarnsLegacyPathWithoutBlocking(t *testing.T) {
	items := checkRepoGovernance(onboardServiceConfig{GitLabProject: "tpo/devex/demo/demo-api"}, repoLayoutOptions{}.defaults())
	item := findRepoPolicyItem(items, "repo_class")
	if item.Status != "warn" || item.Level != "warning" || !strings.Contains(item.Message, "legacy") {
		t.Fatalf("repo_class item = %#v", item)
	}
}

func TestRepoGovernanceBlocksLatestImageForAppRepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "deploy", "k8s"), 0o755); err != nil {
		t.Fatal(err)
	}
	deployment := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo-api
spec:
  template:
    spec:
      containers:
        - name: demo-api
          image: docker-hub.tpo.xzoa.com/devex/demo-api:latest
`
	if err := os.WriteFile(filepath.Join(dir, "deploy", "k8s", "deployment.yaml"), []byte(deployment), 0o644); err != nil {
		t.Fatal(err)
	}
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
	items := checkRepoGovernance(onboardServiceConfig{GitLabProject: "tpo/apps/devex/demo/demo-api"}, repoLayoutOptions{}.defaults())
	item := findRepoPolicyItem(items, "immutable_image_tag")
	if item.Status != "fail" || item.Level != "blocker" || !strings.Contains(item.Message, ":latest") {
		t.Fatalf("immutable image item = %#v", item)
	}
}

func TestCodePrecheckIgnoresHTTPQueryHelperLoops(t *testing.T) {
	items := scanCodePrecheckText("core/http.go", `package main

func queryList(r *http.Request, name string) []string {
	values := []string{}
	for _, raw := range r.URL.Query()[name] {
		for _, part := range strings.FieldsFunc(raw, func(ch rune) bool {
			return ch == ',' || ch == '|'
		}) {
			values = append(values, part)
		}
	}
	return values
}
`)
	for _, item := range items {
		if item.ID == "possible_n_plus_one" {
			t.Fatalf("unexpected possible_n_plus_one finding: %#v", items)
		}
	}
}

func findRepoPolicyItem(items []repoPolicyItem, name string) repoPolicyItem {
	for _, item := range items {
		if item.Name == name {
			return item
		}
	}
	return repoPolicyItem{}
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
		filepath.Join("deploy", "k8s", "limitrange.yaml"),
		filepath.Join("deploy", "k8s", "resourcequota.yaml"),
		filepath.Join("deploy", "k8s", "serviceaccount.yaml"),
		filepath.Join("deploy", "k8s", "deployment.yaml"),
		filepath.Join("deploy", "k8s", "service.yaml"),
		filepath.Join("deploy", "k8s", "kustomization.yaml"),
		filepath.Join(".opspilot", "quality.yaml"),
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

func TestRepoPreflightBlocksUnreferencedKustomizeManifest(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"repo", "autofix", "--repo", dir, "--project", "tpo/devex/demo/demo-api", "--write"}, &out); err != nil {
		t.Fatal(err)
	}
	kustomizationPath := filepath.Join(dir, "deploy", "k8s", "kustomization.yaml")
	body, err := os.ReadFile(kustomizationPath)
	if err != nil {
		t.Fatal(err)
	}
	body = bytes.ReplaceAll(body, []byte("  - serviceaccount.yaml\n"), nil)
	if err := os.WriteFile(kustomizationPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	err = run([]string{"repo", "preflight", "--repo", dir, "--project", "tpo/devex/demo/demo-api"}, &out)
	if err == nil {
		t.Fatal("expected preflight to fail when serviceaccount.yaml is not referenced")
	}
	var payload repoPreflightResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	item := findRepoPolicyItem(payload.Items, "kustomization_references")
	if item.Status != "fail" || item.Level != "blocker" || !strings.Contains(item.Message, "serviceaccount.yaml") {
		t.Fatalf("kustomization reference item = %#v\n%s", item, out.String())
	}
	if !containsString(payload.Gaps, "kustomization_references") {
		t.Fatalf("expected kustomization_references gap: %#v", payload.Gaps)
	}
}

func TestRepoPreflightAllowsKustomizeDirectoryReference(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "app")
	deploy := filepath.Join(dir, "deploy", "core")
	rbac := filepath.Join(dir, "deploy", "rbac")
	for _, path := range []string{app, deploy, rbac} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(app, "go.mod"), []byte("module example.com/opspilot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Dockerfile"), []byte("FROM alpine:3.20\nEXPOSE 18080\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitlab-ci.yml"), []byte("buildctl-daemonless.sh build\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"namespace.yaml", "limitrange.yaml", "resourcequota.yaml", "serviceaccount.yaml"} {
		if err := os.WriteFile(filepath.Join(rbac, name), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: placeholder\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(rbac, "kustomization.yaml"), []byte("resources:\n  - namespace.yaml\n  - limitrange.yaml\n  - resourcequota.yaml\n  - serviceaccount.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deployment := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: opspilot
  namespace: opspilot
spec:
  template:
    spec:
      containers:
        - name: opspilot
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 500m
              memory: 256Mi
          readinessProbe:
            httpGet:
              path: /health
              port: http
          livenessProbe:
            httpGet:
              path: /health
              port: http
`
	if err := os.WriteFile(filepath.Join(deploy, "deployment.yaml"), []byte(deployment), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deploy, "service.yaml"), []byte("apiVersion: v1\nkind: Service\nmetadata:\n  name: opspilot\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deploy, "kustomization.yaml"), []byte("resources:\n  - ../rbac\n  - deployment.yaml\n  - service.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{
		"repo", "preflight",
		"--repo", app,
		"--project", "platform/opspilot",
		"--ci-path", filepath.Join("..", ".gitlab-ci.yml"),
		"--deploy-path", filepath.Join("..", "deploy", "core"),
		"--namespace", "opspilot",
		"--namespace-path", filepath.Join("..", "deploy", "rbac", "namespace.yaml"),
		"--limitrange-path", filepath.Join("..", "deploy", "rbac", "limitrange.yaml"),
		"--resourcequota-path", filepath.Join("..", "deploy", "rbac", "resourcequota.yaml"),
		"--serviceaccount-path", filepath.Join("..", "deploy", "rbac", "serviceaccount.yaml"),
	}, &out); err != nil {
		t.Fatalf("preflight should accept kustomize directory references: %v\n%s", err, out.String())
	}
	var payload repoPreflightResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if item := findRepoPolicyItem(payload.Items, "kustomization_references"); item.Status != "pass" {
		t.Fatalf("kustomization references item = %#v", item)
	}
}

func TestRepoPreflightAllowsPlatformStorage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := "LOG_DIR=/var/log/demo-api\nCACHE_DIR=/tmp/cache\n"
	if err := os.WriteFile(filepath.Join(dir, ".env.example"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"repo", "autofix", "--repo", dir, "--project", "tpo/devex/demo/demo-api", "--write"}, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := run([]string{"repo", "preflight", "--repo", dir, "--project", "tpo/devex/demo/demo-api"}, &out); err != nil {
		t.Fatalf("preflight with platform storage failed: %v\n%s", err, out.String())
	}
	var payload repoPreflightResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	foundStorage := false
	for _, item := range payload.Items {
		if item.Name == "storage_logs" && item.Status == "pass" && strings.Contains(item.Message, defaultHostPathRoot) {
			foundStorage = true
		}
	}
	if !foundStorage {
		t.Fatalf("storage item missing: %#v", payload.Items)
	}
}

func TestRepoPreflightBlocksRawHostPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"repo", "autofix", "--repo", dir, "--project", "tpo/devex/demo/demo-api", "--write"}, &out); err != nil {
		t.Fatal(err)
	}
	rawDeployment := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo-api
  namespace: cicd-devex-demo
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: demo-api
  template:
    metadata:
      labels:
        app.kubernetes.io/name: demo-api
    spec:
      containers:
        - name: demo-api
          image: placeholder
          ports:
            - name: http
              containerPort: 8080
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 500m
              memory: 256Mi
          readinessProbe:
            httpGet:
              path: /healthz
              port: http
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
          volumeMounts:
            - name: raw-logs
              mountPath: /app/logs
      volumes:
        - name: raw-logs
          hostPath:
            path: /data/logs/demo-api
            type: DirectoryOrCreate
`
	if err := os.WriteFile(filepath.Join(dir, "deploy", "k8s", "deployment.yaml"), []byte(rawDeployment), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	err := run([]string{"repo", "preflight", "--repo", dir, "--project", "tpo/devex/demo/demo-api"}, &out)
	if err == nil {
		t.Fatal("expected raw hostPath to fail preflight")
	}
	var payload repoPreflightResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !containsString(payload.Gaps, "deployment") || !bytes.Contains(out.Bytes(), []byte("outside /data/opspilot/hostpath")) {
		t.Fatalf("expected hostPath policy failure: %s", out.String())
	}
}

func TestRepoPrecheckWarnOnlyDoesNotFail(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `package main

func users() string {
	return "SELECT * FROM users"
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"repo", "precheck", "--repo", dir, "--project", "tpo/devex/demo/demo-api"}, &out); err != nil {
		t.Fatalf("warning-only precheck should not fail: %v\n%s", err, out.String())
	}
	var payload codePrecheckResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "warn" || !payload.Ready || payload.Summary.Warnings == 0 || payload.Summary.Blockers != 0 {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Items[0].Skill != "database-optimizer" {
		t.Fatalf("skill = %s", payload.Items[0].Skill)
	}
}

func TestRepoPrecheckBlocksDangerousCode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `package main

func wipe() string {
	return "DELETE FROM users"
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := run([]string{"repo", "precheck", "--repo", dir, "--project", "tpo/devex/demo/demo-api"}, &out)
	if err == nil {
		t.Fatal("expected dangerous precheck to fail")
	}
	var payload codePrecheckResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "blocker" || payload.Ready || payload.Summary.Blockers == 0 {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Items[0].ID != "db_unguarded_write" {
		t.Fatalf("item = %#v", payload.Items[0])
	}
}

func TestRepoPrecheckBlocksVueRuntimeTemplateWithoutCompiler(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"vue":"^3.5.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `import { createApp } from "vue";

const App = {
  template: "<main>blank risk</main>",
};

createApp(App).mount("#app");
`
	if err := os.WriteFile(filepath.Join(src, "main.js"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := run([]string{"repo", "precheck", "--repo", dir, "--project", "tpo/devex/demo/demo-web"}, &out)
	if err == nil {
		t.Fatal("expected Vue runtime template precheck to fail")
	}
	var payload codePrecheckResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Policy.Mode != "automatic_quality_gate" || payload.Policy.HumanApprovalRequired {
		t.Fatalf("policy = %#v", payload.Policy)
	}
	if payload.Status != "blocker" || payload.Ready || payload.Summary.Blockers == 0 {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Items[0].ID != "vue_runtime_template_without_compiler" {
		t.Fatalf("item = %#v", payload.Items[0])
	}
	if payload.Items[0].Decision != "block_release" || len(payload.Items[0].FixOptions) == 0 {
		t.Fatalf("expected AI-readable fix options: %#v", payload.Items[0])
	}
}

func TestRepoPrecheckWritesEvidence(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := `package main

const apiToken = "0123456789abcdef"
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := run([]string{"repo", "precheck", "--repo", dir, "--project", "tpo/devex/demo/demo-api", "--write", "--warn-only"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".opspilot", "evidence", "code-precheck.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var payload codePrecheckResult
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.EvidencePath == "" || payload.Status != "blocker" || payload.Items[0].ID != "secret_leak" {
		t.Fatalf("payload = %#v", payload)
	}
	if !bytes.Contains(out.Bytes(), []byte("code-precheck.json")) {
		t.Fatalf("expected evidence path in output: %s", out.String())
	}
}

func TestRepoPrecheckSkipsCredentialCatalogMetadata(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(dir, "config", "credentials")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := `credentials:
  - name: opspilot-release-secrets
    storage: kubernetes-secret
    permissions:
      - read_gitlab
`
	if err := os.WriteFile(filepath.Join(configDir, "platform.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"repo", "precheck", "--repo", dir, "--project", "tpo/devex/demo/demo-api"}, &out); err != nil {
		t.Fatalf("expected credential metadata to pass: %v\n%s", err, out.String())
	}
}

func TestRepoPrecheckBlocksCredentialPasswordValue(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := `credentials:
  - name: demo-db
    password: "0123456789abcdef"
`
	if err := os.WriteFile(filepath.Join(dir, "credentials.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := run([]string{"repo", "precheck", "--repo", dir, "--project", "tpo/devex/demo/demo-api"}, &out)
	if err == nil {
		t.Fatal("expected hardcoded credential password to fail")
	}
	var payload codePrecheckResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "blocker" || payload.Items[0].ID != "secret_leak" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestRepoPrecheckSkipsGeneratedOpsPilotServiceConfig(t *testing.T) {
	dir := t.TempDir()
	config := `name: demo-api
middleware:
  mysql:
    secret: demo-api-mysql-conn
`
	if err := os.WriteFile(filepath.Join(dir, "opspilot.service.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"repo", "precheck", "--repo", dir, "--project", "tpo/devex/demo/demo-api"}, &out); err != nil {
		t.Fatalf("expected generated config to be skipped: %v\n%s", err, out.String())
	}
	var payload codePrecheckResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status == "blocker" {
		t.Fatalf("generated config should not trigger blocker: %#v", payload.Items)
	}
}

func TestRepoPreflightReportsMiddlewareIntent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/orders-api\nrequire github.com/go-sql-driver/mysql v1.8.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	_ = run([]string{"repo", "preflight", "--repo", dir, "--project", "tpo/devex/orders/orders-api"}, &out)
	var payload repoPreflightResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range payload.Items {
		if item.Name == "middleware_mysql" && item.Status == "pass" && bytes.Contains([]byte(item.Message), []byte("shared-database")) {
			found = true
		}
	}
	if !found {
		t.Fatalf("middleware item missing: %#v", payload.Items)
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
