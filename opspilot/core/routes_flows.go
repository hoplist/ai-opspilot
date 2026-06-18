package main

import (
	"context"
	"net/http"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/flow"
)

func registerFlowRoutes(mux *http.ServeMux, state *runtimeState) {
	handleAPI(mux, "/api/flows/catalog", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		return state.snapshot().flowRegistry.Catalog(), nil, nil
	}))
	handleAPI(mux, "/api/flows/inspect", wrap(func(ctx context.Context, r *http.Request) (any, []string, error) {
		q := r.URL.Query()
		return state.snapshot().flowRegistry.Inspect(flow.InspectRequest{
			Name:   q.Get("name"),
			Stage:  q.Get("stage"),
			Window: q.Get("window"),
		}), nil, nil
	}))
}
