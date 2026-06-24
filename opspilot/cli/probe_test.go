package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeHTTPCommandPostsForm(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/probe/http" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("url") != "http://example.test/api" {
			t.Fatalf("form = %#v", r.Form)
		}
		if got := r.Form["header"]; len(got) != 2 {
			t.Fatalf("headers = %#v", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{
				"probe": map[string]any{
					"probe_id":    "probe-test",
					"method":      "GET",
					"url":         "http://example.test/api",
					"status_code": 200,
					"duration_ms": 4,
				},
				"correlation": map[string]any{
					"investigation_mode": "no_evidence",
					"evidence_strength":  "missing",
					"gaps":               []string{"apisix_log_empty"},
				},
				"evidence_pack": map[string]any{
					"id":      "pack-test",
					"status":  "healthy",
					"trigger": "http_probe",
					"summary": "ok",
				},
			},
		})
	}))
	defer backend.Close()

	var out bytes.Buffer
	err := run([]string{
		"--backend-url", backend.URL,
		"--output", "human",
		"probe", "http",
		"--url", "http://example.test/api",
		"--header", "X-Test: one",
		"--header", "X-Other: two",
	}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Probe: id=probe-test") || !strings.Contains(out.String(), "Evidence Pack: id=pack-test") {
		t.Fatalf("output = %s", out.String())
	}
}
