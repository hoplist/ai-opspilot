package datasource

import (
	"testing"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/configloader"
)

func TestResolvePrefersServiceGatewayDatasource(t *testing.T) {
	cfg := configloader.Config{
		Services: []configloader.Service{
			{
				Name:    "todo-api",
				Domains: []string{"todo.tpo.xzoa.com"},
				Runtime: configloader.RuntimeSpec{Cluster: "node200-test", Namespace: "todo", Deployment: "todo-api"},
				Logs:    configloader.ServiceLogSpec{AppIndexes: []string{"todo-api-*"}, MessageFields: []string{"msg"}},
				Gateway: configloader.GatewaySpec{Datasource: "gz-es", APISIXIndex: "apisix-gz-*"},
			},
		},
		Datasources: []configloader.Datasource{
			{Name: "node200-logs", Kind: "elasticsearch", Region: "chengdu-inner", URL: "http://cd-es:9200", Indexes: configloader.DatasourceIndexes{AppDefault: []string{"app-*"}}},
			{Name: "gz-es", Kind: "elasticsearch", Region: "guangzhou-inner", URL: "http://gz-es:9200", Indexes: configloader.DatasourceIndexes{APISIX: "apisix-*"}},
		},
		Clusters: []configloader.Cluster{
			{Name: "node200-test", Logs: "node200-logs"},
		},
	}
	result := Resolve(cfg, RouteRequest{Host: "todo.tpo.xzoa.com"})
	if result.Status != "ready" {
		t.Fatalf("status = %s gaps=%v", result.Status, result.Gaps)
	}
	if result.Selected == nil || result.Selected.Name != "gz-es" {
		t.Fatalf("selected = %#v", result.Selected)
	}
	if result.Selected.APISIXIndex != "apisix-gz-*" {
		t.Fatalf("apisix index = %s", result.Selected.APISIXIndex)
	}
	if got := result.Selected.ServiceIndexes[0]; got != "todo-api-*" {
		t.Fatalf("service index = %s", got)
	}
}

func TestResolveSkipsKibanaAsQueryable(t *testing.T) {
	cfg := configloader.Config{
		Datasources: []configloader.Datasource{
			{Name: "prom", Kind: "prometheus", Region: "guangzhou-inner", URL: "http://prom"},
			{Name: "kibana-gz", Kind: "kibana", Region: "guangzhou-inner", URL: "http://kibana"},
			{Name: "es-gz", Kind: "elasticsearch", Region: "guangzhou-inner", URL: "http://es"},
		},
	}
	result := Resolve(cfg, RouteRequest{Region: "guangzhou-inner"})
	if result.Status != "ready" {
		t.Fatalf("status = %s gaps=%v", result.Status, result.Gaps)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Name != "es-gz" {
		t.Fatalf("candidates = %#v", result.Candidates)
	}
	if len(result.UI) != 1 || result.UI[0].Name != "kibana-gz" {
		t.Fatalf("ui = %#v", result.UI)
	}
}

func TestResolveUsesNeighborOnlyAfterSameRegion(t *testing.T) {
	cfg := configloader.Config{
		Datasources: []configloader.Datasource{
			{Name: "cd-es", Kind: "elasticsearch", Region: "chengdu-inner", URL: "http://cd"},
			{Name: "cloud-es", Kind: "elasticsearch", Region: "cloud-inner", URL: "http://cloud"},
		},
		Topology: []configloader.Region{
			{Name: "chengdu-inner", Neighbors: []string{"cloud-inner"}},
		},
	}
	result := Resolve(cfg, RouteRequest{Region: "chengdu-inner"})
	if len(result.Candidates) != 2 {
		t.Fatalf("candidates = %#v", result.Candidates)
	}
	if result.Candidates[0].Name != "cd-es" || result.Candidates[1].Name != "cloud-es" {
		t.Fatalf("order = %#v", result.Candidates)
	}
}
