package profile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/configloader"
)

func TestHealthReportsMissingWhenNotConfigured(t *testing.T) {
	got := NewRegistry(configloader.Config{}).Health(context.Background())
	if got.Configured || got.Ready || len(got.MissingEvidence) == 0 {
		t.Fatalf("health = %#v", got)
	}
}

func TestHealthChecksParcaDatasource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/-/healthy" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	got := NewRegistry(configloader.Config{Datasources: []configloader.Datasource{
		{Name: "parca-test", Kind: "parca", Cluster: "node200-test", Region: "chengdu-inner", URL: server.URL},
	}}).Health(context.Background())
	if !got.Configured || !got.Ready || got.DatasourceCount != 1 || got.Datasources[0].Status != "ready" {
		t.Fatalf("health = %#v", got)
	}
	if got.Datasources[0].Cluster != "node200-test" {
		t.Fatalf("cluster = %q", got.Datasources[0].Cluster)
	}
}

func TestLinkBuildsUIHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	got := NewRegistry(configloader.Config{Datasources: []configloader.Datasource{
		{Name: "parca-test", Kind: "parca", URL: server.URL},
	}}).Link(context.Background(), LinkRequest{Namespace: "opspilot", Pod: "opspilot-core-abc", Since: "15m"})
	if !got.Ready || got.URL == "" {
		t.Fatalf("link = %#v", got)
	}
	if !strings.Contains(got.Query, `namespace="opspilot"`) || !strings.Contains(got.Query, `pod="opspilot-core-abc"`) {
		t.Fatalf("query = %s", got.Query)
	}
	if !strings.Contains(got.URL, "from=now-15m") {
		t.Fatalf("url = %s", got.URL)
	}
}

func TestHealthReportsServerOnlyWhenAgentDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	got := NewRegistry(configloader.Config{Datasources: []configloader.Datasource{
		{Name: "parca-test", Kind: "parca", URL: server.URL, Options: map[string]string{"agent_enabled": "false"}},
	}}).Health(context.Background())
	if got.Ready || got.Datasources[0].Status != "server_only" {
		t.Fatalf("health = %#v", got)
	}
	if len(got.MissingEvidence) == 0 || !strings.Contains(got.MissingEvidence[0], "profile_agent_disabled") {
		t.Fatalf("missing evidence = %#v", got.MissingEvidence)
	}
}
