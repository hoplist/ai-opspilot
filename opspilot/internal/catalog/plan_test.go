package catalog

import "testing"

func TestCredentialRegistrationPlan(t *testing.T) {
	plan := CredentialRegistrationPlan(RegistrationPlanRequest{
		Kind:    "mysql",
		Service: "demo-api",
	})
	if plan.Type != "credential" || plan.Kind != "mysql" || plan.Automation != "plan_first" {
		t.Fatalf("plan = %#v", plan)
	}
	if plan.Credential.Name != "demo-api-mysql-credentials" || plan.Credential.Storage != "kubernetes-credential-ref" {
		t.Fatalf("credential = %#v", plan.Credential)
	}
	if len(plan.RequiredKeys) != 1 || plan.RequiredKeys[0] != "DATABASE_URL" {
		t.Fatalf("required keys = %v", plan.RequiredKeys)
	}
}

func TestDatasourceRegistrationPlan(t *testing.T) {
	plan := DatasourceRegistrationPlan(RegistrationPlanRequest{
		Kind:    "prometheus",
		Cluster: "node200-test",
		Name:    "node200-k8s",
	})
	if plan.Type != "datasource" || plan.ClusterMetadata.Prometheus != "node200-k8s" {
		t.Fatalf("plan = %#v", plan)
	}
	if len(plan.Validation) == 0 {
		t.Fatal("expected validation commands")
	}
}

func TestDebugAccessPlan(t *testing.T) {
	plan := CredentialRegistrationPlan(RegistrationPlanRequest{
		Kind:    "mysql",
		Service: "demo-api",
		Mode:    "readonly",
		TTL:     "2h",
	})
	if plan.Credential.Class != "debug-temporary" || plan.Mode != "readonly" || plan.TTL != "2h" {
		t.Fatalf("debug plan = %#v", plan)
	}
	if len(plan.Warnings) == 0 {
		t.Fatal("expected plan-only warnings")
	}
}

func TestClusterRegistrationPlan(t *testing.T) {
	plan := ClusterRegistrationPlan(RegistrationPlanRequest{
		Name:        "prod-a",
		Environment: "prod",
		Mode:        "remote",
	})
	if plan.Type != "cluster" || plan.ClusterMetadata.KubeconfigPath == "" {
		t.Fatalf("cluster plan = %#v", plan)
	}
	if len(plan.RequiredKeys) != 1 || plan.RequiredKeys[0] != "kubeconfig" {
		t.Fatalf("required keys = %v", plan.RequiredKeys)
	}
}
