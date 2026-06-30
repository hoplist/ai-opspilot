package assets

import (
	"testing"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/configloader"
)

func TestInspectIPMatchesCIDRAndEntryPoint(t *testing.T) {
	cfg := configloader.Config{
		Source: "test",
		NetworkZones: []configloader.NetworkZone{
			{Name: "chengdu-inner", Region: "chengdu", Zone: "inner", CIDRs: []string{"10.65.0.0/16"}},
			{Name: "guangzhou-cloud-entry", Region: "guangzhou", Zone: "entry", EntryPoints: []string{"10.236.12.19"}},
		},
		AssetSources: []configloader.AssetSource{
			{Name: "jumpserver-chengdu-inner", Kind: "jumpserver", NetworkZone: "chengdu-inner", Enabled: boolPtr(false)},
			{Name: "prometheus-chengdu-inner", Kind: "prometheus", NetworkZone: "chengdu-inner", Enabled: boolPtr(false)},
		},
	}

	got := InspectIP(cfg, "10.65.1.10")
	if got.Zone == nil || got.Zone.Name != "chengdu-inner" {
		t.Fatalf("zone = %#v", got.Zone)
	}
	if len(got.Sources) != 2 {
		t.Fatalf("sources = %#v", got.Sources)
	}

	entry := InspectIP(cfg, "10.236.12.19")
	if entry.Zone == nil || entry.Zone.Name != "guangzhou-cloud-entry" || !entry.MatchedEntry {
		t.Fatalf("entry result = %#v", entry)
	}
}

func TestDiffIsAdvisoryOnly(t *testing.T) {
	cfg := configloader.Config{
		Source: "test",
		NetworkZones: []configloader.NetworkZone{
			{Name: "chengdu-inner", CIDRs: []string{"10.65.0.0/16"}},
		},
		Assets: []configloader.Asset{
			{
				Name:            "node-a",
				IPs:             []string{"10.65.1.10"},
				Sources:         []string{"manual"},
				ExpectedSources: []string{"jumpserver", "prometheus"},
			},
			{
				Name:    "old-prom-target",
				IPs:     []string{"10.65.1.11"},
				Sources: []string{"prometheus:node200"},
			},
		},
	}

	got := Diff(cfg)
	if got.Mode != "advisory_only" {
		t.Fatalf("mode = %s", got.Mode)
	}
	if !hasFinding(got.Findings, "missing_jumpserver") {
		t.Fatalf("missing_jumpserver not found: %#v", got.Findings)
	}
	if !hasFinding(got.Findings, "missing_prometheus") {
		t.Fatalf("missing_prometheus not found: %#v", got.Findings)
	}
	if !hasFinding(got.Findings, "remove_candidate") {
		t.Fatalf("remove_candidate not found: %#v", got.Findings)
	}
}

func TestSyncPlanForOptionalJMS(t *testing.T) {
	cfg := configloader.Config{
		Source: "test",
		AssetSources: []configloader.AssetSource{
			{
				Name:        "jms-chengdu-inner",
				Kind:        "jms",
				NetworkZone: "chengdu-inner",
				Enabled:     boolPtr(false),
				Required:    boolPtr(false),
				Sync: configloader.AssetSourceSync{
					Enabled:      boolPtr(false),
					Mode:         "readonly",
					DeletePolicy: "mark_stale",
				},
			},
		},
	}

	got := SyncPlanForSource(cfg, "jms-chengdu-inner")
	if got.Mode != "readonly_plan" || got.DeletePolicy != "mark_stale" {
		t.Fatalf("plan = %#v", got)
	}
	if got.Ready {
		t.Fatalf("disabled jms source should not be ready: %#v", got)
	}
	if !contains(got.MissingEvidence, "cmdb_source_inactive") || !contains(got.MissingEvidence, "cmdb_source_url_missing") {
		t.Fatalf("missing evidence = %#v", got.MissingEvidence)
	}
}

func TestSyncPlanMissingSourceIsNonBlocking(t *testing.T) {
	got := SyncPlanForSource(configloader.Config{Source: "test"}, "")
	if got.Ready {
		t.Fatalf("missing source should not be ready: %#v", got)
	}
	if !contains(got.MissingEvidence, "cmdb_source_missing") {
		t.Fatalf("missing evidence = %#v", got.MissingEvidence)
	}
	if len(got.Findings) == 0 || got.Findings[0].Severity != "warning" {
		t.Fatalf("findings = %#v", got.Findings)
	}
}

func hasFinding(items []Finding, typ string) bool {
	for _, item := range items {
		if item.Type == typ {
			return true
		}
	}
	return false
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func boolPtr(value bool) *bool {
	return &value
}
