package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/configloader"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/errorevidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/evidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
)

func TestHTTPProbeEndpointReturnsEvidencePack(t *testing.T) {
	var gotProbeID string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProbeID = r.Header.Get("X-OpsPilot-Probe-Id")
		_, _ = w.Write([]byte("hello"))
	}))
	defer target.Close()

	mux := http.NewServeMux()
	registerRoutes(mux, testRuntimeState("", emptyConfig()), errorevidence.NewCollector(t.TempDir()), release.QualitySettings{}, nil, evidence.NewStore(t.TempDir()))
	body := "url=" + target.URL + "&include_response=true&persist=true"
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/probe/http", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if gotProbeID == "" {
		t.Fatal("probe id header was not sent")
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	data := payload["data"].(map[string]any)
	pack := data["evidence_pack"].(map[string]any)
	if pack["trigger"] != "http_probe" || pack["status"] != "healthy" {
		t.Fatalf("pack = %#v", pack)
	}
	policy := data["policy"].(map[string]any)
	if policy["name"] != "default-http-probe" {
		t.Fatalf("policy = %#v", policy)
	}
	if !strings.Contains(recorder.Body.String(), "logs: log search is not configured") {
		t.Fatalf("expected missing log warning, body=%s", recorder.Body.String())
	}
}

func emptyConfig() configloader.Config {
	return configloader.Config{Version: "v1", Source: "test", Valid: true}
}
