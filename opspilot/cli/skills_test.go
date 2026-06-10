package main

import (
	"bytes"
	"encoding/json"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/skillregistry"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSkillsRegistryUsesBackendOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/skills/registry" {
			http.NotFound(w, r)
			return
		}
		writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
			"version":          "test",
			"source":           "gitlab",
			"source_path":      "/opt/opspilot/skills/current/skills",
			"source_version":   "abc123",
			"item_count":       1,
			"integrated_count": 1,
			"dynamic_count":    1,
			"items": []map[string]any{{
				"name":             "kubernetes-specialist",
				"label":            "Kubernetes Specialist",
				"category":         "kubernetes",
				"integration_tier": "core",
				"integrated":       true,
				"commands":         []string{"inspect pod"},
			}},
		}})
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "--output", "json", "skills", "registry", "--integrated-only"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload skillsRegistryResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Source != "gitlab" || payload.IntegratedCount != 1 || !hasSkillName(payload.Items, "kubernetes-specialist") {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestSkillsRegistryDoesNotFallbackToClientEmbeddedRegistry(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{"--backend-url", "http://127.0.0.1:1", "--output", "json", "skills", "registry"}, &out)
	if err == nil {
		t.Fatal("expected backend-only skills registry query to fail")
	}
	if bytes.Contains(out.Bytes(), []byte("kubernetes-specialist")) {
		t.Fatalf("unexpected client embedded registry fallback: %s", out.String())
	}
}

func TestSkillsValidateUsesBackendByDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/skills/validate" {
			http.NotFound(w, r)
			return
		}
		writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
			"ready":       true,
			"root":        "/opt/opspilot/skills/current/skills",
			"skill_count": 1,
			"error_count": 0,
			"warn_count":  0,
			"skill_names": []string{"opspilot-ops"},
		}})
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "--output", "json", "skills", "validate"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload skillregistry.ValidationResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Ready || payload.Root != "/opt/opspilot/skills/current/skills" || payload.SkillCount != 1 {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestSkillsSourcesCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/skills/sources" {
			http.NotFound(w, r)
			return
		}
		writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
			"ready":             true,
			"root":              "/opt/opspilot/skills/current",
			"skills_count":      15,
			"candidate_count":   1,
			"unsupported_count": 1,
			"upstream_count":    1,
			"sources": []map[string]any{{
				"name":   "garrytan/gstack",
				"status": "mirrored",
				"reason": "test source",
			}},
		}})
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "--output", "human", "skills", "sources"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "garrytan/gstack") || !strings.Contains(out.String(), "candidates=1") {
		t.Fatalf("out = %s", out.String())
	}
}

func TestSkillsCandidatesCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/skills/candidates" {
			http.NotFound(w, r)
			return
		}
		writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
			"ready":             true,
			"root":              "/opt/opspilot/skills/current",
			"candidate_count":   1,
			"unsupported_count": 1,
			"candidates": []map[string]any{{
				"name":     "gstack-health",
				"status":   "candidate",
				"category": "platform",
				"source":   "garrytan/gstack",
				"reason":   "test candidate",
			}},
			"unsupported": []map[string]any{{
				"name":   "gstack-browse",
				"status": "unsupported",
				"source": "garrytan/gstack",
				"reason": "browser runtime",
			}},
		}})
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "--output", "human", "skills", "candidates"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "gstack-health") || !strings.Contains(out.String(), "gstack-browse") {
		t.Fatalf("out = %s", out.String())
	}
}

func TestSkillsImportPlanCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/skills/import-plan" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("name") != "gstack-health" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
			"ready":        true,
			"dry_run":      true,
			"name":         "gstack-health",
			"status":       "candidate_plan",
			"source":       "garrytan/gstack",
			"category":     "platform",
			"runtime_path": "skills/gstack-health",
			"files": []map[string]any{{
				"path":   "skills/gstack-health/skill.yaml",
				"body":   "name: gstack-health\nintegrated: false\n",
				"exists": false,
			}},
			"next": []string{"review", "commit"},
		}})
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "--output", "human", "skills", "import-plan", "--name", "gstack-health"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "candidate_plan") || !strings.Contains(out.String(), "skills/gstack-health/skill.yaml") {
		t.Fatalf("out = %s", out.String())
	}
}

func TestSkillsPromoteDryRunCommand(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/skills/import-plan" {
			http.NotFound(w, r)
			return
		}
		called = true
		writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
			"ready":   true,
			"dry_run": true,
			"name":    "gstack-health",
			"status":  "candidate_plan",
		}})
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "skills", "promote", "--name", "gstack-health", "--dry-run"}, &out); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("import plan endpoint was not called")
	}
}

func TestSkillsPromoteRejectsWriteMode(t *testing.T) {
	var out bytes.Buffer
	err := run([]string{"skills", "promote", "--name", "gstack-health", "--dry-run=false"}, &out)
	if err == nil || !strings.Contains(err.Error(), "dry-run only") {
		t.Fatalf("err = %v", err)
	}
}

func TestSkillsDiscoverAndReviewCommands(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/skills/discover":
			if r.URL.Query().Get("include_unsupported") != "true" {
				t.Fatalf("query = %s", r.URL.RawQuery)
			}
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"ready":               true,
				"root":                "/opt/opspilot/skills/current",
				"item_count":          2,
				"promotion_ready":     1,
				"blocked":             1,
				"confirmation_needed": true,
				"items": []map[string]any{
					{"name": "api-quality-check", "decision": "promotion_ready", "score": 95, "grade": "A", "category": "quality", "import_plan_ready": true, "reasons": []string{"quality mapping"}},
					{"name": "gstack-browse", "decision": "blocked", "score": 0, "grade": "F", "category": "browser", "blockers": []string{"browser runtime"}},
				},
			}})
		case "/api/skills/review":
			if r.URL.Query().Get("name") != "api-quality-check" {
				t.Fatalf("query = %s", r.URL.RawQuery)
			}
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"ready":               true,
				"root":                "/opt/opspilot/skills/current",
				"item_count":          1,
				"promotion_ready":     1,
				"confirmation_needed": true,
				"items": []map[string]any{
					{"name": "api-quality-check", "decision": "promotion_ready", "score": 95, "grade": "A", "category": "quality", "import_plan_ready": true, "reasons": []string{"quality mapping"}},
				},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "--output", "human", "skills", "discover", "--include-unsupported"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "api-quality-check") || !strings.Contains(out.String(), "gstack-browse") {
		t.Fatalf("out = %s", out.String())
	}
	out.Reset()
	if err := run([]string{"--backend-url", server.URL, "--output", "human", "skills", "review", "--name", "api-quality-check"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "promotion_ready") || !strings.Contains(out.String(), "confirmation_needed=true") {
		t.Fatalf("out = %s", out.String())
	}
}

func hasSkillName(items []skillregistry.Skill, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func TestCredentialsCatalogUsesBackend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/credentials/catalog" {
			http.NotFound(w, r)
			return
		}
		writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
			"version": "v1", "source": "env", "count": 1,
			"items": []any{map[string]any{"name": "opspilot-release-secrets", "class": "platform-runtime"}},
		}})
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "credentials", "catalog"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "opspilot-release-secrets") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestClustersCatalogUsesBackend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/clusters/catalog" {
			http.NotFound(w, r)
			return
		}
		writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
			"version": "v1", "source": "env", "count": 1,
			"items": []any{map[string]any{"name": "node200-test", "environment": "test"}},
		}})
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "clusters", "catalog"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "node200-test") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestCredentialPlanUsesBackend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/credentials/plan" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("kind") != "mysql" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
			"type": "credential", "kind": "mysql", "name": "demo-api-mysql-credentials",
			"risk": "controlled_mutate", "automation": "plan_first",
			"required_keys": []any{"DATABASE_URL"}, "steps": []any{"create secret"},
		}})
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "--output", "human", "credentials", "plan", "--kind", "mysql", "--service", "demo-api"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "demo-api-mysql-credentials") || !strings.Contains(out.String(), "DATABASE_URL") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestDatasourcePlanUsesBackend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/datasources/plan" {
			http.NotFound(w, r)
			return
		}
		writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
			"type": "datasource", "kind": "prometheus", "name": "node200-k8s",
			"cluster": "node200-test", "risk": "controlled_mutate", "automation": "plan_first",
			"required_keys": []any{"PROMETHEUS_URL"}, "steps": []any{"configure endpoint"},
		}})
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "--output", "human", "datasources", "plan", "--kind", "prometheus", "--name", "node200-k8s"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "node200-k8s") || !strings.Contains(out.String(), "PROMETHEUS_URL") {
		t.Fatalf("output = %s", out.String())
	}
}
