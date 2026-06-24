package main

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/evidence"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/httpprobe"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/probeevidence"
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
		logPlan := probeevidence.ResolveLogPlan(policy, probeevidence.LogOverrides{
			SkipLogs:    boolForm(r, "skip_logs"),
			SkipGateway: boolForm(r, "skip_apisix") || boolForm(r, "service_only"),
		})
		if logPlan.Enabled {
			correlation, err = snap.logClient.CorrelateRequest(ctx, logsearch.CorrelateRequest{
				Host:            firstNonEmpty(r.Form.Get("host"), probe.Host),
				URI:             firstNonEmpty(r.Form.Get("uri"), probe.Path),
				Status:          firstNonEmpty(r.Form.Get("status"), statusStringFromCode(probe.StatusCode)),
				At:              firstNonEmpty(r.Form.Get("at"), probe.CompletedAt),
				SinceSeconds:    intForm(r, "since_seconds", policy.Window.SinceSeconds),
				WindowSeconds:   intForm(r, "window_seconds", policy.Window.WindowSeconds),
				Limit:           intForm(r, "limit", policy.Window.Limit),
				IncludeOptions:  boolForm(r, "include_options"),
				SkipAPISIX:      logPlan.SkipGateway,
				APISIXIndex:     firstNonEmpty(r.Form.Get("apisix_index"), logPlan.GatewayIndex),
				ServiceIndex:    firstNonEmpty(r.Form.Get("service_index"), logPlan.ServiceIndex),
				ServiceURIField: firstNonEmpty(r.Form.Get("service_uri_field"), logPlan.ServiceURIField),
				ProbeID:         probe.ProbeID,
				UserAgent:       probe.UserAgent,
				TraceID:         r.Form.Get("trace_id"),
				Keywords:        formList(r, "keyword"),
			})
			if err != nil {
				warnings = probeevidence.AppendMissing(warnings, logPlan.OnMissing, "logs", err.Error())
			}
		}

		kubernetes := map[string]any{}
		namespace := strings.TrimSpace(r.Form.Get("namespace"))
		pod := strings.TrimSpace(r.Form.Get("pod"))
		podEvidence := probeevidence.ResolveEvidence(policy, "kubernetes_pod", "pod")
		metricsEvidence := probeevidence.ResolveEvidence(policy, "prometheus_pod", "metrics")
		if probeevidence.Enabled(podEvidence) && namespace != "" && pod != "" {
			client, clientWarnings, err := k8sClientForRequest(r, snap.k8sRegistry)
			warnings = append(warnings, clientWarnings...)
			if err != nil {
				warnings = probeevidence.AppendMissing(warnings, podEvidence.OnMissing, "kubernetes", err.Error())
			} else if podContext, err := client.PodContext(ctx, namespace, pod); err != nil {
				warnings = probeevidence.AppendMissing(warnings, podEvidence.OnMissing, "pod context", err.Error())
			} else {
				if probeevidence.Enabled(metricsEvidence) {
					addPodMetrics(ctx, snap.promRegistry, firstNonEmpty(r.Form.Get("source"), metricsEvidence.Source), podContext, namespace, pod)
				}
				kubernetes["pod"] = podContext
			}
		}

		pack := probeevidence.BuildHTTPPack(probe, policy, logPlan, correlation, kubernetes, namespace, pod, warnings)
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
