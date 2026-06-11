package evidence

import "testing"

func TestStoreWriteAndRecent(t *testing.T) {
	store := NewStore(t.TempDir())
	_, err := store.Write(Pack{
		Trigger: "test",
		Target:  Target{Type: "service", Name: "opspilot-core"},
		Status:  "healthy",
		Summary: "ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	if got["count"].(int) != 1 {
		t.Fatalf("recent = %#v", got)
	}
}

func TestGapsFromCodes(t *testing.T) {
	gaps := GapsFromCodes([]string{"elk_logs_missing", "elk_logs_missing"})
	if len(gaps) != 1 || gaps[0].Code != "elk_logs_missing" {
		t.Fatalf("gaps = %#v", gaps)
	}
}
