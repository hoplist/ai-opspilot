package catalog

import "testing"

func TestCredentialsFromEnv(t *testing.T) {
	got, warnings := CredentialsFromEnv("name=opspilot-release-secrets,class=platform-runtime,scope=node200/opspilot,storage=kubernetes-secret,namespace=opspilot,used_by=opspilot-core|argocd,permissions=read_gitlab|write_gitops_confirmed")
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if got.Count != 1 || got.Items[0].Name != "opspilot-release-secrets" {
		t.Fatalf("catalog = %#v", got)
	}
	if len(got.Items[0].UsedBy) != 2 || len(got.Items[0].Permissions) != 2 {
		t.Fatalf("list fields not parsed: %#v", got.Items[0])
	}
}

func TestClustersFromEnv(t *testing.T) {
	got, warnings := ClustersFromEnv("node200-test=environment:test,kubernetes:in-cluster,prometheus:node200-k8s,gitops_project:platform/gitops-manifests,path:clusters/test,argocd_ns:argocd,registry:192.168.48.206:5050")
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if got.Count != 1 || got.Items[0].Name != "node200-test" {
		t.Fatalf("catalog = %#v", got)
	}
	if got.Items[0].KubernetesMode != "in-cluster" || got.Items[0].GitOpsPath != "clusters/test" {
		t.Fatalf("cluster not parsed: %#v", got.Items[0])
	}
}

func TestCatalogWarnsForMissingName(t *testing.T) {
	_, warnings := CredentialsFromEnv("class=platform-runtime")
	if len(warnings) == 0 {
		t.Fatal("expected warning")
	}
}
