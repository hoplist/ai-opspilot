package logsearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchUsesBoundedTimeWindowAndSourceFilter(t *testing.T) {
	var requestPath string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"hits":{"total":{"value":0},"hits":[]}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "apisix-*")
	result, err := client.Search(context.Background(), SearchRequest{Query: "error", SinceSeconds: 999999, Limit: 999})
	if err != nil {
		t.Fatal(err)
	}
	if requestPath != "/apisix-*/_search" {
		t.Fatalf("path = %s", requestPath)
	}
	if body["timeout"] != searchTimeout {
		t.Fatalf("timeout = %#v", body["timeout"])
	}
	if body["track_total_hits"] != false {
		t.Fatalf("track_total_hits = %#v", body["track_total_hits"])
	}
	if got := int(result["since_seconds"].(int)); got != MaxSearchSinceSeconds {
		t.Fatalf("since_seconds = %d", got)
	}
	if got := int(result["limit"].(int)); got != MaxSearchLimit {
		t.Fatalf("limit = %d", got)
	}
	raw, _ := json.Marshal(body)
	text := string(raw)
	for _, want := range []string{`"range"`, `"@timestamp"`, `"_source"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("query body missing %s: %s", want, text)
		}
	}
}
