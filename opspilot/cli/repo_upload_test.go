package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestRepoUploadRequiresConfirm(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo-api\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := run([]string{"repo", "upload", "--repo", dir, "--name", "demo-api"}, &out)
	if err == nil {
		t.Fatal("expected repo upload to require --confirm")
	}
	var payload repoUploadResult
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "planned" || payload.Ready {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Plan.Target.GitLabProject != "tpo/sandbox/devex/demo-api" {
		t.Fatalf("target = %#v", payload.Plan.Target)
	}
}

func TestRepoUploadGitLabClientCreatesProject(t *testing.T) {
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
			writeTestJSON(w, map[string]any{"id": 123, "full_path": "tpo/sandbox/devex"})
		case r.Method == http.MethodPost && r.URL.EscapedPath() == "/api/v4/projects":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["path"] != "demo-api" || int(payload["namespace_id"].(float64)) != 123 {
				t.Fatalf("payload = %#v", payload)
			}
			writeTestJSON(w, map[string]any{
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
	oldClient := cliHTTPClient
	cliHTTPClient = server.Client()
	defer func() { cliHTTPClient = oldClient }()

	client := newRepoUploadGitLabClient(server.URL, "test-token")
	project, action, err := client.ensureProject(context.Background(), "tpo/sandbox/devex/demo-api", true)
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

func serverURLToRepo(r *http.Request, projectPath string) string {
	base := "http://" + r.Host
	escaped := url.PathEscape(projectPath)
	return base + "/" + escaped + ".git"
}
