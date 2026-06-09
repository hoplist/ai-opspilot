package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/skillregistry"
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

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--version"}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Fatal("version output is empty")
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

func TestErrorsRecentCommand(t *testing.T) {
	endpoint, values := errorsCommand([]string{
		"recent",
		"--source", "middleware",
		"--service", "orders-api",
		"--namespace", "cicd-devex-orders",
		"--limit", "5",
	})
	if endpoint != "/api/errors/recent" {
		t.Fatalf("endpoint = %s", endpoint)
	}
	if values.Get("source") != "middleware" || values.Get("service") != "orders-api" || values.Get("namespace") != "cicd-devex-orders" || values.Get("limit") != "5" {
		t.Fatalf("values = %#v", values)
	}
}

func TestCapabilitiesCommandHumanOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/capabilities" {
			http.NotFound(w, r)
			return
		}
		writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
			"ready": true,
			"capabilities": []any{
				map[string]any{"name": "kubernetes_api", "status": "ready", "available": true, "available_evidence": []any{"Pod 状态"}},
				map[string]any{"name": "prometheus_metrics", "status": "missing", "available": false, "missing_evidence": []any{"Prometheus 未接入"}},
			},
			"available_evidence": []any{"Pod 状态"},
			"missing_evidence":   []any{"Prometheus 未接入"},
		}})
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "--output", "human", "capabilities"}, &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !bytes.Contains([]byte(text), []byte("Capabilities: ready=true")) || !bytes.Contains([]byte(text), []byte("Missing evidence:")) {
		t.Fatalf("unexpected output:\n%s", text)
	}
}

func TestDoctorCommandEvidenceOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"version": "test-version",
			}})
		case "/api/capabilities":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"ready":              true,
				"available_evidence": []any{"Pod status"},
				"missing_evidence":   []any{"ELK missing"},
				"capabilities": []any{
					map[string]any{"name": "kubernetes_api", "status": "ready", "available": true, "available_evidence": []any{"Pod status"}},
				},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "--output", "evidence", "doctor"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload evidencePack
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Subject.Type != "opspilot" || payload.Status != "healthy" || !containsString(payload.MissingEvidence, "ELK missing") {
		t.Fatalf("payload = %#v", payload)
	}
}

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

func TestCLISchemaIncludesSkillsMirrorCommands(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "contracts", "cli-schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range [][]byte{
		[]byte(`"name": "skills sources"`),
		[]byte(`"name": "skills candidates"`),
		[]byte(`"name": "skills discover"`),
		[]byte(`"name": "skills review"`),
		[]byte(`"name": "skills import-plan"`),
		[]byte(`"name": "skills promote"`),
		[]byte("Does not write files or enable the skill"),
	} {
		if !bytes.Contains(body, expected) {
			t.Fatalf("cli schema missing %s", expected)
		}
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

func hasSkillName(items []skillregistry.Skill, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func TestCheckReleaseAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/release/status" {
			http.NotFound(w, r)
			return
		}
		writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
			"service":    "opspilot-core",
			"status":     "healthy",
			"stage":      "rollout",
			"namespace":  "opspilot",
			"deployment": "opspilot-core",
			"evidence":   map[string]any{},
		}})
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "--output", "human", "check", "release", "opspilot-core"}, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Release: opspilot-core")) {
		t.Fatalf("unexpected output: %s", out.String())
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

func TestReleaseServiceSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/release/status":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"service":     "demo-api",
				"environment": "test",
				"namespace":   "cicd-devex-demo",
				"deployment":  "demo-api",
				"status":      "healthy",
				"stage":       "rollout",
				"image":       "registry/demo-api:abc123",
				"evidence": map[string]any{
					"gitlab_pipeline": map[string]any{"status": "success", "id": 18, "ref": "main", "sha": "abc123"},
					"buildkit":        map[string]any{"status": "success"},
					"gitops":          map[string]any{"status": "matches_cluster", "desired_image": "registry/demo-api:abc123"},
					"argocd":          map[string]any{"sync_status": "Synced", "health_status": "Healthy"},
				},
				"gaps":        []any{},
				"next_checks": []any{},
			}})
		case "/api/release/jobs":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"service":    "demo-api",
				"item_count": 1,
				"items": []any{
					map[string]any{"id": 1, "stage": "build", "name": "build:image", "status": "success", "duration": 12.5},
				},
			}})
		case "/api/release/history":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"service":    "demo-api",
				"item_count": 1,
				"items": []any{
					map[string]any{"short_revision": "abc123", "tag": "abc123", "current": true, "message": "deploy demo"},
				},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "release", "service", "demo-api"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload releaseServiceResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Service != "demo-api" || payload.Status != "healthy" || payload.JobCount != 1 || payload.HistoryCount != 1 {
		t.Fatalf("payload = %#v", payload)
	}
	if !payload.TriggerSupported {
		t.Fatalf("release service should expose trigger support")
	}
}

func TestReleaseServiceTrigger(t *testing.T) {
	triggered := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/release/status":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"service": "demo-api", "environment": "test", "namespace": "cicd-devex-demo", "deployment": "demo-api", "status": "healthy", "stage": "rollout",
				"evidence": map[string]any{}, "gaps": []any{}, "next_checks": []any{},
			}})
		case "/api/release/jobs":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{"service": "demo-api", "item_count": 0, "items": []any{}}})
		case "/api/release/history":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{"service": "demo-api", "item_count": 0, "items": []any{}}})
		case "/api/release/trigger":
			triggered = true
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("service") != "demo-api" || r.Form.Get("ref") != "main" {
				t.Fatalf("form = %#v", r.Form)
			}
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"service": "demo-api",
				"status":  "submitted",
				"pipeline": map[string]any{
					"id": 7, "status": "pending", "ref": "main", "sha": "abc123",
				},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "release", "service", "demo-api", "--trigger"}, &out); err != nil {
		t.Fatal(err)
	}
	if !triggered {
		t.Fatal("trigger endpoint was not called")
	}
	var payload releaseServiceResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Triggered || intValue(mapValue(payload.Trigger, "pipeline")["id"]) != 7 {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestInspectServiceAggregatesPods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/capabilities":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"ready":              true,
				"available_evidence": []any{"Pod 状态", "当前容器日志"},
				"missing_evidence":   []any{"ELK/OpenSearch 未接入"},
				"capabilities":       []any{},
			}})
		case "/api/release/status":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"service":     "demo-api",
				"environment": "test",
				"namespace":   "cicd-devex-demo",
				"deployment":  "demo-api",
				"status":      "healthy",
				"stage":       "rollout",
				"image":       "registry/demo-api:abc123",
				"evidence": map[string]any{
					"pods": map[string]any{
						"item_count": 1,
						"items": []any{
							map[string]any{"namespace": "cicd-devex-demo", "name": "demo-api-abc", "status": "Ready", "ready": true, "restart_count": 0},
						},
					},
				},
				"gaps":        []any{},
				"next_checks": []any{},
			}})
		case "/api/context/pod":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"summary": map[string]any{
					"namespace":     "cicd-devex-demo",
					"name":          "demo-api-abc",
					"node":          "worker-1",
					"status":        "Ready",
					"ready":         true,
					"restart_count": 0,
					"containers": []any{map[string]any{
						"name":         "app",
						"spec_image":   "registry/demo-api:new",
						"status_image": "registry/demo-api:old",
						"image_id":     "registry/demo-api@sha256:abc",
					}},
				},
			}})
		case "/api/metrics/pod":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"cpu_cores":                0.02,
				"memory_working_set_bytes": 64 * 1024 * 1024,
				"restart_count":            0,
			}})
		case "/api/k8s/logs/pod":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{"text": "started\n"}})
		case "/api/logs/search":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{"total": 1, "item_count": 1, "items": []any{}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "inspect", "service", "demo-api"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload inspectServiceResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Service != "demo-api" || payload.PodCount != 1 || payload.RestartCount != 0 {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.TotalCPUCore != 0.02 || payload.TotalMemoryMiB != 64 {
		t.Fatalf("usage = cpu %.3f memory %.1f", payload.TotalCPUCore, payload.TotalMemoryMiB)
	}
	if len(payload.AvailableEvidence) == 0 || len(payload.MissingEvidence) == 0 {
		t.Fatalf("capability evidence missing: %#v", payload)
	}
	if len(payload.Pods) != 1 || payload.Pods[0].SpecImage != "registry/demo-api:new" || payload.Pods[0].StatusImage != "registry/demo-api:old" {
		t.Fatalf("image evidence missing: %#v", payload.Pods)
	}
	if payload.Pods[0].ImageID != "registry/demo-api@sha256:abc" {
		t.Fatalf("image id = %s", payload.Pods[0].ImageID)
	}
}

func TestInspectPodImageAndLogHumanHints(t *testing.T) {
	pod := inspectPodResult{
		Pod:         "demo-api-abc",
		Container:   "app",
		SpecImage:   "registry/demo-api:new",
		StatusImage: "registry/demo-api:old",
		ImageID:     "registry/demo-api@sha256:abc",
	}
	if got := imageTagHint(pod); got != "new" {
		t.Fatalf("image tag hint = %s", got)
	}
	var out bytes.Buffer
	writeImageEvidenceHuman(&out, pod)
	text := out.String()
	if !bytes.Contains([]byte(text), []byte("Spec image: registry/demo-api:new")) || !bytes.Contains([]byte(text), []byte("Image note:")) {
		t.Fatalf("image evidence output = %s", text)
	}
	findings := logEvidenceFindings(inspectPodResult{}, true, false)
	joined := strings.Join(findings, " ")
	if !strings.Contains(joined, "short-window logs are empty") || !strings.Contains(joined, "Pod-level checks remain usable") {
		t.Fatalf("log findings = %#v", findings)
	}
	serviceFindings := strings.Join(serviceLogEvidenceFindings([]string{"kubernetes_logs_unavailable", "elk_logs_unavailable"}), " ")
	if !strings.Contains(serviceFindings, "release evidence remain usable") || !strings.Contains(serviceFindings, "historical logs are incomplete") {
		t.Fatalf("service log findings = %s", serviceFindings)
	}
}

func TestInspectServiceEvidenceOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/capabilities":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"ready":              true,
				"available_evidence": []any{"Pod status", "Pod logs"},
				"missing_evidence":   []any{"APISIX missing"},
				"capabilities":       []any{},
			}})
		case "/api/release/status":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"service": "demo-api", "environment": "test", "namespace": "cicd-devex-demo", "deployment": "demo-api", "status": "healthy", "stage": "rollout",
				"evidence": map[string]any{
					"pods": map[string]any{"item_count": 1, "items": []any{
						map[string]any{"namespace": "cicd-devex-demo", "name": "demo-api-abc"},
					}},
				},
				"gaps": []any{}, "next_checks": []any{},
			}})
		case "/api/context/pod":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"summary": map[string]any{"node": "worker-1", "status": "Ready", "ready": true, "restart_count": 0},
			}})
		case "/api/metrics/pod":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"cpu_cores": 0.01, "memory_working_set_bytes": 8 * 1024 * 1024, "restart_count": 0,
			}})
		case "/api/k8s/logs/pod":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{"text": "ok\n"}})
		case "/api/logs/search":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{"total": 0, "item_count": 0}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "--output", "evidence", "check", "service", "demo-api"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload evidencePack
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Subject.Type != "service" || payload.Subject.Name != "demo-api" || len(payload.Evidence) == 0 {
		t.Fatalf("payload = %#v", payload)
	}
	if !containsString(payload.MissingEvidence, "APISIX missing") {
		t.Fatalf("missing evidence = %#v", payload.MissingEvidence)
	}
}

func TestFixServiceRequiresDryRunAndReturnsPlan(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/capabilities":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"ready":              true,
				"available_evidence": []any{"Pod status"},
				"missing_evidence":   []any{},
				"capabilities":       []any{},
			}})
		case "/api/release/status":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"service": "demo-api", "environment": "test", "namespace": "cicd-devex-demo", "deployment": "demo-api", "status": "degraded", "stage": "rollout",
				"evidence": map[string]any{
					"pods": map[string]any{"item_count": 1, "items": []any{
						map[string]any{"namespace": "cicd-devex-demo", "name": "demo-api-abc"},
					}},
				},
				"gaps": []any{}, "next_checks": []any{"inspect Pod logs"},
			}})
		case "/api/context/pod":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"summary": map[string]any{"node": "worker-1", "status": "CrashLoopBackOff", "ready": false, "restart_count": 3},
			}})
		case "/api/metrics/pod":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"cpu_cores": 0.01, "memory_working_set_bytes": 8 * 1024 * 1024, "restart_count": 3,
			}})
		case "/api/k8s/logs/pod":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{"text": "panic: config missing\n"}})
		case "/api/logs/search":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{"total": 0, "item_count": 0}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := run([]string{"--backend-url", server.URL, "fix", "service", "demo-api"}, &out)
	if err == nil || err.Error() != "fix service currently requires --dry-run" {
		t.Fatalf("err = %v", err)
	}

	out.Reset()
	if err := run([]string{"--backend-url", server.URL, "--output", "evidence", "fix", "service", "demo-api", "--dry-run"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload evidencePack
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Subject.Type != "service" || payload.Subject.Name != "demo-api" || len(payload.RecommendedActions) == 0 {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestNaturalLanguageDryRunRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			http.NotFound(w, r)
			return
		}
		writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
			"release": map[string]any{"configured": true, "services": []any{"opspilot-core"}},
		}})
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "ask", "发布 opspilot-core", "--dry-run"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload naturalLanguageResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Action != "release_service" || payload.Service != "opspilot-core" || !payload.DryRun || payload.Executed {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestNaturalLanguageInspectExecutes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"release": map[string]any{"configured": true, "services": []any{"opspilot-core"}},
			}})
		case "/api/release/status":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"service": "opspilot-core", "environment": "test", "namespace": "opspilot", "deployment": "opspilot-core", "status": "healthy", "stage": "rollout",
				"evidence": map[string]any{
					"pods": map[string]any{"item_count": 1, "items": []any{
						map[string]any{"namespace": "opspilot", "name": "opspilot-core-abc"},
					}},
				},
				"gaps": []any{}, "next_checks": []any{},
			}})
		case "/api/context/pod":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"summary": map[string]any{"node": "worker-1", "status": "Ready", "ready": true, "restart_count": 0},
			}})
		case "/api/metrics/pod":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
				"cpu_cores": 0.01, "memory_working_set_bytes": 8 * 1024 * 1024, "restart_count": 0,
			}})
		case "/api/k8s/logs/pod":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{"text": "ok\n"}})
		case "/api/logs/search":
			writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{"total": 1, "item_count": 1}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "ask", "检查 opspilot-core 是否正常"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload naturalLanguageResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Action != "inspect_service" || payload.Service != "opspilot-core" || !payload.Executed {
		t.Fatalf("payload = %#v", payload)
	}
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

func TestCLISchemaIncludesLifecyclePlanningCommands(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "contracts", "cli-schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range [][]byte{
		[]byte(`"name": "janitor plan"`),
		[]byte(`"name": "healer diagnose"`),
		[]byte(`"name": "app decommission plan"`),
		[]byte("high-risk actions are plan-only"),
	} {
		if !bytes.Contains(body, expected) {
			t.Fatalf("cli schema missing %s", expected)
		}
	}
	for _, args := range [][]string{
		{"janitor", "run"},
		{"healer", "fix"},
		{"app", "decommission", "run"},
	} {
		var out bytes.Buffer
		if err := run(args, &out); err == nil {
			t.Fatalf("expected %v to be disabled in v1", args)
		}
	}
}

func TestDecommissionPlanBlocksMutationsWhenMappingsAreMissing(t *testing.T) {
	release := releaseServiceResult{
		Service:    "fullstack-vue-web",
		Namespace:  "cicd-demo-fullstack",
		Deployment: "fullstack-vue-web",
		GitOps:     map[string]any{},
		ArgoCD:     map[string]any{},
	}
	inspection := inspectServiceResult{PodCount: 1}
	actions, blocked := decommissionActions(release, inspection, nil, true)

	for _, action := range actions {
		if action.Risk != "read_only" {
			t.Fatalf("missing mappings must not produce mutable action: %#v", action)
		}
	}
	blockedByID := map[string]lifecycleAction{}
	for _, action := range blocked {
		blockedByID[action.ID] = action
	}
	for _, id := range []string{"remove_gitops_application", "remove_gitops_workload_manifests", "delete_namespace"} {
		action, ok := blockedByID[id]
		if !ok {
			t.Fatalf("expected %s to be blocked: %#v", id, blocked)
		}
		if action.Risk != "high_risk" || action.Automation != "plan_only" {
			t.Fatalf("expected %s to be high-risk plan-only: %#v", id, action)
		}
	}
}

func TestDecommissionPlanAllowsGitOpsPlanOnlyWhenFullyMapped(t *testing.T) {
	release := releaseServiceResult{
		Service:    "opspilot-core",
		Namespace:  "opspilot",
		Deployment: "opspilot-core",
		GitOps:     map[string]any{"path": "clusters/test/apps/opspilot-core/deployment.yaml"},
		ArgoCD:     map[string]any{"app": "opspilot-core"},
	}
	inspection := inspectServiceResult{PodCount: 1}
	actions, blocked := decommissionActions(release, inspection, nil, true)

	allowedByID := map[string]lifecycleAction{}
	for _, action := range actions {
		allowedByID[action.ID] = action
	}
	for _, id := range []string{"remove_gitops_application", "remove_gitops_workload_manifests"} {
		action, ok := allowedByID[id]
		if !ok {
			t.Fatalf("expected %s to be allowed in the plan: %#v", id, actions)
		}
		if action.Risk != "controlled_mutate" || action.Automation != "confirm_allowed" {
			t.Fatalf("expected %s to be controlled and confirmation-gated: %#v", id, action)
		}
	}
	for _, action := range blocked {
		if action.ID == "delete_namespace" && action.Target != "opspilot" {
			t.Fatalf("unexpected namespace block target: %#v", action)
		}
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

func TestQualityRunCommand(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/quality/run" {
			http.NotFound(w, r)
			return
		}
		called = true
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("service") != "demo-api" || r.Form.Get("base_url") != "http://demo" {
			t.Fatalf("form = %#v", r.Form)
		}
		writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
			"service": "demo-api", "status": "submitted", "optional": true, "job_name": "demo-api-quality-1",
		}})
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "quality", "run", "service", "demo-api", "--base-url", "http://demo"}, &out); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("quality run endpoint was not called")
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["status"] != "submitted" || payload["service"] != "demo-api" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestQualityStatusCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/quality/status" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("service") != "demo-api" {
			t.Fatalf("query = %s", r.URL.RawQuery)
		}
		writeTestJSON(w, map[string]any{"ok": true, "data": map[string]any{
			"service":  "demo-api",
			"status":   "passed",
			"optional": true,
			"report": map[string]any{
				"status": "passed", "check_count": 1, "passed_count": 1, "failed_count": 0, "duration_ms": 10, "summary": "1/1 quality checks passed.",
			},
		}})
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run([]string{"--backend-url", server.URL, "--output", "human", "quality", "status", "service", "demo-api"}, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Quality status: service=demo-api status=passed")) || !bytes.Contains(out.Bytes(), []byte("Report: status=passed")) {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestQualityRunnerCommand(t *testing.T) {
	t.Setenv("OPSPILOT_QUALITY_CONFIG_JSON", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "quality.yaml")
	if err := os.WriteFile(configPath, []byte(`quality:
  enabled: true
  optional: true
  smoke:
    timeoutSeconds: 3
    latencyP95Ms: 1000
    endpoints:
      - name: health
        method: GET
        path: /health
        expectStatus: 200
`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"quality", "runner", "--config", configPath, "--base-url", server.URL}, &out); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["status"] != "passed" || intValue(payload["check_count"]) != 1 {
		t.Fatalf("payload = %#v", payload)
	}
}

func writeTestJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
