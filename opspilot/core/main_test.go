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

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/errorevidence"
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

func registerTestRoutes(t *testing.T, mux *http.ServeMux, services string) {
	t.Helper()
	registerRoutes(
		mux,
		k8s.NewRegistry(k8s.RegistryConfig{}),
		prom.NewRegistry("", "", ""),
		nodeagent.NewRegistry("", ""),
		logsearch.NewClientWithConfig("", "", logsearch.CorrelationConfig{}),
		release.NewRegistry(services),
		errorevidence.NewCollector(t.TempDir()),
		release.QualitySettings{},
	)
}
