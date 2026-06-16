package repoupload

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestClientCreatesProject(t *testing.T) {
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.EscapedPath())
		if r.Header.Get("PRIVATE-TOKEN") != "test-token" {
			t.Fatalf("missing private token")
		}
		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/tpo%2Fsandbox%2Fdevex%2Fdemo-api":
			http.NotFound(w, r)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/groups/tpo%2Fsandbox%2Fdevex":
			writeJSON(w, map[string]any{"id": 123, "full_path": "tpo/sandbox/devex"})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["path"] != "demo-api" || int(payload["namespace_id"].(float64)) != 123 {
				t.Fatalf("payload = %#v", payload)
			}
			writeJSON(w, map[string]any{
				"id":                  456,
				"path_with_namespace": "tpo/sandbox/devex/demo-api",
				"http_url_to_repo":    serverURLToRepo(r, "tpo/sandbox/devex/demo-api"),
				"web_url":             "http://gitlab.example/tpo/sandbox/devex/demo-api",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", server.Client())
	project, action, err := client.EnsureProject(context.Background(), "tpo/sandbox/devex/demo-api", true)
	if err != nil {
		t.Fatal(err)
	}
	if action != "created" || project.ID != 456 || project.PathWithNamespace != "tpo/sandbox/devex/demo-api" {
		t.Fatalf("project=%#v action=%s", project, action)
	}
	if len(requests) != 3 {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestTargetAllowed(t *testing.T) {
	allowed := ParseAllowedBases("tpo/sandbox/devex,tpo/apps/devex")
	if !TargetAllowed("tpo/sandbox/devex/demo-api", allowed) {
		t.Fatal("expected sandbox target to be allowed")
	}
	if TargetAllowed("tpo/platform/opspilot", allowed) {
		t.Fatal("expected platform target to be blocked")
	}
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		panic(err)
	}
}

func serverURLToRepo(r *http.Request, projectPath string) string {
	base := "http://" + r.Host
	escaped := url.PathEscape(projectPath)
	return base + "/" + escaped + ".git"
}
