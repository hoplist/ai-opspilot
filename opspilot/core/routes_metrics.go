package main

import (
	"context"
	"net/http"

	prom "github.com/dualistpeng-netizen/ai-observability/opspilot/internal/prometheus"
)

func registerMetricsRoutes(mux *http.ServeMux, promRegistry *prom.Registry) {
	mux.HandleFunc("/api/metrics/health", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return promRegistry.Health(ctx), nil, nil
	}))
	mux.HandleFunc("/api/metrics/datasources", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return promRegistry.Health(ctx), nil, nil
	}))
	mux.HandleFunc("/api/metrics/query", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := required(r.URL.Query().Get("query"), "query")
		result, warnings, err := promRegistry.QueryRaw(ctx, sourceQuery(r), q)
		return result, warnings, err
	}))
	mux.HandleFunc("/api/metrics/nodes", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		result, warnings, err := promRegistry.NodeMetrics(ctx, sourceQuery(r), intQuery(r, "limit", 20))
		return result, warnings, err
	}))
	mux.HandleFunc("/api/metrics/pods", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, warnings, err := promRegistry.PodMetrics(ctx, sourceQuery(r), q.Get("namespace"), q.Get("sort"), intQuery(r, "limit", 20))
		return result, warnings, err
	}))
	mux.HandleFunc("/api/metrics/containers", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, warnings, err := promRegistry.ContainerMetrics(ctx, sourceQuery(r), q.Get("sort"), intQuery(r, "limit", 20))
		return result, warnings, err
	}))
	mux.HandleFunc("/api/metrics/pod", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, warnings, err := promRegistry.SinglePodMetrics(ctx, sourceQuery(r), required(q.Get("namespace"), "namespace"), required(q.Get("pod"), "pod"))
		return result, warnings, err
	}))
}
