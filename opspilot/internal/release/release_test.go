package release

import "testing"

func TestNewRegistryParsesServices(t *testing.T) {
	registry := NewRegistry("opspilot-core=namespace:opspilot,deployment:opspilot-core,container:core,source:node200-k8s,image:registry/app:tag,gitlab:platform/opspilot,gitops:clusters/test/apps/opspilot-core/deployment.yaml,argocd:opspilot-core")
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
	if service.Container != "core" || service.GitLab != "platform/opspilot" || service.GitOps == "" || service.ArgoCD != "opspilot-core" {
		t.Fatalf("service release fields = %#v", service)
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
