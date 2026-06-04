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
