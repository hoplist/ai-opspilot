package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestEvidenceRequestCommand(t *testing.T) {
	endpoint, values := evidenceCommand([]string{
		"request",
		"--host", "workflow.tpo.xzoa.com",
		"--uri", "/api/hr/queryUserScheduleList",
		"--service-index", "workflow-server*",
		"--service-uri-field", "msg",
	})
	if endpoint != "/api/evidence/request" {
		t.Fatalf("endpoint = %s", endpoint)
	}
	if values.Get("host") != "workflow.tpo.xzoa.com" {
		t.Fatalf("host = %s", values.Get("host"))
	}
	if values.Get("service_index") != "workflow-server*" {
		t.Fatalf("service_index = %s", values.Get("service_index"))
	}
}

func TestEvidenceRequestServiceOnlyCommand(t *testing.T) {
	_, values := evidenceCommand([]string{
		"request",
		"--uri", "/api/hr/queryUserScheduleList",
		"--service-index", "workflow-server*",
		"--service-only",
	})
	if values.Get("skip_apisix") != "true" {
		t.Fatalf("skip_apisix = %s", values.Get("skip_apisix"))
	}
	if values.Get("host") != "" {
		t.Fatalf("host = %s", values.Get("host"))
	}
}

func TestOnboardServicePlan(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opspilot.service.yaml")
	config := `name: skillshub-api
gitlabProject: platform/skillshub-api
language: go
build:
  entry: ./cmd/skillshub-api
  output: build/skillshub-api
runtime:
  port: 8080
  healthPath: /health
deploy:
  namespace: skillshub
  replicas: 2
  container: skillshub-api
dockerfile:
  mode: existing
  path: Dockerfile
ci:
  mode: include
release:
  prometheusSource: node200-k8s
`
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"onboard", "service", "--config", configPath}, &out); err != nil {
		t.Fatal(err)
	}
	var payload onboardResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Service != "skillshub-api" || payload.Mode != "plan" {
		t.Fatalf("payload = %#v", payload)
	}
	if !bytes.Contains(out.Bytes(), []byte("platform/skillshub-api")) {
		t.Fatalf("release mapping missing project: %s", out.String())
	}
}

func TestOnboardServiceWriteSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(wd)
	}()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	config := `name: demo-api
dockerfile:
  mode: generate
`
	if err := os.WriteFile("opspilot.service.yaml", []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("Dockerfile", []byte("FROM custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"onboard", "service", "--write"}, &out); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "FROM custom\n" {
		t.Fatalf("Dockerfile was overwritten: %s", string(body))
	}
	if _, err := os.Stat(filepath.Join("deploy", "k8s", "deployment.yaml")); err != nil {
		t.Fatal(err)
	}
}
