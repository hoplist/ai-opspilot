package inspection

import (
	"strings"
	"testing"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/configloader"
)

func TestRunReportsAdapterGaps(t *testing.T) {
	cfg := configloader.Config{
		Inspections: []configloader.Inspection{{
			Name:    "node200-daily",
			Cluster: "node200-test",
			Checks: []configloader.InspectionCheck{{
				Name:    "abnormal-pods",
				Type:    "kubernetes_pods",
				Enabled: boolPtr(true),
			}},
		}},
	}

	result := NewRegistry(cfg).Run(RunRequest{Name: "node200-daily"})
	if result.Ready {
		t.Fatalf("expected run to report missing adapter evidence")
	}
	if len(result.Checks) != 1 || result.Checks[0].Status != "missing_evidence" {
		t.Fatalf("unexpected check result: %#v", result.Checks)
	}
	if !contains(result.MissingEvidence, "inspection_check_adapter_not_configured:abnormal-pods") {
		t.Fatalf("missing adapter gap: %#v", result.MissingEvidence)
	}
}

func TestGenerateDraftIncludesFlowAndKafkaDatasource(t *testing.T) {
	cfg := configloader.Config{
		Flows: []configloader.Flow{{
			Name:    "crash-flow",
			Cluster: "node200-test",
		}},
		Datasources: []configloader.Datasource{{
			Name:    "node200-kafka",
			Kind:    "kafka_exporter",
			Cluster: "node200-test",
		}},
	}

	result := NewRegistry(cfg).Generate(GenerateRequest{Cluster: "node200-test", Service: "opspilot-core"})
	if !result.Ready {
		t.Fatalf("expected ready draft: %#v", result)
	}
	if !strings.Contains(result.YAML, "name: flow-health") {
		t.Fatalf("draft should include flow-health check:\n%s", result.YAML)
	}
	if !strings.Contains(result.YAML, "datasource: node200-kafka") {
		t.Fatalf("draft should include kafka datasource:\n%s", result.YAML)
	}
	if !strings.Contains(result.YAML, "thresholds:\n        cpu_usage_percent: 85\n        memory_usage_percent: 85") {
		t.Fatalf("draft should include node thresholds:\n%s", result.YAML)
	}
	if !strings.Contains(result.YAML, "thresholds:\n        free_gib: 20\n        usage_percent: 85") {
		t.Fatalf("draft should include filesystem thresholds:\n%s", result.YAML)
	}
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
