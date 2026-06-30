package release

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewRegistryParsesServices(t *testing.T) {
	registry := NewRegistry("opspilot-core=namespace:opspilot,deployment:opspilot-core,container:core,source:node200-k8s,image:registry/app:tag,gitlab:tpo/platform/opspilot/opspilot-core,gitops:clusters/test/apps/opspilot-core/deployment.yaml,argocd:opspilot-core")
	if !registry.Configured() {
		t.Fatal("expected configured registry")
	}
	names := registry.Services()
	if len(names) != 1 || names[0] != "opspilot-core" {
		t.Fatalf("names = %#v", names)
	}
	service := registry.services["opspilot-core"]
	if service.Namespace != "opspilot" || service.Deployment != "opspilot-core" || service.Source != "node200-k8s" {
		t.Fatalf("service = %#v", service)
	}
	if service.Container != "core" || service.GitLab != "tpo/platform/opspilot/opspilot-core" || service.GitOps == "" || service.ArgoCD != "opspilot-core" {
		t.Fatalf("service release fields = %#v", service)
	}
}

func TestRegistryCanFallbackToServiceCatalog(t *testing.T) {
	registry := NewRegistryWithCatalog("", "demo=namespace:apps,deployment:demo,container:api,source:node200-k8s,image:registry/demo,gitlab:platform/demo,gitops:apps/demo/deployment.yaml,argocd:demo", Datasources{})
	if !registry.Configured() {
		t.Fatal("registry should be configured from service catalog")
	}
	items := registry.ServiceItems()
	if len(items) != 1 || items[0].Name != "demo" || items[0].GitOps == "" {
		t.Fatalf("items = %#v", items)
	}
}

func TestRegistryPrefersServiceCatalogOverLegacyEnv(t *testing.T) {
	registry := NewRegistryWithCatalog(
		"demo=namespace:legacy,deployment:legacy,gitlab:legacy/demo",
		"demo=namespace:apps,deployment:demo,container:api,source:node200-k8s,image:registry/demo,gitlab:tpo/apps/demo,gitops:apps/demo/deployment.yaml,argocd:demo",
		Datasources{},
	)
	items := registry.ServiceItems()
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].Namespace != "apps" || items[0].GitLab != "tpo/apps/demo" {
		t.Fatalf("service catalog should win over env mapping: %#v", items[0])
	}
}

func TestTriggerCreatesGitLabPipeline(t *testing.T) {
	var seenPath string
	var seenRef string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.EscapedPath()
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if r.Header.Get("PRIVATE-TOKEN") != "token" {
			t.Fatalf("missing private token")
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		seenRef = r.Form.Get("ref")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     42,
			"status": "pending",
			"ref":    seenRef,
			"sha":    "abc123",
		})
	}))
	defer server.Close()

	registry := NewRegistryWithDatasources(
		"demo-api=namespace:cicd-devex-demo,deployment:demo-api,gitlab:tpo/devex/demo/demo-api",
		Datasources{GitLabURL: server.URL, GitLabToken: "token", GitOpsRef: "main"},
	)
	got, _, err := registry.Trigger(context.Background(), "demo-api", "main", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(seenPath, "/api/v4/projects/tpo%2Fdevex%2Fdemo%2Fdemo-api/pipeline") {
		t.Fatalf("path = %s", seenPath)
	}
	if seenRef != "main" {
		t.Fatalf("ref = %s", seenRef)
	}
	pipeline, _ := got["pipeline"].(map[string]any)
	if pipeline["status"] != "pending" {
		t.Fatalf("pipeline = %#v", pipeline)
	}
}

func TestDesiredImageFromManifestFindsContainerImage(t *testing.T) {
	manifest := `
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: sidecar
          image: registry/sidecar:v1
        - name: core
          image: 192.168.48.206:5050/platform/opspilot/opspilot-core:abc123
`
	got := desiredImageFromManifest(manifest, "core")
	want := "192.168.48.206:5050/platform/opspilot/opspilot-core:abc123"
	if got != want {
		t.Fatalf("desired image = %q, want %q", got, want)
	}
}

func TestReplaceImageInManifestUpdatesOnlyTargetContainer(t *testing.T) {
	manifest := `
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
        - name: sidecar
          image: registry/sidecar:v1
        - name: core
          ports:
            - name: http
              containerPort: 18080
          image: 192.168.48.206:5050/platform/opspilot/opspilot-core:abc123
`
	got, oldImage, err := replaceImageInManifest(manifest, "core", "192.168.48.206:5050/platform/opspilot/opspilot-core:def456")
	if err != nil {
		t.Fatal(err)
	}
	if oldImage != "192.168.48.206:5050/platform/opspilot/opspilot-core:abc123" {
		t.Fatalf("old image = %q", oldImage)
	}
	if desiredImageFromManifest(got, "sidecar") != "registry/sidecar:v1" {
		t.Fatalf("sidecar image changed: %s", got)
	}
	if desiredImageFromManifest(got, "core") != "192.168.48.206:5050/platform/opspilot/opspilot-core:def456" {
		t.Fatalf("core image not updated: %s", got)
	}
}

func TestImageWithTagPreservesRegistryPort(t *testing.T) {
	got, err := imageWithTag("192.168.48.206:5050/platform/opspilot/opspilot-core:abc123", "def456")
	if err != nil {
		t.Fatal(err)
	}
	want := "192.168.48.206:5050/platform/opspilot/opspilot-core:def456"
	if got != want {
		t.Fatalf("image = %q, want %q", got, want)
	}
}

func TestLooksLikeImage(t *testing.T) {
	if !looksLikeImage("192.168.48.206:5050/platform/opspilot/opspilot-core:def456") {
		t.Fatal("expected full image")
	}
	if looksLikeImage("def456") {
		t.Fatal("tag should not be treated as full image")
	}
}

func TestSplitImageNameTag(t *testing.T) {
	name, tag := splitImageNameTag("192.168.48.206:5050/platform/opspilot/opspilot-core:abc123")
	if name != "opspilot-core" || tag != "abc123" {
		t.Fatalf("name=%q tag=%q", name, tag)
	}
}

func TestGapDetailsExplainOptionalEvidenceGaps(t *testing.T) {
	details := gapDetails([]string{"pod_metrics_missing", "elk_logs_missing", "pod_metrics_missing"})
	if len(details) != 2 {
		t.Fatalf("details = %#v", details)
	}
	if details[0]["code"] != "pod_metrics_missing" || details[0]["blocking"] != false {
		t.Fatalf("pod metrics detail = %#v", details[0])
	}
	if !strings.Contains(details[1]["action"].(string), "datasource") {
		t.Fatalf("elk detail = %#v", details[1])
	}
}

func TestReconcileEvidenceDetectsStaleArgoCache(t *testing.T) {
	got := reconcileEvidence(map[string]any{
		"gitops": map[string]any{
			"status":        "differs_from_cluster",
			"desired_image": "registry/app:new",
		},
		"argocd": map[string]any{
			"sync_status":   "Synced",
			"health_status": "Healthy",
			"revision":      "old",
		},
	})
	if got["status"] != "pending_or_stale" || got["reason"] != "gitops_desired_image_differs_from_cluster" {
		t.Fatalf("reconcile = %#v", got)
	}
	if !strings.Contains(got["action"].(string), "hard refresh") {
		t.Fatalf("action = %#v", got)
	}
}

func TestLimitTailBytes(t *testing.T) {
	got, truncated := limitTailBytes("abcdef", 3)
	if got != "def" || !truncated {
		t.Fatalf("got=%q truncated=%t", got, truncated)
	}
}

func TestLimitTailLines(t *testing.T) {
	got, truncated := limitTailLines("one\ntwo\nthree", 2)
	if got != "two\nthree" || !truncated {
		t.Fatalf("got=%q truncated=%t", got, truncated)
	}
}
