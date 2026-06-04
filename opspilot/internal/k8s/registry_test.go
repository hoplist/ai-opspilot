package k8s

import (
	"strings"
	"testing"
)

func TestRegistryBuildsRemoteClientFromCatalog(t *testing.T) {
	registry := NewRegistry(RegistryConfig{
		CatalogRaw:     "prod-a=environment:prod,kubernetes:remote,secret:opspilot-cluster-prod-a,context:prod-a",
		DefaultCluster: "node200-test",
		KubeconfigDir:  "/var/run/opspilot/clusters",
	})

	client, warnings, err := registry.ClientFor("prod-a")
	if err != nil {
		t.Fatalf("ClientFor returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if client.ClusterName() != "prod-a" || client.mode != "remote" {
		t.Fatalf("client = %#v", client)
	}
	args := client.kubectlArgs([]string{"get", "pods"})
	want := []string{"--kubeconfig", "/var/run/opspilot/clusters/opspilot-cluster-prod-a/kubeconfig", "--context", "prod-a", "get", "pods"}
	if len(args) != len(want) {
		t.Fatalf("args = %v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %v, want %v", args, want)
		}
	}
}

func TestRegistryRejectsUnknownCluster(t *testing.T) {
	registry := NewRegistry(RegistryConfig{
		CatalogRaw:     "node200-test=environment:test,kubernetes:in-cluster",
		DefaultCluster: "node200-test",
	})

	if _, _, err := registry.ClientFor("prod-a"); err == nil {
		t.Fatal("expected unknown cluster error")
	}
}

func TestRegistryBuildsInClusterClientWithServiceAccountEnv(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "6443")
	registry := NewRegistry(RegistryConfig{
		CatalogRaw:     "node200-test=environment:test,kubernetes:in-cluster",
		DefaultCluster: "node200-test",
	})

	client, _, err := registry.ClientFor("node200-test")
	if err != nil {
		t.Fatalf("ClientFor returned error: %v", err)
	}
	if client.mode != "in-cluster" || client.host != "10.0.0.1" || client.port != "6443" {
		t.Fatalf("client = %#v", client)
	}
}

func TestRegistryRejectsNonKubernetesDatasource(t *testing.T) {
	registry := NewRegistry(RegistryConfig{
		CatalogRaw:     "node206-host=environment:test,kubernetes:host-agent,prometheus:node206-host",
		DefaultCluster: "node206-host",
	})

	if _, _, err := registry.ClientFor("node206-host"); err == nil || !strings.Contains(err.Error(), "does not have a Kubernetes datasource") {
		t.Fatalf("expected non Kubernetes datasource error, got %v", err)
	}
}
