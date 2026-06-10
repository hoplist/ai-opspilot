package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

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
