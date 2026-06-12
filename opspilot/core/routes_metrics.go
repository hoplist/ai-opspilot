package main

import (
	"context"
	"net/http"
)

func registerMetricsRoutes(mux *http.ServeMux, state *runtimeState) {
	handleAPI(mux, "/api/metrics/health", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return state.snapshot().promRegistry.Health(ctx), nil, nil
	}))
	handleAPI(mux, "/api/metrics/datasources", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return state.snapshot().promRegistry.Health(ctx), nil, nil
	}))
	handleAPI(mux, "/api/metrics/query", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := required(r.URL.Query().Get("query"), "query")
		result, warnings, err := state.snapshot().promRegistry.QueryRaw(ctx, sourceQuery(r), q)
		return result, warnings, err
	}))
	handleAPI(mux, "/api/metrics/nodes", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		result, warnings, err := state.snapshot().promRegistry.NodeMetrics(ctx, sourceQuery(r), intQuery(r, "limit", 20))
		return result, warnings, err
	}))
	handleAPI(mux, "/api/metrics/pods", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, warnings, err := state.snapshot().promRegistry.PodMetrics(ctx, sourceQuery(r), q.Get("namespace"), q.Get("sort"), intQuery(r, "limit", 20))
		return result, warnings, err
	}))
	handleAPI(mux, "/api/metrics/containers", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, warnings, err := state.snapshot().promRegistry.ContainerMetrics(ctx, sourceQuery(r), q.Get("sort"), intQuery(r, "limit", 20))
		return result, warnings, err
	}))
	handleAPI(mux, "/api/metrics/pod", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		result, warnings, err := state.snapshot().promRegistry.SinglePodMetrics(ctx, sourceQuery(r), required(q.Get("namespace"), "namespace"), required(q.Get("pod"), "pod"))
		return result, warnings, err
	}))
}
