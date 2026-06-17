package main

import (
	"context"
	"net/http"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/profile"
)

func registerProfileRoutes(mux *http.ServeMux, state *runtimeState) {
	handleAPI(mux, "/api/profiles/datasources", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return state.snapshot().profileRegistry.Health(ctx), nil, nil
	}))
	handleAPI(mux, "/api/profiles/status", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return state.snapshot().profileRegistry.Health(ctx), nil, nil
	}))
	handleAPI(mux, "/api/profiles/link", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return state.snapshot().profileRegistry.Link(ctx, profile.LinkRequest{
			Source:    q.Get("source"),
			Service:   q.Get("service"),
			Namespace: q.Get("namespace"),
			Pod:       q.Get("pod"),
			Since:     q.Get("since"),
		}), nil, nil
	}))
}
