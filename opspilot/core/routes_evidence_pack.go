package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/errorevidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/evidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/k8s"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/nodeagent"
	prom "github.com/dualistpeng-netizen/ai-observability/opspilot/internal/prometheus"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
)

func registerEvidencePackRoutes(mux *http.ServeMux, k8sRegistry *k8s.Registry, promRegistry *prom.Registry, agentRegistry *nodeagent.Registry, logClient *logsearch.Client, releaseRegistry *release.Registry, errorCollector *errorevidence.Collector, qualitySettings release.QualitySettings, store *evidence.Store) {
	handleAPI(mux, "/api/evidence/pack", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		client, warnings, err := k8sClientForRequest(r, k8sRegistry)
		if err != nil {
			return nil, warnings, err
		}
		pack, moreWarnings, err := buildEvidencePack(ctx, r, client, promRegistry, agentRegistry, logClient, releaseRegistry, errorCollector, qualitySettings)
		warnings = append(warnings, moreWarnings...)
		if err != nil {
			return nil, warnings, err
		}
		if boolQuery(r, "persist") && store != nil && store.Enabled() {
			path, err := store.Write(pack)
			if err != nil {
				return nil, warnings, err
			}
			pack.Evidence["stored_path"] = path
		}
		return evidence.Normalize(pack), warnings, nil
	}))
	handleAPI(mux, "/api/evidence/packs/recent", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if store == nil {
			store = evidence.NewStore("")
		}
		result, err := store.Recent(intQuery(r, "limit", 20))
		return result, nil, err
	}))
}

func buildEvidencePack(ctx context.Context, r *http.Request, client *k8s.Client, promRegistry *prom.Registry, agentRegistry *nodeagent.Registry, logClient *logsearch.Client, releaseRegistry *release.Registry, errorCollector *errorevidence.Collector, qualitySettings release.QualitySettings) (evidence.Pack, []string, error) {
	q := r.URL.Query()
	targetType := firstNonEmpty(q.Get("target_type"), q.Get("type"))
	service := q.Get("service")
	namespace := q.Get("namespace")
	pod := q.Get("pod")
	if targetType == "" {
		switch {
		case service != "":
			targetType = "service"
		case pod != "":
			targetType = "pod"
		default:
			targetType = "cluster"
		}
	}
	trigger := firstNonEmpty(q.Get("trigger"), "manual")
	warnings := []string{}
	evidenceMap := map[string]any{}
	gapCodes := []string{}
	status := "unknown"
	summary := ""

	capabilities, capabilityWarnings, err := buildCapabilities(ctx, client, promRegistry, agentRegistry, logClient, releaseRegistry, qualitySettings)
	warnings = append(warnings, capabilityWarnings...)
	if err == nil {
		evidenceMap["capabilities"] = map[string]any{
			"summary":            capabilities["summary"],
			"available_evidence": capabilities["available_evidence"],
			"missing_evidence":   capabilities["missing_evidence"],
		}
		gapCodes = append(gapCodes, standardMissingCodes(capabilities)...)
	} else {
		warnings = append(warnings, "capabilities: "+err.Error())
	}

	switch targetType {
	case "service":
		if service == "" {
			return evidence.Pack{}, warnings, requestError{message: "service is required"}
		}
		status, summary = addServiceEvidence(ctx, client, promRegistry, logClient, releaseRegistry, qualitySettings, service, evidenceMap, &warnings, &gapCodes)
	case "pod":
		if namespace == "" || pod == "" {
			return evidence.Pack{}, warnings, requestError{message: "namespace and pod are required"}
		}
		status, summary = addPodEvidence(ctx, client, promRegistry, namespace, pod, evidenceMap, &warnings)
	case "cluster":
		status, summary = addClusterEvidence(ctx, client, evidenceMap, &warnings)
	default:
		return evidence.Pack{}, warnings, requestError{message: "target_type must be service, pod, or cluster"}
	}

	events, eventWarnings, err := errorCollector.Recent(ctx, client, releaseRegistry, promRegistry, logClient, errorevidence.Request{
		Service:   service,
		Namespace: namespace,
		Limit:     intQuery(r, "event_limit", 10),
	})
	warnings = append(warnings, eventWarnings...)
	if err != nil {
		warnings = append(warnings, "events: "+err.Error())
	} else {
		evidenceMap["events"] = events
		if events.ItemCount > 0 && status == "healthy" {
			status = "degraded"
		}
	}

	target := evidence.Target{
		Type:      targetType,
		Name:      firstNonEmpty(service, pod),
		Namespace: namespace,
		Cluster:   client.ClusterName(),
	}
	return evidence.Pack{
		Trigger:         trigger,
		Target:          target,
		Status:          status,
		Summary:         summary,
		Sources:         packSources(evidenceMap, gapCodes),
		Evidence:        evidenceMap,
		MissingEvidence: evidence.GapsFromCodes(gapCodes),
		RecommendedActions: []evidence.Action{
			evidence.ReadOnlyNextCheck(nextCheckForTarget(target), "Continue with read-only evidence first; only generate a mutation plan after the evidence points to a concrete fix."),
			evidence.PlanOnlyAction("High-risk mutations such as namespace deletion, data deletion, or hostPath cleanup must remain plan-only.", []string{"Confirm target namespace/service.", "Confirm owner and GitOps path.", "Verify before/after evidence pack."}),
		},
		Warnings: warnings,
	}, warnings, nil
}

func addServiceEvidence(ctx context.Context, client *k8s.Client, promRegistry *prom.Registry, logClient *logsearch.Client, releaseRegistry *release.Registry, qualitySettings release.QualitySettings, service string, evidenceMap map[string]any, warnings *[]string, gapCodes *[]string) (string, string) {
	if releaseRegistry == nil || !releaseRegistry.Configured() {
		*gapCodes = append(*gapCodes, "release_mapping_missing")
		return "unknown", "Release service mapping is not configured; use Kubernetes pod inspection."
	}
	status, releaseWarnings, err := releaseRegistry.Status(ctx, service, client, promRegistry, logClient, qualitySettings)
	*warnings = append(*warnings, releaseWarnings...)
	if err != nil {
		*warnings = append(*warnings, "release: "+err.Error())
		*gapCodes = append(*gapCodes, "release_mapping_missing")
		return "unknown", "Release evidence could not be read."
	}
	evidenceMap["release"] = status
	*gapCodes = append(*gapCodes, stringSliceValue(status["gaps"])...)
	currentStatus := fmt.Sprint(status["status"])
	return currentStatus, fmt.Sprintf("Service %s release status is %s.", service, currentStatus)
}

func addPodEvidence(ctx context.Context, client *k8s.Client, promRegistry *prom.Registry, namespace, pod string, evidenceMap map[string]any, warnings *[]string) (string, string) {
	contextData, err := client.PodContext(ctx, namespace, pod)
	if err != nil {
		*warnings = append(*warnings, "pod context: "+err.Error())
		return "unknown", "Pod evidence could not be read."
	}
	addPodMetrics(ctx, promRegistry, "", contextData, namespace, pod)
	evidenceMap["inventory"] = contextData
	status := "unknown"
	if fmt.Sprint(contextData["status"]) != "" {
		status = fmt.Sprint(contextData["status"])
	}
	return status, fmt.Sprintf("Pod %s/%s status is %s.", namespace, pod, status)
}

func addClusterEvidence(ctx context.Context, client *k8s.Client, evidenceMap map[string]any, warnings *[]string) (string, string) {
	overview := client.InventoryOverview(ctx, 10)
	evidenceMap["inventory"] = overview
	status := "healthy"
	if abnormal, ok := overview["abnormal_pods"].(map[string]any); ok && intMapValue(abnormal, "total_count") > 0 {
		status = "degraded"
	}
	return status, "Cluster evidence pack includes inventory, recent events, and capability status."
}

func packSources(evidenceMap map[string]any, gapCodes []string) []evidence.Source {
	sources := []evidence.Source{}
	for _, name := range []string{"inventory", "release", "events", "capabilities"} {
		if _, ok := evidenceMap[name]; ok {
			sources = append(sources, evidence.Source{Name: name, Status: "available"})
		}
	}
	for _, gap := range evidence.GapsFromCodes(gapCodes) {
		sources = append(sources, evidence.Source{Name: gap.Code, Status: "missing", Detail: gap.Message})
	}
	return sources
}

func standardMissingCodes(capabilities map[string]any) []string {
	codes := []string{}
	for _, item := range toSlice(capabilities["capabilities"]) {
		capability, ok := item.(map[string]any)
		if !ok || boolMapValue(capability, "available") {
			continue
		}
		switch fmt.Sprint(capability["name"]) {
		case "elk_logs":
			codes = append(codes, "logs.unavailable")
		case "service_logs":
			codes = append(codes, "service_logs.missing")
		case "apisix_logs":
			codes = append(codes, "apisix.not_configured")
		case "release_mapping":
			codes = append(codes, "release_mapping_missing")
		}
	}
	return codes
}

func nextCheckForTarget(target evidence.Target) string {
	switch target.Type {
	case "service":
		return "inspect service --service " + target.Name
	case "pod":
		return "inspect pod --namespace " + target.Namespace + " --pod " + target.Name
	default:
		return "inspect cluster"
	}
}

func intMapValue(values map[string]any, key string) int {
	switch value := values[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
