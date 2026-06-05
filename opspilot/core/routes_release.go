package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/k8s"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/logsearch"
	prom "github.com/dualistpeng-netizen/ai-observability/opspilot/internal/prometheus"
	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/release"
)

func registerReleaseRoutes(mux *http.ServeMux, k8sRegistry *k8s.Registry, promRegistry *prom.Registry, logClient *logsearch.Client, releaseRegistry *release.Registry, qualitySettings release.QualitySettings) {
	mux.HandleFunc("/api/release/status", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if !releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		q := r.URL.Query()
		client, warnings, err := k8sClientForRequest(r, k8sRegistry)
		if err != nil {
			return nil, warnings, err
		}
		data, moreWarnings, err := releaseRegistry.Status(ctx, required(q.Get("service"), "service"), client, promRegistry, logClient, qualitySettings)
		return data, append(warnings, moreWarnings...), err
	}))
	mux.HandleFunc("/api/quality/status", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if !releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		q := r.URL.Query()
		client, warnings, err := k8sClientForRequest(r, k8sRegistry)
		if err != nil {
			return nil, warnings, err
		}
		data, moreWarnings, err := releaseRegistry.QualityStatus(ctx, required(q.Get("service"), "service"), client, qualitySettings)
		return data, append(warnings, moreWarnings...), err
	}))
	mux.HandleFunc("/api/quality/run", wrapPost(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if !releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		if err := r.ParseForm(); err != nil {
			return nil, nil, requestError{message: "form body is invalid"}
		}
		client, warnings, err := k8sClientForRequest(r, k8sRegistry)
		if err != nil {
			return nil, warnings, err
		}
		data, moreWarnings, err := releaseRegistry.RunQuality(ctx, required(r.Form.Get("service"), "service"), r.Form.Get("base_url"), client, qualitySettings)
		return data, append(warnings, moreWarnings...), err
	}))
	mux.HandleFunc("/api/release/jobs", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if !releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		q := r.URL.Query()
		return releaseRegistry.Jobs(ctx, required(q.Get("service"), "service"))
	}))
	mux.HandleFunc("/api/release/trigger", wrapPost(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if !releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		if err := r.ParseForm(); err != nil {
			return nil, nil, requestError{message: "form body is invalid"}
		}
		return releaseRegistry.Trigger(
			ctx,
			required(r.Form.Get("service"), "service"),
			r.Form.Get("ref"),
			releaseVariablesFromForm(r),
		)
	}))
	mux.HandleFunc("/api/release/logs", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if !releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		q := r.URL.Query()
		return releaseRegistry.JobTrace(
			ctx,
			required(q.Get("service"), "service"),
			int64Query(r, "job_id", 0),
			q.Get("job"),
			intQuery(r, "limit_bytes", 128*1024),
			intQueryAliases(r, []string{"tail_lines", "tail"}, 200),
		)
	}))
	mux.HandleFunc("/api/release/history", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if !releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		q := r.URL.Query()
		return releaseRegistry.History(ctx, required(q.Get("service"), "service"), intQuery(r, "limit", 10))
	}))
	mux.HandleFunc("/api/release/rollback", wrapPost(func(ctx context.Context, r *http.Request) (any, []string, error) {
		if !releaseRegistry.Configured() {
			return nil, nil, fmt.Errorf("release services are not configured")
		}
		if err := r.ParseForm(); err != nil {
			return nil, nil, requestError{message: "form body is invalid"}
		}
		return releaseRegistry.Rollback(
			ctx,
			required(r.Form.Get("service"), "service"),
			required(r.Form.Get("to"), "to"),
			boolForm(r, "confirm"),
		)
	}))
}
