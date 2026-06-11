package nodeagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseDataSources(t *testing.T) {
	sources := ParseDataSources("node206=http://192.168.48.206:19080,dev=http://127.0.0.1:19080/")
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(sources))
	}
	if sources[0].Name != "node206" || sources[0].URL != "http://192.168.48.206:19080" {
		t.Fatalf("unexpected first source: %#v", sources[0])
	}
	if sources[1].URL != "http://127.0.0.1:19080" {
		t.Fatalf("url should be trimmed: %s", sources[1].URL)
	}
}

func TestParseTokenMap(t *testing.T) {
	tokens := ParseTokenMap("node206=secret-a, dev = secret-b, missing")
	if tokens["node206"] != "secret-a" || tokens["dev"] != "secret-b" {
		t.Fatalf("unexpected tokens: %#v", tokens)
	}
}

func TestClientSendsBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-a" {
			t.Fatalf("Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewClientWithToken(server.URL, "secret-a")
	if _, err := client.Containers(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestBoundedLogRequest(t *testing.T) {
	req := BoundedLogRequest(LogRequest{TailLines: 99999, SinceSeconds: 999999, LimitBytes: 999999999})
	if req.TailLines != MaxTailLines {
		t.Fatalf("tail lines not clamped: %d", req.TailLines)
	}
	if req.SinceSeconds != MaxSinceSeconds {
		t.Fatalf("since seconds not clamped: %d", req.SinceSeconds)
	}
	if req.LimitBytes != MaxLimitBytes {
		t.Fatalf("limit bytes not clamped: %d", req.LimitBytes)
	}
}

func TestBoundedHostDiskRequest(t *testing.T) {
	req := BoundedHostDiskRequest(HostDiskRequest{Limit: 999, Depth: 99})
	if req.Limit != MaxDiskTopLimit {
		t.Fatalf("limit not clamped: %d", req.Limit)
	}
	if req.Depth != MaxDiskMaxDepth {
		t.Fatalf("depth not clamped: %d", req.Depth)
	}
}

func TestClientHostDiskPassesQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/host/disk" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("limit"); got != "7" {
			t.Fatalf("limit = %s", got)
		}
		if got := r.URL.Query().Get("depth"); got != "3" {
			t.Fatalf("depth = %s", got)
		}
		_, _ = w.Write([]byte(`{"filesystems":[],"top_paths":[]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	if _, err := client.HostDisk(context.Background(), HostDiskRequest{Limit: 7, Depth: 3}); err != nil {
		t.Fatal(err)
	}
}
