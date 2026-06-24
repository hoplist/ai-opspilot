package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/configloader"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/evidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/httpprobe"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
)

func registerProbeRoutes(mux *http.ServeMux, state *runtimeState, store *evidence.Store) {
	handleAPI(mux, "/api/probe/http", wrapPost(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if err := r.ParseForm(); err != nil {
			return nil, nil, requestError{message: "invalid form body"}
		}
		probe, err := httpprobe.Run(ctx, httpprobe.Request{
			Method:          r.Form.Get("method"),
			URL:             required(r.Form.Get("url"), "url"),
			Headers:         headersFromForm(r.Form["header"]),
			Body:            r.Form.Get("body"),
			ProbeID:         r.Form.Get("probe_id"),
			TimeoutSeconds:  intForm(r, "timeout_seconds", httpprobe.DefaultTimeoutSeconds),
			BodyLimitBytes:  intForm(r, "body_limit_bytes", httpprobe.DefaultBodyLimitBytes),
			IncludeResponse: boolForm(r, "include_response"),
		})
		if err != nil {
			return nil, nil, requestError{message: err.Error()}
		}

		snap := state.snapshot()
		policy := snap.config.ResolveProbePolicy(r.Form.Get("policy"))
		warnings := []string{}
		var correlation map[string]any
		logPlan := probeLogPlan(policy, r)
		if logPlan.enabled {
			correlation, err = snap.logClient.CorrelateRequest(ctx, logsearch.CorrelateRequest{
				Host:            firstNonEmpty(r.Form.Get("host"), probe.Host),
				URI:             firstNonEmpty(r.Form.Get("uri"), probe.Path),
				Status:          firstNonEmpty(r.Form.Get("status"), statusStringFromCode(probe.StatusCode)),
				At:              firstNonEmpty(r.Form.Get("at"), probe.CompletedAt),
				SinceSeconds:    intForm(r, "since_seconds", policy.Window.SinceSeconds),
				WindowSeconds:   intForm(r, "window_seconds", policy.Window.WindowSeconds),
				Limit:           intForm(r, "limit", policy.Window.Limit),
				IncludeOptions:  boolForm(r, "include_options"),
				SkipAPISIX:      logPlan.skipGateway,
				APISIXIndex:     firstNonEmpty(r.Form.Get("apisix_index"), logPlan.gatewayIndex),
				ServiceIndex:    firstNonEmpty(r.Form.Get("service_index"), logPlan.serviceIndex),
				ServiceURIField: firstNonEmpty(r.Form.Get("service_uri_field"), logPlan.serviceURIField),
				ProbeID:         probe.ProbeID,
				UserAgent:       probe.UserAgent,
				TraceID:         r.Form.Get("trace_id"),
				Keywords:        formList(r, "keyword"),
			})
			if err != nil {
				warnings = appendMissing(warnings, logPlan.onMissing, "logs", err.Error())
			}
		}

		kubernetes := map[string]any{}
		namespace := strings.TrimSpace(r.Form.Get("namespace"))
		pod := strings.TrimSpace(r.Form.Get("pod"))
		podEvidence := probeEvidence(policy, "kubernetes_pod", "pod")
		metricsEvidence := probeEvidence(policy, "prometheus_pod", "metrics")
		if configloader.ProbeEvidenceEnabled(podEvidence) && namespace != "" && pod != "" {
			client, clientWarnings, err := k8sClientForRequest(r, snap.k8sRegistry)
			warnings = append(warnings, clientWarnings...)
			if err != nil {
				warnings = appendMissing(warnings, podEvidence.OnMissing, "kubernetes", err.Error())
			} else if podContext, err := client.PodContext(ctx, namespace, pod); err != nil {
				warnings = appendMissing(warnings, podEvidence.OnMissing, "pod context", err.Error())
			} else {
				if configloader.ProbeEvidenceEnabled(metricsEvidence) {
					addPodMetrics(ctx, snap.promRegistry, firstNonEmpty(r.Form.Get("source"), metricsEvidence.Source), podContext, namespace, pod)
				}
				kubernetes["pod"] = podContext
			}
		}

		pack := buildHTTPProbePack(probe, policy, correlation, kubernetes, namespace, pod, warnings)
		if boolForm(r, "persist") && store != nil && store.Enabled() {
			path, err := store.Write(pack)
			if err != nil {
				return nil, warnings, err
			}
			pack.Evidence["stored_path"] = path
		}
		return map[string]any{
			"probe":         probe,
			"policy":        policy,
			"correlation":   correlation,
			"kubernetes":    kubernetes,
			"evidence_pack": evidence.Normalize(pack),
		}, warnings, nil
	}))
}

func buildHTTPProbePack(probe httpprobe.Result, policy configloader.ProbePolicy, correlation, kubernetes map[string]any, namespace, pod string, warnings []string) evidence.Pack {
	gaps := []string{}
	sources := []evidence.Source{{Name: "http_probe", Status: "available", Detail: fmt.Sprintf("%s %s -> %s", probe.Method, probe.URL, probe.Status)}}
	status := "healthy"
	if probe.Error != "" || probe.StatusCode >= 500 {
		status = "degraded"
	}
	if probe.StatusCode >= 400 && probe.StatusCode < 500 {
		status = "warning"
	}
	if correlation == nil && probeLogPlan(policy, nil).enabled {
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

type probeLogEvidencePlan struct {
	enabled         bool
	skipGateway     bool
	gatewayIndex    string
	serviceIndex    string
	serviceURIField string
	onMissing       string
}

func probeLogPlan(policy configloader.ProbePolicy, r *http.Request) probeLogEvidencePlan {
	if r != nil && boolForm(r, "skip_logs") {
		return probeLogEvidencePlan{}
	}
	gateway := probeEvidence(policy, "gateway_logs", "apisix", "nginx")
	service := probeEvidence(policy, "service_logs")
	gatewayEnabled := configloader.ProbeEvidenceEnabled(gateway)
	serviceEnabled := configloader.ProbeEvidenceEnabled(service)
	if r != nil && (boolForm(r, "skip_apisix") || boolForm(r, "service_only")) {
		gatewayEnabled = false
	}
	return probeLogEvidencePlan{
		enabled:         gatewayEnabled || serviceEnabled,
		skipGateway:     !gatewayEnabled,
		gatewayIndex:    gateway.Index,
		serviceIndex:    service.Index,
		serviceURIField: service.URIField,
		onMissing:       firstNonEmpty(service.OnMissing, gateway.OnMissing, "warn"),
	}
}

func probeEvidence(policy configloader.ProbePolicy, typesOrNames ...string) configloader.ProbeEvidencePolicy {
	for _, item := range policy.Evidence {
		for _, expected := range typesOrNames {
			if strings.EqualFold(item.Type, expected) || strings.EqualFold(item.Name, expected) {
				return item
			}
		}
	}
	return configloader.ProbeEvidencePolicy{Enabled: disabledBool(), OnMissing: "skip"}
}

func disabledBool() *bool {
	value := false
	return &value
}

func appendMissing(warnings []string, mode, source, detail string) []string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "skip", "ignore":
		return warnings
	default:
		return append(warnings, source+": "+detail)
	}
}

func headersFromForm(values []string) map[string]string {
	headers := map[string]string{}
	for _, raw := range values {
		key, value, ok := strings.Cut(raw, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		headers[key] = strings.TrimSpace(value)
	}
	return headers
}

func intForm(r *http.Request, name string, fallback int) int {
	raw := strings.TrimSpace(r.Form.Get(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		panic(requestError{message: name + " must be an integer"})
	}
	return value
}

func formList(r *http.Request, name string) []string {
	values := []string{}
	for _, raw := range r.Form[name] {
		for _, part := range strings.FieldsFunc(raw, func(ch rune) bool {
			return ch == ',' || ch == '|'
		}) {
			part = strings.TrimSpace(part)
			if part != "" {
				values = append(values, part)
			}
		}
	}
	return values
}

func statusStringFromCode(code int) string {
	if code == 0 {
		return ""
	}
	return strconv.Itoa(code)
}

func correlationStrengthLabel(correlation map[string]any) string {
	if correlation == nil {
		return "missing"
	}
	return fmt.Sprint(correlation["evidence_strength"])
}
