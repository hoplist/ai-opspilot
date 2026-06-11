package evidence

import (
	"testing"
	"time"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/retention"
)

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

func TestStoreRetentionMaxItems(t *testing.T) {
	store := NewStoreWithRetention(t.TempDir(), retention.Policy{MaxItems: 2})
	for i, name := range []string{"a", "b", "c"} {
		_, err := store.Write(Pack{
			ID:          name,
			GeneratedAt: time.Now().Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
			Trigger:     "test",
			Target:      Target{Type: "service", Name: name},
			Status:      "healthy",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.Recent(10)
	if err != nil {
		t.Fatal(err)
	}
	if got["count"].(int) != 2 {
		t.Fatalf("recent = %#v", got)
	}
}
