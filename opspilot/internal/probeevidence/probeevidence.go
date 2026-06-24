package probeevidence

import (
	"fmt"
	"strings"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/configloader"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/evidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/httpprobe"
)

type LogOverrides struct {
	SkipLogs    bool
	SkipGateway bool
}

type LogPlan struct {
	Enabled         bool
	SkipGateway     bool
	GatewayIndex    string
	ServiceIndex    string
	ServiceURIField string
	OnMissing       string
}

func BuildHTTPPack(probe httpprobe.Result, policy configloader.ProbePolicy, logPlan LogPlan, correlation, kubernetes map[string]any, namespace, pod string, warnings []string) evidence.Pack {
	gaps := []string{}
	sources := []evidence.Source{{Name: "http_probe", Status: "available", Detail: fmt.Sprintf("%s %s -> %s", probe.Method, probe.URL, probe.Status)}}
	status := "healthy"
	if probe.Error != "" || probe.StatusCode >= 500 {
		status = "degraded"
	}
	if probe.StatusCode >= 400 && probe.StatusCode < 500 {
		status = "warning"
	}
	if correlation == nil && logPlan.Enabled {
		gaps = append(gaps, "logs.unavailable")
		sources = append(sources, evidence.Source{Name: "logs", Status: "missing", Detail: "log correlation was skipped or unavailable"})
	} else if correlation != nil {
		gaps = append(gaps, stringSliceValue(correlation["gaps"])...)
		sources = append(sources, evidence.Source{Name: "log_correlation", Status: fmt.Sprint(correlation["evidence_strength"]), Detail: fmt.Sprint(correlation["investigation_mode"])})
	}
	if namespace != "" && pod != "" {
		if len(kubernetes) > 0 {
			sources = append(sources, evidence.Source{Name: "kubernetes", Status: "available", Detail: namespace + "/" + pod})
		} else {
			sources = append(sources, evidence.Source{Name: "kubernetes", Status: "missing", Detail: namespace + "/" + pod})
		}
	}
	target := evidence.Target{Type: "http_route", Name: probe.Host + probe.Path}
	if pod != "" {
		target.Namespace = namespace
	}
	return evidence.Pack{
		Trigger: "http_probe",
		Target:  target,
		Status:  status,
		Summary: fmt.Sprintf("HTTP probe %s returned status=%d duration_ms=%d; correlation=%s.",
			probe.ProbeID, probe.StatusCode, probe.DurationMs, correlationStrengthLabel(correlation)),
		Sources: sources,
		Evidence: map[string]any{
			"probe":       probe,
			"policy":      policy,
			"correlation": correlation,
			"kubernetes":  kubernetes,
		},
		MissingEvidence: evidence.GapsFromCodes(gaps),
		RecommendedActions: []evidence.Action{
			evidence.ReadOnlyNextCheck("probe http --url "+probe.URL+" --persist", "Repeat the controlled HTTP probe if the issue is intermittent."),
			evidence.ReadOnlyNextCheck("evidence request --host "+probe.Host+" --uri "+probe.Path, "Use the same host, URI, status, and time window to inspect gateway and service logs."),
		},
		Warnings: warnings,
	}
}

func ResolveLogPlan(policy configloader.ProbePolicy, overrides LogOverrides) LogPlan {
	if overrides.SkipLogs {
		return LogPlan{}
	}
	gateway := ResolveEvidence(policy, "gateway_logs", "apisix", "nginx")
	service := ResolveEvidence(policy, "service_logs")
	gatewayEnabled := configloader.ProbeEvidenceEnabled(gateway)
	serviceEnabled := configloader.ProbeEvidenceEnabled(service)
	if overrides.SkipGateway {
		gatewayEnabled = false
	}
	return LogPlan{
		Enabled:         gatewayEnabled || serviceEnabled,
		SkipGateway:     !gatewayEnabled,
		GatewayIndex:    gateway.Index,
		ServiceIndex:    service.Index,
		ServiceURIField: service.URIField,
		OnMissing:       firstNonEmpty(service.OnMissing, gateway.OnMissing, "warn"),
	}
}

func ResolveEvidence(policy configloader.ProbePolicy, typesOrNames ...string) configloader.ProbeEvidencePolicy {
	for _, item := range policy.Evidence {
		for _, expected := range typesOrNames {
			if strings.EqualFold(item.Type, expected) || strings.EqualFold(item.Name, expected) {
				return item
			}
		}
	}
	return configloader.ProbeEvidencePolicy{Enabled: disabledBool(), OnMissing: "skip"}
}

func Enabled(item configloader.ProbeEvidencePolicy) bool {
	return configloader.ProbeEvidenceEnabled(item)
}

func AppendMissing(warnings []string, mode, source, detail string) []string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "skip", "ignore":
		return warnings
	default:
		return append(warnings, source+": "+detail)
	}
}

func disabledBool() *bool {
	value := false
	return &value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func correlationStrengthLabel(correlation map[string]any) string {
	if correlation == nil {
		return "missing"
	}
	return fmt.Sprint(correlation["evidence_strength"])
}

func stringSliceValue(value any) []string {
	out := []string{}
	for _, item := range anySlice(value) {
		text := strings.TrimSpace(fmt.Sprint(item))
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

func anySlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}
