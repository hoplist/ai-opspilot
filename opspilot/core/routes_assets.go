package main

import (
	"context"
	"net/http"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/assets"
)

func registerAssetRoutes(mux *http.ServeMux, state *runtimeState) {
	handleAPI(mux, "/api/assets/zones", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return map[string]any{
			"version": assets.Version,
			"count":   len(assets.Zones(state.snapshot().config)),
			"items":   assets.Zones(state.snapshot().config),
		}, nil, nil
	}))
	handleAPI(mux, "/api/assets/catalog", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return assets.Build(state.snapshot().config), nil, nil
	}))
	handleAPI(mux, "/api/assets/inspect", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return assets.InspectIP(state.snapshot().config, required(q.Get("ip"), "ip")), nil, nil
	}))
	handleAPI(mux, "/api/assets/diff", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return assets.Diff(state.snapshot().config), nil, nil
	}))
}
