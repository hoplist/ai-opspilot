package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
