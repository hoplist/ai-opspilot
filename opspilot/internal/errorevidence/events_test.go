package errorevidence

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/retention"
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

func TestCollectorCleanup(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.json", "b.json", "ignore.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := NewCollector(dir).Cleanup(retention.Policy{MaxItems: 1}); err != nil {
		t.Fatal(err)
	}
	jsonCount := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			jsonCount++
		}
	}
	if jsonCount != 1 {
		t.Fatalf("jsonCount = %d", jsonCount)
	}
	if _, err := os.Stat(filepath.Join(dir, "ignore.txt")); err != nil {
		t.Fatalf("non-event file should remain: %v", err)
	}
}
