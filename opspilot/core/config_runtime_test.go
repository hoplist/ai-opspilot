package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/configloader"
)

func TestRuntimeStateReloadUpdatesServiceMapping(t *testing.T) {
	dir := t.TempDir()
	writeRuntimeService(t, dir, "todo-server")
	cfg := configloader.Load(dir)
	state := newRuntimeState(cfg)
	if !hasString(state.snapshot().releaseRegistry.Services(), "todo-server") {
		t.Fatalf("initial services = %v", state.snapshot().releaseRegistry.Services())
	}

	writeRuntimeService(t, dir, "workflow-server")
	state.reload(configloader.Load(dir))
	services := state.snapshot().releaseRegistry.Services()
	if !hasString(services, "workflow-server") {
		t.Fatalf("reloaded services = %v", services)
	}

	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("kind: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	state.reload(configloader.Load(dir))
	services = state.snapshot().releaseRegistry.Services()
	if !hasString(services, "workflow-server") {
		t.Fatalf("invalid reload overwrote previous snapshot: %v", services)
	}
}

func writeRuntimeService(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, "services", "devex", name+".yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `apiVersion: opspilot.io/v1
kind: Service
metadata:
  name: ` + name + `
spec:
  runtime:
    cluster: node200-test
    namespace: demo
    deployment: ` + name + `
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasString(items []string, want string) bool {
	return strings.Contains(strings.Join(items, ","), want)
}
