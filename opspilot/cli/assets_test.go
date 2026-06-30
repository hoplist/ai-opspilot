package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCMDBSyncPlanAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/assets/sync-plan" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("source"); got != "jms-chengdu-inner" {
			t.Fatalf("source = %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"ok": true,
			"data": {
				"version": "v1",
				"mode": "readonly_plan",
				"source": "jms-chengdu-inner",
				"kind": "jms",
				"ready": false,
				"delete_policy": "mark_stale",
				"missing_evidence": ["cmdb_source_inactive"],
				"actions": ["Read assets from the configured CMDB/JMS source."],
				"validation": ["opspilot cmdb diff --output human"]
			}
		}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	err := run([]string{"--backend-url", server.URL, "--output", "human", "cmdb", "sync-plan", "--source", "jms-chengdu-inner"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CMDB sync plan", "readonly_plan", "mark_stale", "cmdb_source_inactive"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q: %s", want, out.String())
		}
	}
}
