package configloader

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigDirectory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "credentials/test.yaml", `
credentials:
  - name: guangzhou-inner-es
    type: elasticsearch
    username: elastic
    password: "secret"
  - name: node206-agent
    type: token
    password: "agent-token"
`)
	writeFile(t, dir, "settings/platform.yaml", `
settings:
  default_cluster: node200-test
  kubeconfig_dir: /var/run/opspilot/clusters
  gitlab_url: http://gitlab.example
  gitops_project: platform/gitops-manifests
  gitops_ref: main
  quality:
    enabled: true
    runner_image: registry/opspilot-core:main
    image_pull_secret: gitlab-registry-pull
    ttl_seconds: 3600
    deadline_seconds: 120
`)
	writeFile(t, dir, "datasources/elasticsearch.yaml", `
apiVersion: opspilot.io/v1
kind: Datasource
metadata:
  name: guangzhou-inner-es
spec:
  kind: elasticsearch
  region: guangzhou-inner
  url: http://es.example:9200
  credential_ref: guangzhou-inner-es
  indexes:
    apisix: apisix-*
    app_default:
      - "*-server-*"
  fields:
    service_uri: msg
`)
	writeFile(t, dir, "datasources/prometheus.yaml", `
datasources:
  - name: node200-k8s
    kind: prometheus
    url: http://prometheus.example
`)
	writeFile(t, dir, "agents/node206.yaml", `
apiVersion: opspilot.io/v1
kind: Agent
metadata:
  name: node206
spec:
  environment: test
  url: http://node206-agent.example
  default: true
  credential_ref: node206-agent
`)
	writeFile(t, dir, "assets/network-zones.yaml", `
network_zones:
  - name: chengdu-inner
    region: chengdu
    zone: inner
    cidrs:
      - 10.65.0.0/16
    coverage: partial
    action_policy: advisory_only
  - name: guangzhou-cloud-entry
    region: guangzhou
    zone: entry
    entrypoints:
      - 10.236.12.19
    coverage: partial
asset_sources:
  - name: jumpserver-chengdu-inner
    kind: jumpserver
    region: chengdu
    network_zone: chengdu-inner
    enabled: false
    coverage: partial
assets:
  - name: node-a
    hostname: node-a
    ips:
      - 10.65.1.10
    asset_type: vm
    sources:
      - manual
    expected_sources:
      - jumpserver
      - prometheus
`)
	writeFile(t, dir, "services/devex/todo-server.yaml", `
apiVersion: opspilot.io/v1
kind: Service
metadata:
  name: todo-server
spec:
  group: devex
  environment: test
  domains:
    - todo.tpo.xzoa.com
  runtime:
    cluster: node200-test
    namespace: todo
    deployment: todo-server
    container: server
  logs:
    app_indexes:
      - todo-server-*
    message_fields:
      - msg
  correlation:
    require_uri: false
    path_prefixes:
      - /api/im/
`)

	cfg := Load(dir)
	if !cfg.Valid {
		t.Fatalf("config errors = %v", cfg.Errors)
	}
	if len(cfg.Services) != 1 || cfg.Services[0].Name != "todo-server" {
		t.Fatalf("services = %#v", cfg.Services)
	}
	if cfg.Datasources[0].Credential == nil || cfg.Datasources[0].Credential.Password != "secret" {
		t.Fatalf("credential was not attached: %#v", cfg.Datasources[0].Credential)
	}
	if !strings.Contains(cfg.ServiceCatalogRaw(), "domains:todo.tpo.xzoa.com") {
		t.Fatalf("service catalog raw = %s", cfg.ServiceCatalogRaw())
	}
	if !strings.Contains(cfg.CredentialCatalogRaw(), "password_set:true") {
		t.Fatalf("credential catalog raw = %s", cfg.CredentialCatalogRaw())
	}
	if strings.Contains(string(mustJSON(t, cfg.Summary())), "secret") {
		t.Fatalf("summary leaked password")
	}
	if !strings.Contains(cfg.CorrelationRoutesRaw(), "todo.tpo.xzoa.com|/api/im/|todo-server-*|msg") {
		t.Fatalf("routes raw = %s", cfg.CorrelationRoutesRaw())
	}
	if cfg.Settings.GitLabURL != "http://gitlab.example" || cfg.Settings.GitOpsProject != "platform/gitops-manifests" {
		t.Fatalf("settings = %#v", cfg.Settings)
	}
	if cfg.DefaultPrometheusSource() != "node200-k8s" || !strings.Contains(cfg.PrometheusDatasourcesRaw(), "node200-k8s=http://prometheus.example") {
		t.Fatalf("prometheus raw = %s", cfg.PrometheusDatasourcesRaw())
	}
	if cfg.DefaultNodeAgent() != "node206" || cfg.NodeAgentsRaw() != "node206=http://node206-agent.example" {
		t.Fatalf("agents raw = %s default=%s", cfg.NodeAgentsRaw(), cfg.DefaultNodeAgent())
	}
	if cfg.NodeAgentTokensRaw() != "node206=agent-token" {
		t.Fatalf("agent token raw = %s", cfg.NodeAgentTokensRaw())
	}
	if len(cfg.NetworkZones) != 2 || cfg.NetworkZones[0].Name != "chengdu-inner" {
		t.Fatalf("network zones = %#v", cfg.NetworkZones)
	}
	if len(cfg.AssetSources) != 1 || cfg.AssetSources[0].Name != "jumpserver-chengdu-inner" || cfg.AssetSources[0].Enabled == nil || *cfg.AssetSources[0].Enabled {
		t.Fatalf("asset sources = %#v", cfg.AssetSources)
	}
	if len(cfg.Assets) != 1 || cfg.Assets[0].Name != "node-a" {
		t.Fatalf("assets = %#v", cfg.Assets)
	}
	defaults := cfg.LogSearchDefaults()
	if defaults.URL != "http://es.example:9200" || defaults.Index != "*-server-*" || defaults.Username != "elastic" || defaults.Password != "secret" {
		t.Fatalf("log defaults = %#v", defaults)
	}
}

func TestLogSearchDefaultsSkipsKibanaDatasource(t *testing.T) {
	cfg := Config{Datasources: []Datasource{
		{Name: "kibana", Kind: "kibana", URL: "http://kibana.example"},
		{Name: "logs", Kind: "elasticsearch", URL: "http://es.example:9200", Indexes: DatasourceIndexes{AppDefault: []string{"app-*"}}},
	}}
	defaults := cfg.LogSearchDefaults()
	if defaults.URL != "http://es.example:9200" || defaults.Index != "app-*" {
		t.Fatalf("log defaults = %#v", defaults)
	}
}

func TestLoadConfigDirectoryReportsInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad.yaml", "kind: [")
	cfg := Load(dir)
	if cfg.Valid || len(cfg.Errors) == 0 {
		t.Fatalf("expected invalid config, got %#v", cfg)
	}
}

func TestLoadConfigDirectoryFollowsRootSymlink(t *testing.T) {
	dir := t.TempDir()
	realRoot := filepath.Join(dir, "root", ".worktrees", "abc123")
	writeFile(t, realRoot, "services/demo.yaml", `
apiVersion: opspilot.io/v1
kind: Service
metadata:
  name: demo
spec:
  runtime:
    namespace: demo
    deployment: demo
`)
	link := filepath.Join(dir, "current")
	if err := os.Symlink(realRoot, link); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
	cfg := Load(link)
	if !cfg.Valid {
		t.Fatalf("config errors = %v", cfg.Errors)
	}
	if len(cfg.Services) != 1 || cfg.Services[0].Name != "demo" {
		t.Fatalf("services = %#v", cfg.Services)
	}
	if cfg.Commit != "abc123" {
		t.Fatalf("commit = %q", cfg.Commit)
	}
}

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(body)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
