package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReleaseHistoryCommand(t *testing.T) {
	endpoint, values := releaseCommand([]string{"history", "--service", "opspilot-core", "--limit", "5"})
	if endpoint != "/api/release/history" {
		t.Fatalf("endpoint = %s", endpoint)
	}
	if values.Get("service") != "opspilot-core" || values.Get("limit") != "5" {
		t.Fatalf("values = %#v", values)
	}
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
