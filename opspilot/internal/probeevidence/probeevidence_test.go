package probeevidence

import (
	"testing"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/configloader"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/httpprobe"
)

func TestResolveLogPlanHonorsOverrides(t *testing.T) {
	policy := configloader.DefaultHTTPProbePolicy()
	plan := ResolveLogPlan(policy, LogOverrides{SkipGateway: true})
	if !plan.Enabled {
		t.Fatal("service logs should keep the log plan enabled when only gateway logs are skipped")
	}
	if !plan.SkipGateway {
		t.Fatal("gateway lookup should be skipped")
	}

	plan = ResolveLogPlan(policy, LogOverrides{SkipLogs: true})
	if plan.Enabled {
		t.Fatal("all log evidence should be disabled when logs are skipped")
	}
}

func TestBuildHTTPPackKeepsMissingEvidenceNonBlocking(t *testing.T) {
	policy := configloader.DefaultHTTPProbePolicy()
	pack := BuildHTTPPack(httpprobe.Result{
		ProbeID:    "probe-test",
		Method:     "GET",
		URL:        "http://example.test/api",
		Host:       "example.test",
		Path:       "/api",
		StatusCode: 200,
		Status:     "200 OK",
	}, policy, ResolveLogPlan(policy, LogOverrides{}), nil, nil, "", "", nil)

	if pack.Status != "healthy" {
		t.Fatalf("status = %s", pack.Status)
	}
	if len(pack.MissingEvidence) == 0 {
		t.Fatal("expected missing log evidence gap")
	}
}

func TestBuildHTTPPackDoesNotReportSkippedLogsAsMissing(t *testing.T) {
	policy := configloader.DefaultHTTPProbePolicy()
	pack := BuildHTTPPack(httpprobe.Result{
		ProbeID:    "probe-test",
		Method:     "GET",
		URL:        "http://example.test/api",
		Host:       "example.test",
		Path:       "/api",
		StatusCode: 200,
		Status:     "200 OK",
	}, policy, ResolveLogPlan(policy, LogOverrides{SkipLogs: true}), nil, nil, "", "", nil)

	if len(pack.MissingEvidence) != 0 {
		t.Fatalf("missing evidence = %#v", pack.MissingEvidence)
	}
}
