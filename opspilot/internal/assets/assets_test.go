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

func hasFinding(items []Finding, typ string) bool {
	for _, item := range items {
		if item.Type == typ {
			return true
		}
	}
	return false
}

func boolPtr(value bool) *bool {
	return &value
}
