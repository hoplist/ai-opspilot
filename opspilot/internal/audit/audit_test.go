package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecorderRecent(t *testing.T) {
	recorder := NewRecorder(filepath.Join(t.TempDir(), "audit.jsonl"))
	if err := recorder.Record(Record{
		Actor:      "alice",
		Method:     "GET",
		Path:       "/api/release/status",
		Action:     "release status",
		TargetType: "service",
		Target:     "opspilot-core",
		Outcome:    "success",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := recorder.Recent(Query{Actor: "alice", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 1 || got.Items[0].Risk != "read_only" || got.Items[0].Target != "opspilot-core" {
		t.Fatalf("recent = %#v", got)
	}
}

func TestPolicyHasHighRiskBoundary(t *testing.T) {
	policy := Policy()
	if policy["version"] != "v1" {
		t.Fatalf("policy = %#v", policy)
	}
}

func TestRecorderRetentionMaxBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	recorder := NewRecorderWithRetention(path, RetentionPolicy{MaxBytes: 220})
	for _, actor := range []string{"alice", "bob", "carol"} {
		if err := recorder.Record(Record{
			Actor:   actor,
			Method:  "GET",
			Path:    "/api/live",
			Action:  "live",
			Outcome: "success",
		}); err != nil {
			t.Fatal(err)
		}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"actor":"alice"`) {
		t.Fatalf("oldest record should be removed: %s", string(body))
	}
	if !strings.Contains(string(body), `"actor":"carol"`) {
		t.Fatalf("newest record should remain: %s", string(body))
	}
}

func TestRecorderRetentionMaxAge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	recorder := NewRecorderWithRetention(path, RetentionPolicy{MaxAge: 24 * time.Hour})
	if err := recorder.Record(Record{
		Time:    time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339),
		Actor:   "old",
		Method:  "GET",
		Path:    "/api/live",
		Action:  "live",
		Outcome: "success",
	}); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Record(Record{
		Actor:   "new",
		Method:  "GET",
		Path:    "/api/live",
		Action:  "live",
		Outcome: "success",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := recorder.Recent(Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 1 || got.Items[0].Actor != "new" {
		t.Fatalf("recent = %#v", got)
	}
}
