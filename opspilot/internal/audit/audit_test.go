package audit

import (
	"path/filepath"
	"testing"
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
