package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaCommand(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"schema"}, &out); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["name"] != "opspilot" {
		t.Fatalf("name = %v", payload["name"])
	}
}

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--version"}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Fatal("version output is empty")
	}
}

func TestConfigValidateCommand(t *testing.T) {
	dir := t.TempDir()
	serviceDir := filepath.Join(dir, "services", "devex")
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serviceDir, "todo-server.yaml"), []byte(`apiVersion: opspilot.io/v1
kind: Service
metadata:
  name: todo-server
spec:
  domains:
    - todo.tpo.xzoa.com
  logs:
    app_indexes:
      - todo-server-*
`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"--output", "human", "config", "validate", "--dir", dir}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "services") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestConfigValidateCommandFailsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("kind: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"config", "validate", "--dir", dir}, &out); err == nil {
		t.Fatal("expected invalid config to fail")
	}
}

func TestConsumeGlobalFlags(t *testing.T) {
	opts := globalOptions{backendURL: "default", output: "json"}
	args := consumeGlobalFlags([]string{"--backend-url", "http://x", "--output", "table", "schema"}, &opts)
	if opts.backendURL != "http://x" {
		t.Fatalf("backend = %s", opts.backendURL)
	}
	if opts.output != "table" {
		t.Fatalf("output = %s", opts.output)
	}
	if len(args) != 1 || args[0] != "schema" {
		t.Fatalf("args = %#v", args)
	}
}

func TestCLISchemaIncludesSkillsMirrorCommands(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "contracts", "cli-schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range [][]byte{
		[]byte(`"name": "skills sources"`),
		[]byte(`"name": "skills candidates"`),
		[]byte(`"name": "skills discover"`),
		[]byte(`"name": "skills review"`),
		[]byte(`"name": "skills import-plan"`),
		[]byte(`"name": "skills promote"`),
		[]byte("Does not write files or enable the skill"),
	} {
		if !bytes.Contains(body, expected) {
			t.Fatalf("cli schema missing %s", expected)
		}
	}
}

func TestCLISchemaIncludesLifecyclePlanningCommands(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "contracts", "cli-schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range [][]byte{
		[]byte(`"name": "janitor plan"`),
		[]byte(`"name": "healer diagnose"`),
		[]byte(`"name": "app decommission plan"`),
		[]byte("high-risk actions are plan-only"),
	} {
		if !bytes.Contains(body, expected) {
			t.Fatalf("cli schema missing %s", expected)
		}
	}
	for _, args := range [][]string{
		{"janitor", "run"},
		{"healer", "fix"},
		{"app", "decommission", "run"},
	} {
		var out bytes.Buffer
		if err := run(args, &out); err == nil {
			t.Fatalf("expected %v to be disabled in v1", args)
		}
	}
}

func TestCLISchemaIncludesConfigCommands(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "contracts", "cli-schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range [][]byte{
		[]byte(`"name": "config validate"`),
		[]byte(`"name": "config status"`),
		[]byte("currently loaded config source"),
	} {
		if !bytes.Contains(body, expected) {
			t.Fatalf("cli schema missing %s", expected)
		}
	}
}
