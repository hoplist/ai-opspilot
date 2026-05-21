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

func TestSplitImageNameTag(t *testing.T) {
	name, tag := splitImageNameTag("192.168.48.206:5050/platform/opspilot/opspilot-core:abc123")
	if name != "opspilot-core" || tag != "abc123" {
		t.Fatalf("name=%q tag=%q", name, tag)
	}
}
