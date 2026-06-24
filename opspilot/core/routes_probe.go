package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

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
		warnings := []string{}
		var correlation map[string]any
		if !boolForm(r, "skip_logs") {
			correlation, err = snap.logClient.CorrelateRequest(ctx, logsearch.CorrelateRequest{
				Host:            firstNonEmpty(r.Form.Get("host"), probe.Host),
				URI:             firstNonEmpty(r.Form.Get("uri"), probe.Path),
				Status:          firstNonEmpty(r.Form.Get("status"), statusStringFromCode(probe.StatusCode)),
				At:              firstNonEmpty(r.Form.Get("at"), probe.CompletedAt),
				SinceSeconds:    intForm(r, "since_seconds", 900),
				WindowSeconds:   intForm(r, "window_seconds", 300),
				Limit:           intForm(r, "limit", 20),
				IncludeOptions:  boolForm(r, "include_options"),
				SkipAPISIX:      boolForm(r, "skip_apisix") || boolForm(r, "service_only"),
				APISIXIndex:     r.Form.Get("apisix_index"),
				ServiceIndex:    r.Form.Get("service_index"),
				ServiceURIField: r.Form.Get("service_uri_field"),
				ProbeID:         probe.ProbeID,
				UserAgent:       probe.UserAgent,
				TraceID:         r.Form.Get("trace_id"),
				Keywords:        formList(r, "keyword"),
			})
			if err != nil {
				warnings = append(warnings, "logs: "+err.Error())
			}
		}

		kubernetes := map[string]any{}
		namespace := strings.TrimSpace(r.Form.Get("namespace"))
		pod := strings.TrimSpace(r.Form.Get("pod"))
		if namespace != "" && pod != "" {
			client, clientWarnings, err := k8sClientForRequest(r, snap.k8sRegistry)
			warnings = append(warnings, clientWarnings...)
			if err != nil {
				warnings = append(warnings, "kubernetes: "+err.Error())
			} else if podContext, err := client.PodContext(ctx, namespace, pod); err != nil {
				warnings = append(warnings, "pod context: "+err.Error())
			} else {
				addPodMetrics(ctx, snap.promRegistry, r.Form.Get("source"), podContext, namespace, pod)
				kubernetes["pod"] = podContext
			}
		}

		pack := buildHTTPProbePack(probe, correlation, kubernetes, namespace, pod, warnings)
		if boolForm(r, "persist") && store != nil && store.Enabled() {
			path, err := store.Write(pack)
			if err != nil {
				return nil, warnings, err
			}
			pack.Evidence["stored_path"] = path
		}
		return map[string]any{
			"probe":         probe,
			"correlation":   correlation,
			"kubernetes":    kubernetes,
			"evidence_pack": evidence.Normalize(pack),
		}, warnings, nil
	}))
}

func buildHTTPProbePack(probe httpprobe.Result, correlation, kubernetes map[string]any, namespace, pod string, warnings []string) evidence.Pack {
	gaps := []string{}
	sources := []evidence.Source{{Name: "http_probe", Status: "available", Detail: fmt.Sprintf("%s %s -> %s", probe.Method, probe.URL, probe.Status)}}
	status := "healthy"
	if probe.Error != "" || probe.StatusCode >= 500 {
		status = "degraded"
	}
	if probe.StatusCode >= 400 && probe.StatusCode < 500 {
		status = "warning"
	}
	if correlation == nil {
		gaps = append(gaps, "logs.unavailable")
		sources = append(sources, evidence.Source{Name: "logs", Status: "missing", Detail: "log correlation was skipped or unavailable"})
	} else {
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
