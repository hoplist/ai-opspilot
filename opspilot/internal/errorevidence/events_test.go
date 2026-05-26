package errorevidence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectorReadsMiddlewareFileEvents(t *testing.T) {
	dir := t.TempDir()
	body := `{
  "source": "middleware",
  "stage": "provision",
  "service": "orders-api",
  "namespace": "cicd-devex-orders",
  "resource": "mysql/devex_orders_orders_api_mysql",
  "message": "failed to create database user"
}`
	if err := os.WriteFile(filepath.Join(dir, "orders-api.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	result, warnings, err := NewCollector(dir).Recent(context.Background(), nil, nil, nil, nil, Request{Source: "middleware", Service: "orders-api"})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if result.ItemCount != 1 {
		t.Fatalf("result = %#v", result)
	}
	item := result.Items[0]
	if item.Source != "middleware" || item.Stage != "provision" || item.Service != "orders-api" || item.ID == "" || item.Severity != "warning" || item.Status != "open" {
		t.Fatalf("item = %#v", item)
	}
}
