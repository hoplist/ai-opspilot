package k8s

import "testing"

func TestBoundedLogRequest(t *testing.T) {
	req := BoundedLogRequest(LogRequest{Namespace: "n", Pod: "p", TailLines: 99999, SinceSeconds: 999999, LimitBytes: 999999999})
	if req.TailLines != MaxTailLines {
		t.Fatalf("tail lines = %d", req.TailLines)
	}
	if req.SinceSeconds != MaxSinceSeconds {
		t.Fatalf("since seconds = %d", req.SinceSeconds)
	}
	if req.LimitBytes != MaxLimitBytes {
		t.Fatalf("limit bytes = %d", req.LimitBytes)
	}
}

func TestPodSummaryAbnormal(t *testing.T) {
	pod := map[string]any{
		"metadata": map[string]any{"namespace": "default", "name": "demo"},
		"spec": map[string]any{
			"nodeName": "node-1",
			"containers": []any{map[string]any{
				"name":  "app",
				"image": "registry.local/app:new",
			}},
		},
		"status": map[string]any{
			"phase":      "Running",
			"conditions": []any{map[string]any{"type": "Ready", "status": "False"}},
			"containerStatuses": []any{map[string]any{
				"name":         "app",
				"ready":        false,
				"restartCount": float64(2),
				"image":        "registry.local/app:old",
				"imageID":      "registry.local/app@sha256:abc",
				"state":        map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}},
			}},
		},
	}
	summary := PodSummary(pod)
	if summary["restart_count"].(int) != 2 {
		t.Fatalf("restart_count = %v", summary["restart_count"])
	}
	if !MatchesStatus(summary, "abnormal") {
		t.Fatal("expected abnormal match")
	}
	if !MatchesStatus(summary, "crashloop") {
		t.Fatal("expected crashloop match")
	}
	containers := summary["containers"].([]any)
	first := containers[0].(map[string]any)
	if first["image"] != "registry.local/app:new" {
		t.Fatalf("image should prefer spec image, got %v", first["image"])
	}
	if first["spec_image"] != "registry.local/app:new" || first["status_image"] != "registry.local/app:old" {
		t.Fatalf("unexpected image evidence: %#v", first)
	}
	if first["image_id"] != "registry.local/app@sha256:abc" {
		t.Fatalf("image_id = %v", first["image_id"])
	}
}

func TestSucceededPodIsNotAbnormal(t *testing.T) {
	pod := map[string]any{
		"metadata": map[string]any{"namespace": "default", "name": "done"},
		"status":   map[string]any{"phase": "Succeeded"},
	}
	summary := PodSummary(pod)
	if MatchesStatus(summary, "abnormal") {
		t.Fatal("succeeded pod should not be abnormal")
	}
}
