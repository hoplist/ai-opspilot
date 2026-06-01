package quality

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseYAMLQualityConfig(t *testing.T) {
	cfg, err := ParseYAML(`quality:
  enabled: true
  optional: true
  baseURL: http://demo.cicd.svc.cluster.local:8080
  smoke:
    timeoutSeconds: 5
    latencyP95Ms: 900
    endpoints:
      - name: health
        method: GET
        path: /health
        expectStatus: 200
`)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Enabled || !cfg.Optional || cfg.BaseURL == "" || cfg.Smoke.TimeoutSeconds != 5 || cfg.Smoke.LatencyP95Ms != 900 {
		t.Fatalf("cfg = %#v", cfg)
	}
	if len(cfg.Smoke.Endpoints) != 1 || cfg.Smoke.Endpoints[0].Name != "health" || cfg.Smoke.Endpoints[0].Path != "/health" {
		t.Fatalf("endpoints = %#v", cfg.Smoke.Endpoints)
	}
}

func TestRunQualityPassesHTTPCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.Smoke.Endpoints = []Endpoint{{Name: "health", Path: "/health", ExpectStatus: 200}}
	report := Run(context.Background(), cfg, "", server.Client())
	if report.Status != "passed" || report.CheckCount != 1 || report.PassedCount != 1 || report.FailedCount != 0 {
		t.Fatalf("report = %#v", report)
	}
}

func TestRunQualityFailsUnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.Smoke.Endpoints = []Endpoint{{Name: "health", Path: "/health", ExpectStatus: 200}}
	report := Run(context.Background(), cfg, "", server.Client())
	if report.Status != "failed" || report.FailedCount != 1 || report.Checks[0].StatusCode != 500 {
		t.Fatalf("report = %#v", report)
	}
}

func TestRunQualityDisabledSkips(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	report := Run(context.Background(), cfg, "", nil)
	if report.Status != "skipped" || report.SkippedReason != "quality_config_disabled" {
		t.Fatalf("report = %#v", report)
	}
}
