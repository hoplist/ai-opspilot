package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/configloader"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/errorevidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/evidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/k8s"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/nodeagent"
	prom "github.com/dualistpeng-netizen/ai-observability/opspilot/internal/prometheus"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
)

func TestRequiredPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	required("", "namespace")
}

func TestWrapConvertsBadRequestPanic(t *testing.T) {
	handler := wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		required("", "namespace")
		return nil, nil, nil
	})
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/api/test", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "namespace is required") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestHandleAPIRegistersV1Alias(t *testing.T) {
	mux := http.NewServeMux()
	handleAPI(mux, "/api/example", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return map[string]string{"path": r.URL.Path}, nil, nil
	}))

	for _, path := range []string{"/api/example", "/api/v1/example"} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestSkillsRecommendEndpoint(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "devops-engineer")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), []byte(`name: devops-engineer
label: DevOps Engineer
category: release
integration_tier: core
integrated: true
priority: 65
commands: [release status, release jobs]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPSPILOT_SKILLS_DYNAMIC_ENABLED", "true")
	t.Setenv("OPSPILOT_SKILLS_DIR", dir)
	mux := http.NewServeMux()
	registerTestRoutes(t, mux, "demo-api=namespace:demo,deployment:demo-api")
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/skills/recommend?target_type=service&status=degraded&missing_evidence=elk_logs_missing&finding=restart", nil)
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(recorder.Body.String(), "devops-engineer") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestSkillsValidateEndpoint(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "opspilot-ops")
	if err := os.MkdirAll(filepath.Join(skillDir, "examples"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# OpsPilot Ops\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), []byte(`name: opspilot-ops
label: OpsPilot Ops
category: platform
integration_tier: core
integrated: true
priority: 100
summary: Server-side OpsPilot skill.
use_when:
  - inspect platform
evidence:
  - doctor
commands:
  - doctor
boundaries:
  - no arbitrary shell
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPSPILOT_SKILLS_DYNAMIC_ENABLED", "true")
	t.Setenv("OPSPILOT_SKILLS_DIR", dir)
	mux := http.NewServeMux()
	registerTestRoutes(t, mux, "demo-api=namespace:demo,deployment:demo-api")
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/skills/validate", nil)
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"ready":true`) || !strings.Contains(recorder.Body.String(), `"skill_count":1`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestSkillsSourcesEndpoint(t *testing.T) {
	root := t.TempDir()
	skillsRoot := filepath.Join(root, "skills")
	skillDir := filepath.Join(skillsRoot, "opspilot-ops")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), []byte(`name: opspilot-ops
category: platform
integrated: true
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "upstream", "garrytan-gstack"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "registry.yaml"), []byte(`version: test
sources:
  - name: garrytan/gstack
    status: mirrored
    reason: test source
skills:
  - name: gstack-health
    status: candidate
    source: garrytan/gstack
    reason: test candidate
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPSPILOT_SKILLS_DIR", skillsRoot)
	mux := http.NewServeMux()
	registerTestRoutes(t, mux, "")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/skills/sources", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "garrytan/gstack") || !strings.Contains(recorder.Body.String(), "gstack-health") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestSkillsImportPlanEndpoint(t *testing.T) {
	root := t.TempDir()
	skillsRoot := filepath.Join(root, "skills")
	if err := os.MkdirAll(skillsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "registry.yaml"), []byte(`version: test
sources:
  - name: garrytan/gstack
    status: mirrored
skills:
  - name: gstack-health
    status: candidate
    source: garrytan/gstack
    upstream_path: skills/health
    category: platform
    reason: test candidate
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPSPILOT_SKILLS_DIR", skillsRoot)
	mux := http.NewServeMux()
	registerTestRoutes(t, mux, "")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/skills/import-plan?name=gstack-health", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"status":"candidate_plan"`) || !strings.Contains(recorder.Body.String(), "skills/gstack-health/skill.yaml") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestSkillsDiscoverAndReviewEndpoints(t *testing.T) {
	root := t.TempDir()
	skillsRoot := filepath.Join(root, "skills")
	if err := os.MkdirAll(skillsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "registry.yaml"), []byte(`version: test
skills:
  - name: api-quality-check
    status: candidate
    source: opspilot-roadmap
    category: quality
    priority: 82
    reason: maps to quality run and quality status
  - name: gstack-browse
    status: unsupported
    source: garrytan/gstack
    category: browser
    reason: requires browser runtime
`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPSPILOT_SKILLS_DIR", skillsRoot)
	mux := http.NewServeMux()
	registerTestRoutes(t, mux, "")

	discover := httptest.NewRecorder()
	mux.ServeHTTP(discover, httptest.NewRequest(http.MethodGet, "/api/skills/discover?include_unsupported=true", nil))
	if discover.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", discover.Code, discover.Body.String())
	}
	if !strings.Contains(discover.Body.String(), `"promotion_ready":1`) || !strings.Contains(discover.Body.String(), `"blocked":1`) {
		t.Fatalf("body = %s", discover.Body.String())
	}

	review := httptest.NewRecorder()
	mux.ServeHTTP(review, httptest.NewRequest(http.MethodGet, "/api/skills/review?name=api-quality-check", nil))
	if review.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", review.Code, review.Body.String())
	}
	if !strings.Contains(review.Body.String(), `"decision":"promotion_ready"`) || !strings.Contains(review.Body.String(), `"import_plan_ready":true`) {
		t.Fatalf("body = %s", review.Body.String())
	}
}

func TestCredentialPlanEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	registerTestRoutes(t, mux, "")
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/credentials/plan?kind=mysql&service=demo-api", nil)
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "DATABASE_URL") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestServicesCatalogEndpoint(t *testing.T) {
	t.Setenv("OPSPILOT_SERVICE_CATALOG", "opspilot-core=repo:tpo/platform/opspilot/opspilot-core,owner:platform,namespace:opspilot,deployment:opspilot-core,middleware:mysql,config:apollo")
	mux := http.NewServeMux()
	registerTestRoutes(t, mux, "opspilot-core=namespace:opspilot,deployment:opspilot-core,gitlab:tpo/platform/opspilot/opspilot-core")
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/services/catalog", nil)
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"release_mapped":true`) || !strings.Contains(recorder.Body.String(), "apollo") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestConfigStatusEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	state := testRuntimeState("", configloader.Config{
		Version: "v1",
		Source:  "file",
		Valid:   true,
		Services: []configloader.Service{{
			Name: "todo-server",
		}},
	})
	registerRoutes(mux, state, errorevidence.NewCollector(t.TempDir()), release.QualitySettings{}, nil, evidence.NewStore(t.TempDir()))
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/config/status", nil)
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"services":1`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestLogsRouteEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	state := testRuntimeState("", configloader.Config{
		Version: "v1",
		Source:  "test",
		Valid:   true,
		Services: []configloader.Service{{
			Name:    "todo-server",
			Domains: []string{"todo.tpo.xzoa.com"},
			Runtime: configloader.RuntimeSpec{Cluster: "node200-test", Namespace: "todo", Deployment: "todo-server"},
			Logs:    configloader.ServiceLogSpec{AppIndexes: []string{"todo-*"}},
		}},
		Clusters: []configloader.Cluster{{Name: "node200-test", Logs: "node200-logs"}},
		Datasources: []configloader.Datasource{{
			Name: "node200-logs",
			Kind: "elasticsearch",
			URL:  "http://es.example:9200",
		}},
	})
	registerRoutes(mux, state, errorevidence.NewCollector(t.TempDir()), release.QualitySettings{}, nil, evidence.NewStore(t.TempDir()))
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/logs/route?host=todo.tpo.xzoa.com", nil)
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"selected"`) || !strings.Contains(recorder.Body.String(), "node200-logs") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestRepoUploadTargetEndpointCreatesProject(t *testing.T) {
	gitlab := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "server-token" {
			t.Fatalf("missing private token")
		}
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/tpo%2Fsandbox%2Fdevex%2Fdemo-api":
			http.NotFound(w, r)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/groups/tpo%2Fsandbox%2Fdevex":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 123})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects":
			base := "http://" + r.Host
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                  456,
				"path_with_namespace": "tpo/sandbox/devex/demo-api",
				"http_url_to_repo":    base + "/tpo/sandbox/devex/demo-api.git",
				"web_url":             base + "/tpo/sandbox/devex/demo-api",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
	}))
	defer gitlab.Close()
	t.Setenv("OPSPILOT_GITLAB_TOKEN", "server-token")
	t.Setenv("OPSPILOT_REPO_UPLOAD_ALLOWED_BASES", "tpo/sandbox/devex")

	mux := http.NewServeMux()
	state := testRuntimeState("", configloader.Config{
		Version:  "v1",
		Source:   "test",
		Valid:    true,
		Settings: configloader.Settings{GitLabURL: gitlab.URL},
	})
	registerRoutes(mux, state, errorevidence.NewCollector(t.TempDir()), release.QualitySettings{}, nil, evidence.NewStore(t.TempDir()))
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/repo/upload-target", strings.NewReader("target_project=tpo%2Fsandbox%2Fdevex%2Fdemo-api"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"server_owned":true`) || !strings.Contains(recorder.Body.String(), `"action":"created"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestAuditPolicyEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	registerTestRoutes(t, mux, "")
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit/policy", nil)
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "high_risk") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func registerTestRoutes(t *testing.T, mux *http.ServeMux, services string) {
	t.Helper()
	registerRoutes(mux, testRuntimeState(services, configloader.Config{Version: "v1", Source: "test", Valid: true}), errorevidence.NewCollector(t.TempDir()), release.QualitySettings{}, nil, evidence.NewStore(t.TempDir()))
}

func testRuntimeState(services string, cfg configloader.Config) *runtimeState {
	return &runtimeState{
		config:          cfg,
		k8sRegistry:     k8s.NewRegistry(k8s.RegistryConfig{}),
		promRegistry:    prom.NewRegistry("", "", ""),
		agentRegistry:   nodeagent.NewRegistry("", ""),
		logClient:       logsearch.NewClientWithConfig("", "", logsearch.CorrelationConfig{}),
		releaseRegistry: release.NewRegistry(services),
	}
}
